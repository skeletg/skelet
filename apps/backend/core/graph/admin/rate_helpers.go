package admin

import (
	"context"
	"errors"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

var ErrRateLimited = errors.New("too many requests; please try again later") // deliberately generic - the dashboard shows a "try again later" notice, not the operator's quota policy

func init() {
	apperr.RegisterSentinel(ErrRateLimited, "RATE_LIMITED", apperr.ClassRateLimited)
}

func (r *Resolver) authLimiter() ratelimit.Limiter { // the tighter credential limiter, falling back to the general one when it isn't wired
	if r.AuthLimiter != nil {
		return r.AuthLimiter
	}
	return r.RateLimiter
}

// checkAuthRate and checkAuthRateKey mirror checkRate and checkRateKey on the tighter AuthLimiter, for the admin login surface
func (r *Resolver) checkAuthRate(ctx context.Context, op string) error {
	lim := r.authLimiter()
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

func (r *Resolver) checkAuthRateKey(ctx context.Context, op, key string) error {
	lim := r.authLimiter()
	if lim == nil || key == "" {
		return nil
	}
	if !lim.Allow(ctx, op+":"+key) {
		return ErrRateLimited
	}
	return nil
}
