package revalidate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type LogEntry struct {
	Surface string
	Tags    []string
	Paths   []string
	Reason  string
	ActorID *uuid.UUID
}

type LoggedPurge struct {
	ID        uuid.UUID
	Surface   string
	Tags      []string
	Paths     []string
	Reason    string
	ActorID   *uuid.UUID
	CreatedAt time.Time
}

// Persists and reads back purge activity - optional, nil meaning no log
type Store interface {
	Record(ctx context.Context, e LogEntry) error
	List(ctx context.Context, limit int) ([]LoggedPurge, error)
}

// Invalidates storefront cache tags and paths through the storefront's /api/revalidate endpoint, recording each purge to the activity log
// A nil Client is a safe no-op, and an empty URL skips the HTTP call but still logs, so dev and test without a storefront still record intent and the admin UI works
type Client struct {
	url    string
	secret string
	http   *http.Client
	store  Store
}

func New(url, secret string, store Store) *Client {
	// Generous timeout - a rebuild request blocks while the storefront re-renders the pages - Plain invalidations return immediately, so this only matters for on-demand rebuilds
	return &Client{url: url, secret: secret, http: &http.Client{Timeout: 30 * time.Second}, store: store}
}

func (c *Client) Log(ctx context.Context, limit int) ([]LoggedPurge, error) {
	if c == nil || c.store == nil {
		return nil, nil
	}
	return c.store.List(ctx, limit)
}

// Marks tags and paths stale so pages rebuild lazily on the next request - best-effort, with HTTP and record failures logged rather than fatal
func (c *Client) Purge(ctx context.Context, surface string, tags, paths []string, reason string, actor *uuid.UUID) {
	c.send(ctx, surface, tags, paths, nil, reason, actor)
}

// Invalidates the given tags and paths AND asks the storefront to warm the `warm` paths immediately, re-rendering and re-caching now, so the page is fresh without waiting for the next visitor
func (c *Client) Rebuild(ctx context.Context, surface string, tags, paths, warm []string, reason string, actor *uuid.UUID) {
	c.send(ctx, surface, tags, paths, warm, reason, actor)
}

func (c *Client) send(ctx context.Context, surface string, tags, paths, warm []string, reason string, actor *uuid.UUID) {
	if c == nil || (len(tags) == 0 && len(paths) == 0 && len(warm) == 0) {
		return
	}
	if c.store != nil {
		if err := c.store.Record(ctx, LogEntry{Surface: surface, Tags: tags, Paths: paths, Reason: reason, ActorID: actor}); err != nil {
			log.Printf("[revalidate] record: %v", err)
		}
	}
	if c.url == "" {
		return
	}
	if _, err := c.post(ctx, tags, paths, warm); err != nil {
		log.Printf("[revalidate] %v", err)
	}
}

// Sends one revalidate request and returns how many warm paths were re-rendered - the HTTP wire layer shared by send() and warmPath()
func (c *Client) post(ctx context.Context, tags, paths, warm []string) (int, error) {
	body, err := json.Marshal(map[string]any{"tags": tags, "paths": paths, "warm": warm})
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Revalidate-Secret", c.secret)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("POST %s: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("%s returned HTTP %d", c.url, resp.StatusCode)
	}
	var out struct {
		Warmed int `json:"warmed"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Warmed, nil
}

// Invalidates the tags and path then warms the path, returning an error when it didn't complete so the queue worker can retry - a nil client or unconfigured URL is a no-op success
func (c *Client) warmPath(ctx context.Context, tags []string, path string) error {
	if c == nil || c.url == "" {
		return nil
	}
	warmed, err := c.post(ctx, tags, []string{path}, []string{path})
	if err != nil {
		return err
	}
	if warmed < 1 {
		return fmt.Errorf("warm did not complete for %s", path)
	}
	return nil
}

// Busts an entire registered surface through its AllTag - a no-op for surfaces without one, which are per-item only by design and so can never be invalidated catalog-wide
func (c *Client) PurgeSurface(ctx context.Context, key, reason string, actor *uuid.UUID) {
	if s, ok := Lookup(key); ok && s.AllTag != "" {
		c.Purge(ctx, key, []string{s.AllTag}, nil, reason, actor)
	}
}

// Busts every bulk-invalidatable surface, those with an AllTag, plus the homepage path
// Surfaces without an AllTag are deliberately excluded, so "invalidate everything" can't stampede a large catalog
func (c *Client) PurgeAll(ctx context.Context, reason string, actor *uuid.UUID) {
	var tags []string
	for _, s := range Surfaces() {
		if s.AllTag != "" {
			tags = append(tags, s.AllTag)
		}
	}
	c.Purge(ctx, "", tags, []string{"/"}, reason, actor)
}
