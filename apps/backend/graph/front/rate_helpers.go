package front

import (
	"context"
	"errors"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
)

var ErrRateLimited = errors.New("too many requests; please try again later") // generic on purpose - the storefront shows a "try again later" notice, not the operator's quota policy

func init() {
	apperr.RegisterSentinel(ErrRateLimited, "RATE_LIMITED", apperr.ClassRateLimited) // without the sentinel the presenter scrubs this as INTERNAL instead of classing it TOO_MANY_REQUESTS
}

// Valkey-backed in production so counters are shared across pods, and the limiter fails open on store errors - a transient outage shouldn't lock out legitimate customers
// Allows outright when nothing is wired: no limiter (a boot path that wires none, tests), no HTTP request on context (CLI), or an empty resolved IP (proxy hasn't set X-Forwarded-For) - all deployment misconfigurations, left to the observability layer rather than the hot path
func (r *Resolver) checkRate(ctx context.Context, op string) error {
	return rateByIP(ctx, r.RateLimiter, op)
}

func (r *Resolver) checkAuthRate(ctx context.Context, op string) error { // tighter AuthLimiter for credential surfaces (login / register / password reset), falling back to the general one when it isn't wired
	lim := r.AuthLimiter
	if lim == nil {
		lim = r.RateLimiter
	}
	return rateByIP(ctx, lim, op)
}

// Keys on something the caller supplies (the login email, say) rather than the IP, so a botnet rotating source addresses still can't stuff one account
func (r *Resolver) checkAuthRateKey(ctx context.Context, op, key string) error {
	lim := r.AuthLimiter
	if lim == nil {
		lim = r.RateLimiter
	}
	if lim == nil || key == "" {
		return nil
	}
	if !lim.Allow(ctx, op+":"+key) {
		return ErrRateLimited
	}
	return nil
}

func rateByIP(ctx context.Context, lim ratelimit.Limiter, op string) error {
	if lim == nil {
		return nil
	}
	req, ok := auth.RequestFromContext(ctx)
	if !ok {
		return nil
	}
	ip := auth.ClientIP(req)
	if ip == "" {
		return nil
	}
	if !lim.Allow(ctx, op+":"+ip) {
		return ErrRateLimited
	}
	return nil
}

// Catalog reads (product / browseProducts / searchProducts) are the storefront's hottest surface and normally must NEVER be gated, but they're also what a scraper or L7 flood hammers - general/security/catalog_query_throttle is per-channel requests/min per IP, default 0 = off, flipped on from Settings mid-attack with no deploy, and all three queries share one per-IP budget
// A THROTTLE, not a rate limit: past the soft budget requests are SLOWED by a growing delay capped at throttleMaxDelay, so a shopper sharing a NAT with a scraper gets a slow page and never an error - only past throttleRejectMult x budget, which means seconds of ignored backpressure and therefore not a browser, does it refuse with RATE_LIMITED

const (
	catalogRatePath    = "general/security/catalog_query_throttle"
	catalogRateMemoTTL = 15 * time.Second

	throttleDelayStep  = 250 * time.Millisecond
	throttleMaxDelay   = 2500 * time.Millisecond
	throttleRejectMult = 3
)

type catalogRateEntry struct {
	rate    int
	expires time.Time
}

type counterLimiter interface { // both ratelimit implementations satisfy this - asserted at the call site so the Limiter interface and its other implementers stay untouched
	Take(ctx context.Context, key string) (int64, bool)
}

func throttleDelay(n int64, soft int) (delay time.Duration, reject bool) { // kept pure so it's unit-tested apart from the transport plumbing
	if n <= int64(soft) {
		return 0, false
	}
	if n > int64(soft)*throttleRejectMult {
		return 0, true
	}
	d := min(time.Duration(n-int64(soft))*throttleDelayStep, throttleMaxDelay)
	return d, false
}

func (r *Resolver) catalogQueryRate(ctx context.Context) int { // memoized per channel for catalogRateMemoTTL - the hot path can't pay a config read per request, and ~15s staleness is fine for an emergency knob
	key := "default"
	scope := config.DefaultScope
	if ch, ok := channel.FromContext(ctx); ok && ch != nil {
		key = ch.ID.String()
		cid := ch.ID
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &cid}
	}
	if v, ok := r.catalogRate.Load(key); ok {
		if m, ok2 := v.(catalogRateEntry); ok2 && time.Now().Before(m.expires) {
			return m.rate
		}
	}
	rate := 0
	if r.ConfigSvc != nil {
		if n, err := r.ConfigSvc.GetInt(ctx, catalogRatePath, scope); err == nil && n > 0 {
			rate = int(n)
		}
	}
	r.catalogRate.Store(key, catalogRateEntry{rate: rate, expires: time.Now().Add(catalogRateMemoTTL)})
	return rate
}

// The render cache sits in front of these resolvers, so hits never reach here and only the expensive misses get metered - and the delay honors context cancellation so a hung-up client doesn't pin the goroutine
func (r *Resolver) checkCatalogRate(ctx context.Context) error {
	soft := r.catalogQueryRate(ctx)
	if soft <= 0 {
		return nil
	}
	lim, ok := r.RateLimiter.(counterLimiter)
	if !ok {
		return nil
	}
	req, ok := auth.RequestFromContext(ctx)
	if !ok {
		return nil
	}
	ip := auth.ClientIP(req)
	if ip == "" {
		return nil
	}
	n, ok := lim.Take(ctx, "catalogq:"+ip)
	if !ok {
		return nil // limiter backend down: fail open, no pressure applied
	}
	delay, reject := throttleDelay(n, soft)
	if reject {
		if n == int64(soft)*throttleRejectMult+1 && r.Bus != nil { // the shared counter hands each value to exactly one request, so only the boundary one signals ops - once per window, deduped further downstream
			ev := &ratelimit.ThrottleTripped{IP: ip, Surface: "catalog", Count: n, Budget: soft}
			if ch, chOK := channel.FromContext(ctx); chOK && ch != nil {
				ev.ChannelID = ch.ID
			}
			r.Bus.Publish(ctx, ev)
		}
		return ErrRateLimited
	}
	if delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}
