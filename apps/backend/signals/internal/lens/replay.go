package lens

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// One /t/rec flush - raw rrweb events for a session plus the metadata to attribute and index the recording - Events stays raw JSON: the server gzips it verbatim and reads only each event's timestamp, for the chunk's time range
type ReplayBatch struct {
	SessionID string          `json:"sessionId"`
	AnonID    string          `json:"anonId"`
	Seq       int32           `json:"seq"` // monotonic per-session (client sessionStorage)
	Page      string          `json:"page"`
	Viewport  string          `json:"viewport"`
	Events    json.RawMessage `json:"events"`
}

// Gzips the events into an append-only chunk and rolls the session metadata forward - best-effort, since the handler always 204s, so a malformed or oversized flush is dropped whole
// verifiedCustomer is the customer the request's session token proves, the only identity a recording is attached to - the body's customerId is forgeable
func (s *Service) IngestReplay(ctx context.Context, channelID uuid.UUID, userAgent string, b ReplayBatch, verifiedCustomer uuid.UUID) {
	if len(b.Events) == 0 || len(b.Events) > MaxReplayChunkBytes || b.Seq < 0 || b.Seq >= maxReplayChunks {
		return
	}
	sid, err := uuid.Parse(b.SessionID)
	if err != nil || b.AnonID == "" {
		return
	}
	// Just the timestamps - rrweb events all carry one in epoch ms - for the chunk's time range and event count
	var stamped []struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(b.Events, &stamped); err != nil || len(stamped) == 0 {
		return
	}
	firstTS, lastTS := stamped[0].Timestamp, stamped[0].Timestamp
	for _, e := range stamped {
		firstTS, lastTS = min(firstTS, e.Timestamp), max(lastTS, e.Timestamp)
	}
	// A retransmitted seq writes nothing twice, and the recording-length cap the tracker honours holds server-side too
	state, err := s.q.LensReplayState(ctx, signalsdb.LensReplayStateParams{SessionID: sid, Seq: b.Seq})
	if err != nil || state.SeqTaken {
		return
	}
	if state.FirstTs > 0 {
		firstTS = min(firstTS, state.FirstTs)
	}
	if limit := s.replayMaxMinutes(ctx, channelID); limit > 0 && time.Duration(lastTS-firstTS)*time.Millisecond > time.Duration(limit)*time.Minute+replayGrace {
		return
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b.Events); err != nil {
		return
	}
	if err := zw.Close(); err != nil {
		return
	}

	var cust uuid.NullUUID
	if verifiedCustomer != uuid.Nil {
		cust = uuid.NullUUID{UUID: verifiedCustomer, Valid: true}
	}
	page, _ := analytics.PageKey(b.Page)
	viewport := b.Viewport
	if !viewports[viewport] {
		viewport = ""
	}
	now := time.Now()
	n, err := s.q.LensUpsertSession(ctx, signalsdb.LensUpsertSessionParams{
		ID:          sid,
		ChannelID:   channelID,
		AnonID:      b.AnonID,
		CustomerID:  cust,
		StartedAt:   analytics.ClientTime(time.UnixMilli(firstTS), now),
		LastEventAt: analytics.ClientTime(time.UnixMilli(lastTS), now),
		EventCount:  int64(len(stamped)),
		EntryPage:   page,
		ExitPage:    page,
		UserAgent:   cut(userAgent, maxUserAgent),
		Viewport:    viewport,
	})
	if err != nil || n == 0 {
		return
	}
	if s.blobs == nil {
		return
	}
	key := chunkBlobKey(sid, b.Seq)
	if err := s.blobs.PutBytes(ctx, key, buf.Bytes(), "application/octet-stream"); err != nil {
		return // best-effort: drop the chunk rather than insert a row pointing at nothing
	}
	_ = s.q.LensInsertChunk(ctx, signalsdb.LensInsertChunkParams{
		SessionID:  sid,
		Seq:        b.Seq,
		BlobKey:    key,
		EventCount: int32(len(stamped)),
		FirstTs:    firstTS,
		LastTs:     lastTS,
	})
}

func (s *Service) replayMaxMinutes(ctx context.Context, channelID uuid.UUID) int64 {
	if s.cfg == nil {
		return defaultReplayMaxMinutes
	}
	if v, err := s.cfg.Int(ctx, cfgReplayMaxMinutes, &channelID); err == nil && v >= 0 {
		return v
	}
	return defaultReplayMaxMinutes
}

// --- session replay reads (dashboard) ---

// One row in a recordings list - enough to label and open a replay
type SessionMeta struct {
	ID         uuid.UUID
	AnonID     string
	CustomerID *uuid.UUID
	StartedAt  time.Time
	EndedAt    time.Time
	DurationMs int64
	EventCount int64
	EntryPage  string
	ExitPage   string
	Viewport   string
	UserAgent  string
	RageCount  int64 // frustration signals tied to this recording (for the badge/filter)
	ErrorCount int64
}

// SessionMeta plus the owning channel, for the player's permission check before streaming events
type SessionInfo struct {
	SessionMeta
	ChannelID uuid.UUID
}

func sessionMeta(r signalsdb.LensSession) SessionMeta {
	m := SessionMeta{
		ID: r.ID, AnonID: r.AnonID, StartedAt: r.StartedAt, EndedAt: r.LastEventAt,
		DurationMs: r.LastEventAt.Sub(r.StartedAt).Milliseconds(), EventCount: r.EventCount,
		EntryPage: r.EntryPage, ExitPage: r.ExitPage, Viewport: r.Viewport, UserAgent: r.UserAgent,
		RageCount: int64(r.RageCount), ErrorCount: int64(r.ErrorCount),
	}
	if r.CustomerID.Valid {
		c := r.CustomerID.UUID
		m.CustomerID = &c
	}
	return m
}

func sessionMetas(rows []signalsdb.LensSession) []SessionMeta {
	out := make([]SessionMeta, len(rows))
	for i, r := range rows {
		out[i] = sessionMeta(r)
	}
	return out
}

// Resolved by customer id OR any anon id they later identified with, so pre-login sessions show up too
func (s *Service) SessionsForCustomer(ctx context.Context, customerID uuid.UUID, channels []uuid.UUID, limit int32) ([]SessionMeta, error) {
	// the identity map lives on the spine engine - a direct signalsdb read misses every stitch made after a ClickHouse flip, and an error surfaces rather than silently dropping pre-login sessions
	anonIDs, err := s.rec.AnonIDsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.read().LensSessionsForCustomer(ctx, signalsdb.LensSessionsForCustomerParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true}, AnonIds: anonIDs,
		AllChannels: channels == nil, ChannelIds: emptyIDs(channels), Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	return sessionMetas(rows), nil
}

func (s *Service) SessionsForAnon(ctx context.Context, anonID string, channels []uuid.UUID, limit int32) ([]SessionMeta, error) {
	rows, err := s.read().LensSessionsForAnon(ctx, signalsdb.LensSessionsForAnonParams{
		AnonID: anonID, AllChannels: channels == nil, ChannelIds: emptyIDs(channels), Lim: limit,
	})
	if err != nil {
		return nil, err
	}
	return sessionMetas(rows), nil
}

func emptyIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

func (s *Service) SessionsForChannel(ctx context.Context, channelID uuid.UUID, limit int32) ([]SessionMeta, error) {
	rows, err := s.read().LensSessionsForChannel(ctx, signalsdb.LensSessionsForChannelParams{ChannelID: channelID, Lim: limit})
	if err != nil {
		return nil, err
	}
	return sessionMetas(rows), nil
}

func (s *Service) GetSession(ctx context.Context, id uuid.UUID) (SessionInfo, error) {
	r, err := s.read().LensGetSession(ctx, id)
	if err != nil {
		return SessionInfo{}, err
	}
	return SessionInfo{
		SessionMeta: sessionMeta(r),
		ChannelID:   r.ChannelID,
	}, nil
}

// Reassembles a recording into one ordered JSON array of rrweb events - every chunk gunzipped and concatenated in seq order, ready for the dashboard's player
func (s *Service) SessionEvents(ctx context.Context, id uuid.UUID) ([]byte, error) {
	chunks, err := s.read().LensSessionChunks(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errors.New("lens: no object store, which is where replay chunks live")
	}
	all := []json.RawMessage{}
	read := 0
	for _, key := range chunks {
		gz, err := s.blobs.GetBytes(ctx, key)
		if err != nil {
			continue
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(zr, int64(maxReplayReadBytes-read)+1))
		_ = zr.Close()
		if err != nil {
			continue
		}
		if read += len(raw); read > maxReplayReadBytes {
			break // the player gets the recording's opening, never enough to exhaust the process
		}
		var evs []json.RawMessage
		if err := json.Unmarshal(raw, &evs); err != nil {
			continue
		}
		all = append(all, evs...)
	}
	return json.Marshal(all)
}
