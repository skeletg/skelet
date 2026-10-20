package revalidate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sync"
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

// Purges storefront cache tags and paths through each storefront's /api/revalidate and records every purge - a nil Client is a safe no-op
type Client struct {
	url      string
	secret   string
	http     *http.Client
	store    Store
	channels ChannelLister

	mu      sync.Mutex
	targets []target
	at      time.Time

	beforePurge func(ctx context.Context, surface string)
}

type StorefrontChannel struct {
	ID        uuid.UUID
	PublicURL string
	IsActive  bool
}

type ChannelLister interface {
	StorefrontChannels(ctx context.Context) ([]StorefrontChannel, error)
}

type target struct {
	url      string
	channels []string
}

const (
	revalidatePath  = "/api/revalidate"
	targetsTTL      = 30 * time.Second
	channelsPerPost = 8
	postConcurrency = 4
)

var sendBudget = 90 * time.Second

func New(url, secret string, store Store) *Client {
	// Generous timeout - a rebuild request blocks while the storefront re-renders the pages - Plain invalidations return immediately, so this only matters for on-demand rebuilds
	return &Client{url: url, secret: secret, http: &http.Client{Timeout: 30 * time.Second}, store: store}
}

// With a configured URL that one storefront pool serves every active channel; without one each channel is reached at its public URL
func (c *Client) SetChannels(l ChannelLister) { c.channels = l }

// Runs before every purge or rebuild reaches a storefront, with the purge's surface - "" means every surface
func (c *Client) SetBeforePurge(fn func(ctx context.Context, surface string)) { c.beforePurge = fn }

func (c *Client) notePurge(ctx context.Context, surface string) {
	if c.beforePurge != nil {
		c.beforePurge(ctx, surface)
	}
}

func storefrontEndpoint(publicURL string) string {
	u, err := url.Parse(publicURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if host == "localhost" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	return u.Scheme + "://" + u.Host + revalidatePath
}

func buildTargets(channels []StorefrontChannel, pool string) []target {
	byURL := map[string][]string{}
	for _, ch := range channels {
		if !ch.IsActive {
			continue
		}
		u := pool
		if u == "" {
			u = storefrontEndpoint(ch.PublicURL)
		}
		if u == "" {
			continue
		}
		byURL[u] = append(byURL[u], ch.ID.String())
	}
	out := make([]target, 0, len(byURL))
	for u, ids := range byURL {
		slices.Sort(ids)
		out = append(out, target{url: u, channels: ids})
	}
	slices.SortFunc(out, func(a, b target) int {
		switch {
		case a.url < b.url:
			return -1
		case a.url > b.url:
			return 1
		}
		return 0
	})
	return out
}

func (c *Client) resolveTargets(ctx context.Context) []target {
	legacy := func() []target {
		if c.url == "" {
			return nil
		}
		return []target{{url: c.url}}
	}
	if c.channels == nil {
		return legacy()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.targets != nil && time.Since(c.at) < targetsTTL {
		return c.targets
	}
	chs, err := c.channels.StorefrontChannels(ctx)
	if err != nil {
		log.Printf("[revalidate] list channels: %v", err)
		if c.targets != nil {
			return c.targets
		}
		return legacy()
	}
	c.targets, c.at = buildTargets(chs, c.url), time.Now()
	return c.targets
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
	c.notePurge(ctx, surface)
	if _, err := c.purgeThenWarm(ctx, tags, paths, warm); err != nil {
		log.Printf("[revalidate] %v", err)
	}
}

// A purge only lands once its request ends, so a warm sent alongside it would re-read the stale page - the warm goes separately
func (c *Client) purgeThenWarm(ctx context.Context, tags, paths, warm []string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, sendBudget)
	defer cancel()
	if len(tags) > 0 || len(paths) > 0 {
		if _, err := c.postAll(ctx, tags, paths, nil); err != nil {
			return 0, err
		}
	}
	if len(warm) == 0 {
		return 0, nil
	}
	return c.postAll(ctx, nil, nil, warm)
}

func splitTargets(targets []target) []target {
	var out []target
	for _, t := range targets {
		if len(t.channels) <= channelsPerPost {
			out = append(out, t)
			continue
		}
		for chunk := range slices.Chunk(t.channels, channelsPerPost) {
			out = append(out, target{url: t.url, channels: chunk})
		}
	}
	return out
}

func (c *Client) postAll(ctx context.Context, tags, paths, warm []string) (int, error) {
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		warmed int
		errs   []error
	)
	sem := make(chan struct{}, postConcurrency)
	for _, t := range splitTargets(c.resolveTargets(ctx)) {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			n, err := c.post(ctx, t, tags, paths, warm)
			if err != nil && len(t.channels) > 0 {
				err = fmt.Errorf("channels %v: %w", t.channels, err)
			}
			mu.Lock()
			defer mu.Unlock()
			warmed += n
			if err != nil {
				errs = append(errs, err)
			}
		}()
	}
	wg.Wait()
	return warmed, errors.Join(errs...)
}

// Sends one revalidate request and returns how many warm paths were re-rendered - the HTTP wire layer shared by send() and warmPath()
func (c *Client) post(ctx context.Context, t target, tags, paths, warm []string) (int, error) {
	payload := map[string]any{"tags": tags, "paths": paths, "warm": warm}
	if len(t.channels) > 0 {
		payload["channels"] = t.channels
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Revalidate-Secret", c.secret)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("POST %s: %w", t.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("%s returned HTTP %d", t.url, resp.StatusCode)
	}
	var out struct {
		Warmed int `json:"warmed"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Warmed, nil
}

// Invalidates the tags and path then warms the path, returning an error when it didn't complete so the queue worker can retry - a nil client or no storefront is a no-op success
func (c *Client) warmPath(ctx context.Context, surface string, tags []string, path string) error {
	if c == nil || len(c.resolveTargets(ctx)) == 0 {
		return nil
	}
	c.notePurge(ctx, surface)
	warmed, err := c.purgeThenWarm(ctx, tags, []string{path}, []string{path})
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
