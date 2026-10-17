// Package destination forwards analytics-spine events to external systems (signed webhooks, GA4 Measurement Protocol, Meta Conversions API)
// the Signals suite's on-ramp to ad platforms and warehouses
package destination

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

const (
	KindWebhook  = "webhook"
	KindGA4      = "ga4_mp"
	KindMetaCAPI = "meta_capi"

	batchSize = 500
	// A backlogged destination yields after this many batches, so one slow receiver can't hold the rest of the sweep
	maxBatchesPerRun = 20
	requestTimeout   = 15 * time.Second
	lastErrorMax     = 500
	responseDrain    = 4 << 10
)

var (
	ErrInvalid              = errors.New("invalid destination")
	ErrNotFound             = errors.New("destination not found")
	ErrAudienceNeedsWebhook = errors.New("audience sync posts to webhook destinations only")
	ErrSealedWithoutKey     = errors.New("destination secret is encrypted but SIGNALS_ENCRYPTION_KEY is not set")
)

// The query parameter GA4 and Meta read their credential from - it lives in the write-only secret and joins the URL only at send time
var credentialParam = map[string]string{KindGA4: "api_secret", KindMetaCAPI: "access_token"}

type Destination struct {
	ID              uuid.UUID
	ChannelID       uuid.UUID
	Name            string
	Kind            string
	TargetURL       string
	EventSources    []string
	EventNames      []string
	IsActive        bool
	LastError       string
	LastForwardedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Event struct {
	ID uuid.UUID `json:"id"`
	// Same for every copy of one logical event, so a receiver can drop a re-forwarded duplicate
	EventID    string          `json:"eventId"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurredAt"`
	CustomerID *uuid.UUID      `json:"customerId,omitempty"`
	AnonID     string          `json:"anonId,omitempty"`
	Source     string          `json:"source"`
	Properties json.RawMessage `json:"properties"`

	createdAt time.Time
}

// The spine read this needs, narrowed to one method so the engine stays pluggable - satisfied by *analytics.Recorder
type SpineReader interface {
	EventsForForwarding(ctx context.Context, channelID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]analytics.ForwardEvent, error)
}

type Service struct {
	queries      *signalsdb.Queries
	spine        SpineReader // whichever engine holds the spine
	client       *http.Client
	guard        URLGuard
	sealer       Sealer
	allowPrivate bool // dev-only SSRF-guard bypass so localhost receivers work
}

// Sealer is *secrets.Cipher - nil stores plaintext (dev without a key)
type Sealer interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(stored string) (string, error)
}

// URLGuard is the SSRF guard a destination is held to - the check at create time, and the transport
// the forwarder posts through, which is where a host that resolves private only at request time is caught
type URLGuard interface {
	ValidateTargetURL(raw string, allowPrivate bool) error
	SafeTransport(allowPrivate bool) *http.Transport
}

func NewService(queries *signalsdb.Queries, spine SpineReader, guard URLGuard, sealer Sealer, allowPrivate bool) *Service {
	return &Service{
		queries: queries,
		spine:   spine,
		guard:   guard,
		sealer:  sealer,
		client: &http.Client{
			Timeout:   requestTimeout,
			Transport: guard.SafeTransport(allowPrivate),
			// A redirect would re-send the signed body, or the credential in the query, to wherever the receiver points
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		allowPrivate: allowPrivate,
	}
}

// ---------- CRUD ----------

// UpsertInput carries a create/update - kind and target URL validated (same SSRF rules as webhooks)
type UpsertInput struct {
	ID           *uuid.UUID
	ChannelID    uuid.UUID
	Name         string
	Kind         string
	TargetURL    string
	Secret       string
	EventSources []string
	EventNames   []string
	IsActive     bool
}

func (s *Service) validate(in UpsertInput, creating bool) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	switch in.Kind {
	case KindWebhook, KindGA4, KindMetaCAPI:
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, in.Kind)
	}
	if err := s.guard.ValidateTargetURL(in.TargetURL, s.allowPrivate); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	u, err := url.Parse(in.TargetURL)
	if err != nil {
		return fmt.Errorf("%w: target url: %w", ErrInvalid, err)
	}
	for _, p := range credentialParam {
		if u.Query().Has(p) {
			return fmt.Errorf("%w: put the %s in the secret field, not the URL", ErrInvalid, p)
		}
	}
	if p, ok := credentialParam[in.Kind]; ok && creating && strings.TrimSpace(in.Secret) == "" {
		return fmt.Errorf("%w: %s needs its %s as the secret", ErrInvalid, in.Kind, p)
	}
	return nil
}

func (s *Service) seal(plain string) (string, error) {
	if s.sealer == nil || plain == "" {
		return plain, nil
	}
	return s.sealer.Encrypt(plain)
}

func (s *Service) open(stored string) (string, error) {
	if !secrets.IsEncrypted(stored) {
		return stored, nil
	}
	if s.sealer == nil {
		return "", ErrSealedWithoutKey
	}
	return s.sealer.Decrypt(stored)
}

func (s *Service) Create(ctx context.Context, in UpsertInput) (Destination, error) {
	if err := s.validate(in, true); err != nil {
		return Destination{}, err
	}
	secret, err := s.seal(in.Secret)
	if err != nil {
		return Destination{}, fmt.Errorf("seal secret: %w", err)
	}
	row, err := s.queries.CreateEventDestination(ctx, signalsdb.CreateEventDestinationParams{
		ChannelID:    in.ChannelID,
		Name:         in.Name,
		Kind:         in.Kind,
		TargetUrl:    in.TargetURL,
		Secret:       secret,
		EventSources: emptyIfNil(in.EventSources),
		EventNames:   emptyIfNil(in.EventNames),
		IsActive:     in.IsActive,
	})
	if err != nil {
		return Destination{}, fmt.Errorf("create destination: %w", err)
	}
	return toDestination(row), nil
}

func (s *Service) Update(ctx context.Context, in UpsertInput) (Destination, error) {
	if in.ID == nil {
		return Destination{}, fmt.Errorf("%w: id is required", ErrInvalid)
	}
	if err := s.validate(in, false); err != nil {
		return Destination{}, err
	}
	secret, err := s.seal(in.Secret)
	if err != nil {
		return Destination{}, fmt.Errorf("seal secret: %w", err)
	}
	row, err := s.queries.UpdateEventDestination(ctx, signalsdb.UpdateEventDestinationParams{
		ID:           *in.ID,
		ChannelID:    in.ChannelID,
		Name:         in.Name,
		Kind:         in.Kind,
		TargetUrl:    in.TargetURL,
		Secret:       secret,
		EventSources: emptyIfNil(in.EventSources),
		EventNames:   emptyIfNil(in.EventNames),
		IsActive:     in.IsActive,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Destination{}, ErrNotFound
	}
	if err != nil {
		return Destination{}, fmt.Errorf("update destination: %w", err)
	}
	return toDestination(row), nil
}

// Update and Delete match on the channel as well as the id, so a row of another channel is never touched
func (s *Service) Delete(ctx context.Context, channelID, id uuid.UUID) error {
	n, err := s.queries.DeleteEventDestination(ctx, signalsdb.DeleteEventDestinationParams{ID: id, ChannelID: channelID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Destination, error) {
	row, err := s.get(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	return toDestination(row), nil
}

func (s *Service) get(ctx context.Context, id uuid.UUID) (signalsdb.EventDestination, error) {
	row, err := s.queries.GetEventDestination(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get destination: %w", err)
	}
	return row, nil
}

func (s *Service) ListByChannel(ctx context.Context, channelID uuid.UUID) ([]Destination, error) {
	rows, err := s.queries.ListEventDestinationsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]Destination, len(rows))
	for i, r := range rows {
		out[i] = toDestination(r)
	}
	return out, nil
}

// Every active destination across channels - the health panel reports forwarding freshness stack-wide, not per store
func (s *Service) ListActive(ctx context.Context) ([]Destination, error) {
	rows, err := s.queries.ListActiveEventDestinations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Destination, len(rows))
	for i, r := range rows {
		out[i] = toDestination(r)
	}
	return out, nil
}

// ---------- forwarding sweep ----------

// ForwardAll sweeps every active destination - a failure is recorded and skipped
// so one dead endpoint never blocks the others
func (s *Service) ForwardAll(ctx context.Context) error {
	rows, err := s.queries.ListActiveEventDestinations(ctx)
	if err != nil {
		return fmt.Errorf("destination sweep: list: %w", err)
	}
	for _, row := range rows {
		if err := s.forwardOne(ctx, row); err != nil {
			log.Printf("[destination] %s (%s): %v", row.Name, row.ID, err)
			if serr := s.queries.SetEventDestinationError(ctx, signalsdb.SetEventDestinationErrorParams{
				ID: row.ID, LastError: truncate(err.Error(), lastErrorMax),
			}); serr != nil {
				log.Printf("[destination] %s (%s): record error: %v", row.Name, row.ID, serr)
			}
		}
	}
	return nil
}

func (s *Service) forwardOne(ctx context.Context, row signalsdb.EventDestination) error {
	secret, err := s.open(row.Secret)
	if err != nil {
		return err
	}
	// NULL cursor = start at creation, forwarding the future not the historical spine
	cursorAt, cursorID := row.CreatedAt, uuid.Nil
	prevAt, prevID := row.CursorCreatedAt, row.CursorEventID
	if row.CursorCreatedAt.Valid {
		cursorAt = row.CursorCreatedAt.Time
		cursorID = row.CursorEventID.UUID
	}
	for range maxBatchesPerRun {
		rows, err := s.spine.EventsForForwarding(ctx, row.ChannelID, cursorAt, cursorID, batchSize)
		if err != nil {
			return fmt.Errorf("page events: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		var batch []Event
		for _, r := range rows {
			e := Event{
				ID: r.ID, EventID: stableEventID(row.ChannelID, r), Name: r.Name, OccurredAt: r.OccurredAt, Source: r.Source,
				CustomerID: r.CustomerID, AnonID: r.AnonID, Properties: r.Properties, createdAt: r.CreatedAt,
			}
			if matchesFilters(e, row.EventSources, row.EventNames) {
				batch = append(batch, e)
			}
		}
		if len(batch) > 0 {
			if err := s.deliver(ctx, toDestination(row), secret, batch); err != nil {
				return err
			}
		}
		last := rows[len(rows)-1]
		cursorAt, cursorID = last.CreatedAt, last.ID
		n, err := s.queries.AdvanceEventDestinationCursor(ctx, signalsdb.AdvanceEventDestinationCursorParams{
			ID:              row.ID,
			CursorCreatedAt: sql.NullTime{Time: cursorAt, Valid: true},
			CursorEventID:   uuid.NullUUID{UUID: cursorID, Valid: true},
			PrevCreatedAt:   prevAt,
			PrevEventID:     prevID,
		})
		if err != nil {
			return fmt.Errorf("advance cursor: %w", err)
		}
		if n == 0 {
			// another sweep advanced this destination first (server + worker co-running) - concede rather than regress its cursor
			return nil
		}
		prevAt = sql.NullTime{Time: cursorAt, Valid: true}
		prevID = uuid.NullUUID{UUID: cursorID, Valid: true}
		if len(rows) < batchSize {
			return nil
		}
	}
	return nil
}

func matchesFilters(e Event, sources, names []string) bool {
	return (len(sources) == 0 || containsFold(sources, e.Source)) &&
		(len(names) == 0 || containsFold(names, e.Name))
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

// deliver shapes + posts one batch - Non-2xx is an error (cursor stays)
func (s *Service) deliver(ctx context.Context, d Destination, secret string, batch []Event) error {
	bodies, err := ShapePayload(d.Kind, batch)
	if err != nil {
		return err
	}
	for _, body := range bodies {
		if err := s.post(ctx, d, secret, body); err != nil {
			return err
		}
	}
	return nil
}

// post sends one body - errors name the destination, never the URL, which can carry the credential once it is added
func (s *Service) post(ctx context.Context, d Destination, secret string, body []byte) error {
	target, err := withCredential(d.Kind, d.TargetURL, secret)
	if err != nil {
		return fmt.Errorf("post %s: target url is invalid", d.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post %s: target url is invalid", d.Name)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.Kind == KindWebhook && secret != "" {
		sign(req, secret, body)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return fmt.Errorf("post %s: %w", d.Name, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, responseDrain))
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return fmt.Errorf("post %s: HTTP %d - redirects are not followed, point the destination at the final URL", d.Name, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("post %s: HTTP %d", d.Name, resp.StatusCode)
	}
	return nil
}

// Same as admin webhooks - HMAC over "<ts>.<body>" plus the ts header, so receivers can bound replay
func sign(req *http.Request, secret string, body []byte) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	req.Header.Set("X-Destination-Timestamp", ts)
	req.Header.Set("X-Destination-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func withCredential(kind, target, secret string) (string, error) {
	p, ok := credentialParam[kind]
	if !ok || secret == "" {
		return target, nil
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set(p, secret)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ---------- helpers ----------

func toDestination(row signalsdb.EventDestination) Destination {
	d := Destination{
		ID:           row.ID,
		ChannelID:    row.ChannelID,
		Name:         row.Name,
		Kind:         row.Kind,
		TargetURL:    row.TargetUrl,
		EventSources: row.EventSources,
		EventNames:   row.EventNames,
		IsActive:     row.IsActive,
		LastError:    row.LastError,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if row.LastForwardedAt.Valid {
		t := row.LastForwardedAt.Time
		d.LastForwardedAt = &t
	}
	return d
}

func emptyIfNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// truncate cuts on a rune boundary - proto strings must be valid UTF-8, so a split rune would fail the whole list call
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

var forwardEventNS = uuid.MustParse("6f1c2a3e-8d4b-5e7f-9a01-3c5d7e9f1b24")

// A UUID derived from the event's dedupe key, so every re-ingested copy of an event forwards under one id - the row id where the event carries no key
func stableEventID(channelID uuid.UUID, e analytics.ForwardEvent) string {
	if e.DedupeKey == "" {
		return e.ID.String()
	}
	return uuid.NewSHA1(forwardEventNS, []byte(channelID.String()+"|"+e.DedupeKey)).String()
}
