package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const refreshTokenTTL = 30 * 24 * time.Hour

// How long a just-rotated token keeps handing back its successor - rotation is single-use, but an SSR storefront fires several prefetches near-simultaneously and without a grace window one wins the GETDEL and the rest hard-fail and wipe the session, so inside it a replayed or concurrent refresh gets the SAME successor instead of an error
const rotateGraceTTL = 30 * time.Second

// How long a rotated token's hash is remembered, so a replay AFTER the grace window reads as reuse - the signature of a stolen token - rather than an anonymous bad one, and a hit revokes the whole family as a tripwire; bounded well below the 30d token TTL to cap the marker keys
const reuseDetectWindow = 24 * time.Hour

// The backend owns the layout, Valkey keys plus a per-owner SET index or auth_refresh_tokens rows, while this stays storage-agnostic and just drives the protocol - with prefix == "admin" each token also gets a matching admin_sessions row keyed by the same hash, recording client IP and user-agent
type RefreshStore struct {
	backend  RefreshBackend
	sessions SessionLog
	prefix   string
	bus      Publisher
	// Optional - publishes killed session ids to services that cannot see admin_sessions
	revocations Revocations
}

func NewRefreshStore(backend RefreshBackend, sessions SessionLog, prefix string) *RefreshStore { // sessions keeps the per-device rows - pass nil for non-admin stores
	return &RefreshStore{backend: backend, sessions: sessions, prefix: prefix}
}

// Optional - without it a killed session still stops working here, just not at a service authorizing from a token
func (s *RefreshStore) SetRevocations(r Revocations) { s.revocations = r }

// Publishes the sessions a delete just removed, so a service holding one of their tokens stops honouring it
func (s *RefreshStore) revoked(ctx context.Context, ids []uuid.UUID) {
	if s.revocations == nil {
		return
	}
	for _, id := range ids {
		s.revocations.Revoke(ctx, id)
	}
}

func (s *RefreshStore) SetPublisher(p Publisher) { s.bus = p } // optional and nil-safe - tests skip it and publishing no-ops

func (s *RefreshStore) subject() Subject {
	if s.prefix == "admin" {
		return SubjectAdmin
	}
	return SubjectCustomer
}

func (s *RefreshStore) publish(ctx context.Context, e Event) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}

func (s *RefreshStore) Create(ctx context.Context, id uuid.UUID, ip, userAgent string) (token string, sessionID uuid.UUID, err error) { // the admin prefix also inserts an admin_sessions row with client IP and user-agent and returns its id, zero UUID otherwise
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", uuid.Nil, fmt.Errorf("generate token: %w", err)
	}
	token = hex.EncodeToString(raw)
	hash := hashToken(token)
	if err = s.backend.Put(ctx, hash, id, refreshTokenTTL); err != nil {
		return "", uuid.Nil, err
	}
	if s.prefix == "admin" && s.sessions != nil {
		sessionID, err = s.sessions.Create(ctx, id, hash, ip, userAgent)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("create admin session: %w", err)
		}
	}
	s.publish(ctx, &RefreshIssued{
		Subject:   s.subject(),
		SubjectID: id,
		SessionID: sessionID,
		IP:        ip,
		UserAgent: userAgent,
	})
	return token, sessionID, nil
}

func (s *RefreshStore) Validate(ctx context.Context, token string) (uuid.UUID, error) {
	id, err := s.backend.Get(ctx, hashToken(token))
	if errors.Is(err, ErrTokenNotFound) {
		return uuid.Nil, fmt.Errorf("invalid or expired refresh token")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup refresh token: %w", err)
	}
	return id, nil
}

func (s *RefreshStore) ValidateAndRevoke(ctx context.Context, token string) (uuid.UUID, error) { // atomic, so two concurrent refreshes can't both mint a successor family from one token - also drops the admin session row, like Revoke
	hash := hashToken(token)
	id, err := s.backend.Consume(ctx, hash)
	if errors.Is(err, ErrTokenNotFound) {
		return uuid.Nil, fmt.Errorf("invalid or expired refresh token")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("consume refresh token: %w", err)
	}
	if s.prefix == "admin" && s.sessions != nil {
		killed, err := s.sessions.Delete(ctx, hash)
		if err != nil {
			return uuid.Nil, fmt.Errorf("delete admin session: %w", err)
		}
		s.revoked(ctx, killed)
	}
	s.publish(ctx, &RefreshRevoked{Subject: s.subject(), SubjectID: id, Reason: "rotation"})
	return id, nil
}

func (s *RefreshStore) Rotate(ctx context.Context, oldToken string) (newToken string, ownerID uuid.UUID, err error) { // customer path only, since admin sessions don't rotate - it refuses the admin store
	if s.prefix == "admin" {
		return "", uuid.Nil, fmt.Errorf("rotate not supported for admin tokens")
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", uuid.Nil, fmt.Errorf("generate token: %w", err)
	}
	candidate := hex.EncodeToString(raw)
	oldHash := hashToken(oldToken)
	newHash := hashToken(candidate)

	res, err := s.backend.Rotate(ctx, oldHash, newHash, candidate, refreshTokenTTL, rotateGraceTTL)
	if err != nil {
		return "", uuid.Nil, err
	}
	switch res.Outcome {
	case RotateWon:
		ownerID = res.Owner
		_ = s.backend.MarkRotated(ctx, oldHash, ownerID, reuseDetectWindow) // so a later post-grace replay reads as reuse rather than an unknown token
		s.publish(ctx, &RefreshRevoked{Subject: s.subject(), SubjectID: ownerID, Reason: "rotation"})
		s.publish(ctx, &RefreshIssued{Subject: s.subject(), SubjectID: ownerID})
		return candidate, ownerID, nil
	case RotateGrace:
		return res.Successor, res.Owner, nil // someone already rotated this within the window - hand back their successor
	default: // RotateInvalid
		if owner, ok := s.backend.RotatedOwner(ctx, oldHash); ok { // a post-grace replay of a token we remember rotating is the stolen-token fingerprint - revoking the family logs out thief and victim, so the victim re-authenticates
			if err := s.RevokeAll(ctx, owner); err != nil {
				slog.Error("revoke refresh family after token reuse", "owner", owner, "err", err) // the reuse signal fired and the family survived - a stolen token is still live, so this needs an operator
			}
			s.publish(ctx, &RefreshRevoked{Subject: s.subject(), SubjectID: owner, Reason: "reuse_detected"})
		}
		return "", uuid.Nil, fmt.Errorf("invalid or expired refresh token")
	}
}

func (s *RefreshStore) Revoke(ctx context.Context, token string) error {
	hash := hashToken(token)
	ownerID, err := s.backend.Get(ctx, hash) // read the owner before deleting so the logout events can name it - an already-expired token reads as not-found, which isn't an error here
	if err != nil && !errors.Is(err, ErrTokenNotFound) {
		return fmt.Errorf("lookup refresh token: %w", err)
	}
	if err := s.backend.Delete(ctx, hash); err != nil {
		return err
	}
	if s.prefix == "admin" && s.sessions != nil {
		killed, err := s.sessions.Delete(ctx, hash)
		if err != nil {
			return fmt.Errorf("delete admin session: %w", err)
		}
		s.revoked(ctx, killed)
	}
	if ownerID != uuid.Nil { // single-token logout, from the admin and customer Logout mutations - an already-expired token isn't a real logout, so it emits nothing
		s.publish(ctx, &RefreshRevoked{Subject: s.subject(), SubjectID: ownerID, Reason: "logout"})
		s.publish(ctx, &Logout{Subject: s.subject(), SubjectID: ownerID, AllDevices: false})
	}
	return nil
}

func (s *RefreshStore) RevokeAll(ctx context.Context, id uuid.UUID) error {
	if err := s.backend.DeleteOwner(ctx, id); err != nil {
		return err
	}
	if s.prefix == "admin" && s.sessions != nil {
		killed, err := s.sessions.DeleteAll(ctx, id)
		if err != nil {
			return fmt.Errorf("delete admin sessions: %w", err)
		}
		s.revoked(ctx, killed)
	}
	s.publish(ctx, &RefreshRevoked{Subject: s.subject(), SubjectID: id, Reason: "revoke_all"}) // every session for the owner is gone - logout-all, password reset or account deletion - so one aggregate revoke rather than one per token
	return nil
}

func (s *RefreshStore) RevokeExcept(ctx context.Context, id uuid.UUID, currentHash string) error { // backs "logout other devices"
	if err := s.backend.DeleteOwnerExcept(ctx, id, currentHash); err != nil {
		return err
	}
	if s.prefix == "admin" && s.sessions != nil {
		killed, err := s.sessions.DeleteExcept(ctx, id, currentHash)
		if err != nil {
			return fmt.Errorf("delete other admin sessions: %w", err)
		}
		s.revoked(ctx, killed)
	}
	s.publish(ctx, &RefreshRevoked{Subject: s.subject(), SubjectID: id, Reason: "logout_other_devices"})
	return nil
}

func (s *RefreshStore) SessionActive(ctx context.Context, adminID, sessionID uuid.UUID) (bool, error) { // backs AdminSessionGuard - every logout path deletes the session row, so the access token stops working at once; reuses ListSessions since admin sessions are few
	rows, err := s.ListSessions(ctx, adminID)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row.ID == sessionID {
			return true, nil
		}
	}
	return false, nil
}

func (s *RefreshStore) ListSessions(ctx context.Context, id uuid.UUID) ([]AdminSession, error) { // an empty slice, never nil
	if s.prefix != "admin" || s.sessions == nil {
		return []AdminSession{}, nil
	}
	rows, err := s.sessions.List(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list admin sessions: %w", err)
	}
	if rows == nil {
		return []AdminSession{}, nil
	}
	return rows, nil
}

func HashToken(token string) string { // exported so resolvers can hash the bearer token to the session hash
	return hashToken(token)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
