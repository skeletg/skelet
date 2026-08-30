package sitecrawl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

var ErrAlreadyRunning = errors.New("a crawl is already running for this channel")

const (
	flushEvery      = 40
	crawlTimeout    = 30 * time.Minute
	staleMinutes    = 10
	DefaultMaxPages = 2000
	MaxMaxPages     = 20000
)

type Service struct {
	queries *db.Queries
	log     *slog.Logger
	mu      sync.Mutex
	running map[uuid.UUID]context.CancelFunc
	// Called once a crawl lands so the caller can diff it against the previous one - kept as a hook rather than a
	// dependency because linkgraph already imports this package for path normalisation
	onComplete func(ctx context.Context, channelID, crawlID uuid.UUID)
}

func NewService(queries *db.Queries, log *slog.Logger) *Service {
	return &Service{queries: queries, log: log, running: map[uuid.UUID]context.CancelFunc{}}
}

type StartInput struct {
	ChannelID     uuid.UUID
	BaseURL       string
	MaxPages      int
	MaxDepth      int
	RespectRobots bool
	AdminID       *uuid.UUID
}

func (s *Service) Start(ctx context.Context, in StartInput) (db.SeoCrawl, error) {
	_ = s.queries.CancelStaleSeoCrawls(ctx, staleMinutes)
	if existing, err := s.queries.RunningSeoCrawl(ctx, in.ChannelID); err == nil {
		return existing, ErrAlreadyRunning
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.SeoCrawl{}, err
	}

	ch, err := s.queries.GetChannelByID(ctx, in.ChannelID)
	if err != nil {
		return db.SeoCrawl{}, err
	}
	base := strings.TrimSpace(in.BaseURL)
	if base == "" {
		base = ch.PublicUrl
	}
	if base == "" {
		return db.SeoCrawl{}, errors.New("this channel has no public url - set one or pass a base url")
	}
	if in.MaxPages <= 0 {
		in.MaxPages = DefaultMaxPages
	}
	if in.MaxPages > MaxMaxPages {
		in.MaxPages = MaxMaxPages
	}
	if in.MaxDepth <= 0 {
		in.MaxDepth = 12
	}

	crawler, err := New(Options{
		BaseURL:       base,
		HostHeader:    hostOverride(base, ch.PublicUrl),
		BypassToken:   os.Getenv("SEO_CRAWL_BYPASS_TOKEN"),
		MaxPages:      in.MaxPages,
		MaxDepth:      in.MaxDepth,
		RespectRobots: in.RespectRobots,
	})
	if err != nil {
		return db.SeoCrawl{}, err
	}

	locales, err := s.queries.ListChannelLocales(ctx, in.ChannelID)
	if err != nil {
		return db.SeoCrawl{}, err
	}
	prefixes := make([]string, 0, len(locales))
	for _, l := range locales {
		prefixes = append(prefixes, l.Code)
	}

	row, err := s.queries.CreateSeoCrawl(ctx, db.CreateSeoCrawlParams{
		ChannelID:     in.ChannelID,
		BaseUrl:       base,
		MaxPages:      int32(in.MaxPages),
		MaxDepth:      int32(in.MaxDepth),
		RespectRobots: in.RespectRobots,
		AdminID:       uuid.NullUUID{UUID: derefID(in.AdminID), Valid: in.AdminID != nil},
	})
	if err != nil {
		return db.SeoCrawl{}, err
	}

	// The crawl outlives the mutation that asked for it, so it gets its own context rather than the request's
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), crawlTimeout)
	s.mu.Lock()
	s.running[in.ChannelID] = cancel
	s.mu.Unlock()

	go func() {
		defer cancel()
		// started from a single admin click - a panic in the crawl must mark the run failed, not
		// take the server down
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("site crawl panicked", "channel", in.ChannelID, "crawl", row.ID, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				if err := s.queries.FinishSeoCrawl(context.WithoutCancel(ctx), db.FinishSeoCrawlParams{
					ID: row.ID, Status: "failed", Error: "crawl panicked - see server logs",
				}); err != nil {
					s.log.Error("site crawl finish after panic failed", "crawl", row.ID, "err", err)
				}
			}
		}()
		defer func() {
			s.mu.Lock()
			delete(s.running, in.ChannelID)
			s.mu.Unlock()
		}()
		s.run(runCtx, row, crawler, prefixes)
	}()
	return row, nil
}

// SetOnComplete installs the post-crawl hook, wired once at boot
func (s *Service) SetOnComplete(fn func(ctx context.Context, channelID, crawlID uuid.UUID)) {
	s.onComplete = fn
}

func (s *Service) Cancel(channelID uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel, ok := s.running[channelID]
	if ok {
		cancel()
	}
	return ok
}

func (s *Service) run(ctx context.Context, row db.SeoCrawl, crawler *Crawler, localePrefixes []string) {
	buf := newBuffer(row.ID)
	flush := func() error {
		if err := buf.flushPages(ctx, s.queries); err != nil {
			return err
		}
		return buf.flushLinks(ctx, s.queries)
	}

	rootStatus := 0
	run, crawlErr := crawler.Crawl(ctx, func(res Result) error {
		if res.Page.Path == "/" {
			rootStatus = res.Page.StatusCode
		}
		buf.add(res, localePrefixes)
		if buf.pending() >= flushEvery {
			if err := flush(); err != nil {
				return err
			}
			_ = s.queries.UpdateSeoCrawlProgress(ctx, db.UpdateSeoCrawlProgressParams{
				ID: row.ID, PagesCrawled: int32(buf.pages), LinksFound: int32(buf.links),
			})
		}
		return nil
	})

	status, message := "completed", ""
	if err := flush(); err != nil {
		status, message = "failed", err.Error()
	}
	if crawlErr != nil {
		status = "failed"
		if errors.Is(crawlErr, context.Canceled) {
			status = "cancelled"
		}
		message = crawlErr.Error()
	}
	// A crawl that could not fetch the front door found nothing and means nothing - reporting it as completed
	// leaves an operator staring at a one-page map wondering what happened
	if status == "completed" && (rootStatus == 0 || rootStatus >= 400) {
		status = "failed"
		message = fmt.Sprintf("the start url answered %d - nothing could be discovered from it", rootStatus)
	}
	if status == "completed" {
		if err := s.queries.ResolveSeoCrawlEntities(ctx, db.ResolveSeoCrawlEntitiesParams{
			ChannelID: row.ChannelID, CrawlID: row.ID,
		}); err != nil {
			status, message = "failed", err.Error()
		}
	}

	if err := s.queries.FinishSeoCrawl(ctx, db.FinishSeoCrawlParams{
		ID: row.ID, Status: status, Error: message, HitBudget: run.HitBudget,
		PagesCrawled: int32(run.Pages), LinksFound: int32(buf.links),
	}); err != nil {
		s.log.Error("site crawl finish failed", "crawl", row.ID, "err", err)
		return
	}
	if status == "completed" && s.onComplete != nil {
		s.onComplete(ctx, row.ChannelID, row.ID)
	}
}

// OptedInChannels crawls every active channel whose `wants` callback says yes, for the nightly job - a channel
// already crawling is skipped rather than queued, since a fresh run would measure the same site
func (s *Service) OptedInChannels(ctx context.Context, wants func(channelID uuid.UUID) (bool, int)) (started int, err error) {
	channels, err := s.queries.ListChannels(ctx)
	if err != nil {
		return 0, err
	}
	for _, ch := range channels {
		if !ch.IsActive || ch.PublicUrl == "" {
			continue
		}
		on, budget := wants(ch.ID)
		if !on {
			continue
		}
		_, err := s.Start(ctx, StartInput{ChannelID: ch.ID, MaxPages: budget, RespectRobots: true})
		switch {
		case errors.Is(err, ErrAlreadyRunning):
			continue
		case err != nil:
			s.log.Error("nightly site crawl failed to start", "channel", ch.ID, "err", err)
			continue
		}
		started++
	}
	return started, nil
}

func (s *Service) List(ctx context.Context, channelID uuid.UUID, limit int32) ([]db.SeoCrawl, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.queries.ListSeoCrawls(ctx, db.ListSeoCrawlsParams{ChannelID: channelID, RowLimit: limit})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.SeoCrawl, error) {
	return s.queries.GetSeoCrawl(ctx, id)
}

func derefID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.UUID{}
	}
	return *id
}

// hostOverride returns the public host to announce when the crawl dials a different address, and "" when they agree
func hostOverride(base, publicURL string) string {
	if publicURL == "" {
		return ""
	}
	dialed, err := url.Parse(base)
	if err != nil {
		return ""
	}
	public, err := url.Parse(publicURL)
	if err != nil || public.Host == "" || public.Host == dialed.Host {
		return ""
	}
	return public.Host
}

// LookupPath strips the query and any locale segment, leaving the shape url_rewrites stores
func LookupPath(path string, localePrefixes []string) string {
	clean, _, _ := strings.Cut(path, "?")
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return ""
	}
	head, rest, found := strings.Cut(clean, "/")
	for _, code := range localePrefixes {
		if !strings.EqualFold(head, code) {
			continue
		}
		if found {
			return rest
		}
		return ""
	}
	return clean
}
