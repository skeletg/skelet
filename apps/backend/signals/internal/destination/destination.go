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
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

const (
	KindWebhook  = "webhook"
	KindGA4      = "ga4_mp"
	KindMetaCAPI = "meta_capi"

	batchSize = 500
)

type Destination struct {
	ID              uuid.UUID
	ChannelID       uuid.UUID
	Name            string
	Kind            string
	TargetURL       string
	Secret          string
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
	queries *signalsdb.Queries // signals: destination rows + forwarding cursor
	// commerce: the audience export reads segments, which stay core's
	spine  SpineReader // whichever engine holds the spine
	client *http.Client
	guard  URLGuard

	// AllowPrivateTargets is a dev-only SSRF-guard bypass so localhost receivers work
	AllowPrivateTargets bool
}

// URLGuard is the SSRF guard a destination is held to - the check at create time, and the transport
// the forwarder posts through, which is where a host that resolves private only at request time is caught
type URLGuard interface {
	ValidateTargetURL(raw string, allowPrivate bool) error
	SafeTransport(allowPrivate bool) *http.Transport
}

func NewService(queries *signalsdb.Queries, spine SpineReader, guard URLGuard, allowPrivate bool) *Service {
	return &Service{
		queries: queries,
		spine:   spine,
		guard:   guard,
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: guard.SafeTransport(allowPrivate),
		},
		AllowPrivateTargets: allowPrivate,
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

func (s *Service) validate(in UpsertInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("destination: name is required")
	}
	switch in.Kind {
	case KindWebhook, KindGA4, KindMetaCAPI:
	default:
		return fmt.Errorf("destination: unknown kind %q", in.Kind)
	}
	return s.guard.ValidateTargetURL(in.TargetURL, s.AllowPrivateTargets)
}

func (s *Service) Create(ctx context.Context, in UpsertInput) (Destination, error) {
	if err := s.validate(in); err != nil {
		return Destination{}, err
	}
	row, err := s.queries.CreateEventDestination(ctx, signalsdb.CreateEventDestinationParams{
		ChannelID:    in.ChannelID,
		Name:         in.Name,
		Kind:         in.Kind,
		TargetUrl:    in.TargetURL,
		Secret:       in.Secret,
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
		return Destination{}, fmt.Errorf("destination: id is required")
	}
	if err := s.validate(in); err != nil {
		return Destination{}, err
	}
	row, err := s.queries.UpdateEventDestination(ctx, signalsdb.UpdateEventDestinationParams{
		ID:           *in.ID,
		ChannelID:    in.ChannelID,
		Name:         in.Name,
		Kind:         in.Kind,
		TargetUrl:    in.TargetURL,
		Secret:       in.Secret,
		EventSources: emptyIfNil(in.EventSources),
		EventNames:   emptyIfNil(in.EventNames),
		IsActive:     in.IsActive,
	})
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
		return fmt.Errorf("destination not found")
	}
	return nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Destination, error) {
	row, err := s.queries.GetEventDestination(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	return toDestination(row), nil
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
			_ = s.queries.SetEventDestinationError(ctx, signalsdb.SetEventDestinationErrorParams{
				ID: row.ID, LastError: truncate(err.Error(), 500),
			})
		}
	}
	return nil
}

func (s *Service) forwardOne(ctx context.Context, row signalsdb.EventDestination) error {
	// NULL cursor = start at creation, forwarding the future not the historical spine
	cursorAt, cursorID := row.CreatedAt, uuid.Nil
	prevAt, prevID := row.CursorCreatedAt, row.CursorEventID
	if row.CursorCreatedAt.Valid {
		cursorAt = row.CursorCreatedAt.Time
		cursorID = row.CursorEventID.UUID
	}
	for {
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
			if err := s.deliver(ctx, toDestination(row), batch); err != nil {
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
func (s *Service) deliver(ctx context.Context, d Destination, batch []Event) error {
	body, err := ShapePayload(d.Kind, batch)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TargetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.Kind == KindWebhook && d.Secret != "" {
		// Same as admin webhooks - HMAC over "<ts>.<body>" plus the ts header, so receivers can bound replay
		ts := fmt.Sprintf("%d", time.Now().Unix())
		mac := hmac.New(sha256.New, []byte(d.Secret))
		mac.Write([]byte(ts + "."))
		mac.Write(body)
		req.Header.Set("X-Destination-Timestamp", ts)
		req.Header.Set("X-Destination-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", d.Name, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("post %s: HTTP %d", d.Name, resp.StatusCode)
	}
	return nil
}

// ---------- helpers ----------

func toDestination(row signalsdb.EventDestination) Destination {
	d := Destination{
		ID:           row.ID,
		ChannelID:    row.ChannelID,
		Name:         row.Name,
		Kind:         row.Kind,
		TargetURL:    row.TargetUrl,
		Secret:       row.Secret,
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
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
