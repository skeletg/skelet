// Package aiusage counts what the storefront assistant costs and what it sells: turns and tokens per store per day, and the orders whose cart an assistant or a shopper's agent filled
package aiusage

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

const (
	priceInputPath      = "ai/usage/price_input"
	priceOutputPath     = "ai/usage/price_output"
	priceCacheReadPath  = "ai/usage/price_cache_read"
	priceCacheWritePath = "ai/usage/price_cache_write"
	MaxDays             = signalsapi.MaxReportDays
)

func init() {
	config.RegisterGroup(config.Group{Section: "ai", Slug: "usage", Label: "Usage pricing", SortOrder: 2})
	config.Register(config.Definition{Path: priceInputPath, Type: config.TypeString, Label: "Input tokens, USD per million",
		Description: "What your AI provider charges, for the shopping assistant's cost estimate on the dashboard. Leave it or the output price blank to show tokens only."})
	config.Register(config.Definition{Path: priceOutputPath, Type: config.TypeString, Label: "Output tokens, USD per million"})
	config.Register(config.Definition{Path: priceCacheReadPath, Type: config.TypeString, Label: "Cached input reads, USD per million",
		Description: "Blank prices them like input tokens. Most providers charge less for a cached read - enter your provider's price for a closer estimate."})
	config.Register(config.Definition{Path: priceCacheWritePath, Type: config.TypeString, Label: "Cache writes, USD per million",
		Description: "Blank prices them like input tokens. Enter your provider's price where writing to its cache costs more."})
}

type Settings interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

type Turn struct {
	Failed      bool
	Unanswered  bool // nothing said and shown, cut short, every search empty, or the model said it didn't know
	CartChanged bool
	Checkout    bool
	Usage       ai.Usage
}

type Orders struct {
	CurrencyCode     string
	All              int32
	Assistant        int32
	AssistantRevenue int64
	Agent            int32
	AgentRevenue     int64
}

type Report struct {
	Days            int
	Turns           int64
	Overviews       int64 // search overviews written, their tokens in Usage with the turns'
	FailedTurns     int64
	UnansweredTurns int64
	CartTurns       int64
	CheckoutTurns   int64
	Usage           ai.Usage
	CostUSD         *float64 // nil until the prices are set
	Orders          []Orders
	Daily           []Day // every day of the window, oldest first, a quiet one at zero
}

// Day is one UTC day of the window
type Day struct {
	Day             time.Time
	Turns           int64
	FailedTurns     int64
	UnansweredTurns int64
	CartTurns       int64
	CheckoutTurns   int64
	AssistantOrders int32
	AgentOrders     int32
}

type Service struct {
	q      *db.Queries
	cfg    Settings
	warned sync.Map // the price settings already reported unusable, so a dashboard reload doesn't log them again
}

func New(q *db.Queries, cfg Settings) *Service { return &Service{q: q, cfg: cfg} }

func (s *Service) RecordTurn(ctx context.Context, channelID uuid.UUID, t Turn) error {
	err := s.q.RecordAssistantTurn(ctx, db.RecordAssistantTurnParams{
		ChannelID: channelID, FailedTurns: flag(t.Failed), UnansweredTurns: flag(t.Unanswered), CartTurns: flag(t.CartChanged), CheckoutTurns: flag(t.Checkout),
		InputTokens: t.Usage.Input, OutputTokens: t.Usage.Output, CacheReadTokens: t.Usage.CacheRead, CacheWriteTokens: t.Usage.CacheWrite,
	})
	if err != nil {
		return fmt.Errorf("record assistant turn: %w", err)
	}
	return nil
}

// Every attempt's tokens count, but only one that wrote something counts as an overview
func (s *Service) RecordOverview(ctx context.Context, channelID uuid.UUID, used ai.Usage, written bool) error {
	var n int32
	if written {
		n = 1
	}
	err := s.q.RecordAssistantOverview(ctx, db.RecordAssistantOverviewParams{ChannelID: channelID, Overviews: n,
		InputTokens: used.Input, OutputTokens: used.Output, CacheReadTokens: used.CacheRead, CacheWriteTokens: used.CacheWrite})
	if err != nil {
		return fmt.Errorf("record search overview: %w", err)
	}
	return nil
}

func (s *Service) TokensToday(ctx context.Context, channelID uuid.UUID) (int64, error) {
	n, err := s.q.AssistantTokensToday(ctx, channelID)
	if err != nil {
		return 0, fmt.Errorf("assistant tokens today: %w", err)
	}
	return n, nil
}

func (s *Service) Report(ctx context.Context, channelID uuid.UUID, days int) (Report, error) {
	days = min(max(days, 1), MaxDays)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	since := today.AddDate(0, 0, 1-days)
	u, err := s.q.AssistantUsageSince(ctx, db.AssistantUsageSinceParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Report{}, fmt.Errorf("assistant usage: %w", err)
	}
	rows, err := s.q.AssistedOrdersSince(ctx, db.AssistedOrdersSinceParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Report{}, fmt.Errorf("assisted orders: %w", err)
	}
	r := Report{
		Days: days, Turns: u.Turns, Overviews: u.Overviews, FailedTurns: u.FailedTurns, UnansweredTurns: u.UnansweredTurns, CartTurns: u.CartTurns, CheckoutTurns: u.CheckoutTurns,
		Usage:  ai.Usage{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadTokens, CacheWrite: u.CacheWriteTokens},
		Orders: make([]Orders, len(rows)),
	}
	for i, row := range rows {
		r.Orders[i] = Orders{CurrencyCode: row.CurrencyCode, All: row.OrderCount, Assistant: row.AssistantOrders, AssistantRevenue: row.AssistantRevenue,
			Agent: row.AgentOrders, AgentRevenue: row.AgentRevenue}
	}
	if p, ok := s.prices(ctx); ok {
		cost := p.cost(r.Usage)
		r.CostUSD = &cost
	}
	if r.Daily, err = s.daily(ctx, channelID, since, days); err != nil {
		return Report{}, err
	}
	return r, nil
}

func (s *Service) daily(ctx context.Context, channelID uuid.UUID, since time.Time, days int) ([]Day, error) {
	usage, err := s.q.AssistantUsageDaily(ctx, db.AssistantUsageDailyParams{ChannelID: channelID, Since: since})
	if err != nil {
		return nil, fmt.Errorf("assistant usage by day: %w", err)
	}
	orders, err := s.q.AssistedOrdersDaily(ctx, db.AssistedOrdersDailyParams{ChannelID: channelID, Since: since})
	if err != nil {
		return nil, fmt.Errorf("assisted orders by day: %w", err)
	}
	out := make([]Day, days)
	index := make(map[string]int, days)
	for i := range out {
		out[i].Day = since.AddDate(0, 0, i)
		index[out[i].Day.Format(time.DateOnly)] = i
	}
	for _, u := range usage {
		if i, ok := index[u.Day.UTC().Format(time.DateOnly)]; ok {
			out[i].Turns, out[i].FailedTurns, out[i].UnansweredTurns, out[i].CartTurns, out[i].CheckoutTurns = u.Turns, u.FailedTurns, u.UnansweredTurns, u.CartTurns, u.CheckoutTurns
		}
	}
	for _, o := range orders {
		if i, ok := index[o.Day.UTC().Format(time.DateOnly)]; ok {
			out[i].AssistantOrders, out[i].AgentOrders = o.AssistantOrders, o.AgentOrders
		}
	}
	return out, nil
}

// PricesSet says whether the cost estimate has the input and output prices it needs
func (s *Service) PricesSet(ctx context.Context) bool {
	_, ok := s.prices(ctx)
	return ok
}

type prices struct{ input, output, cacheRead, cacheWrite float64 }

func (p prices) cost(u ai.Usage) float64 {
	return (float64(u.Input)*p.input + float64(u.Output)*p.output + float64(u.CacheRead)*p.cacheRead + float64(u.CacheWrite)*p.cacheWrite) / 1e6
}

func (s *Service) prices(ctx context.Context) (prices, bool) {
	in, okIn := s.price(ctx, priceInputPath)
	out, okOut := s.price(ctx, priceOutputPath)
	if !okIn || !okOut {
		return prices{}, false
	}
	p := prices{input: in, output: out, cacheRead: in, cacheWrite: in}
	if v, ok := s.price(ctx, priceCacheReadPath); ok {
		p.cacheRead = v
	}
	if v, ok := s.price(ctx, priceCacheWritePath); ok {
		p.cacheWrite = v
	}
	return p, true
}

// A blank price is the operator's choice; one that can't be read or isn't a number is a mistake worth a log line, once
func (s *Service) price(ctx context.Context, path string) (float64, bool) {
	raw, err := s.cfg.GetString(ctx, path, config.DefaultScope)
	if err != nil {
		s.warn(ctx, path, "", err)
		return 0, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimPrefix(raw, "$"), 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		s.warn(ctx, path, raw, fmt.Errorf("not a price in USD: %q", raw))
		return 0, false
	}
	return v, true
}

func (s *Service) warn(ctx context.Context, path, raw string, err error) {
	if _, seen := s.warned.LoadOrStore(path+"="+raw, true); !seen {
		slog.WarnContext(ctx, "aiusage: price setting unusable, so the cost estimate is off", "path", path, "err", err)
	}
}

func flag(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
