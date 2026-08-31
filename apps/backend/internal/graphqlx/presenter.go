package graphqlx

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/applog"
	"github.com/skeletg/skelet/apps/backend/internal/httpx"
)

// What the front surface returns when ClassInternal is in play - a frozen string, so an attacker can't fingerprint Go internals by diffing error messages
const genericInternalMessage = "internal error"

// A surface-tailored gqlgen ErrorPresenterFunc - it resolves the error's Code and Class, mapping unknown to ClassInternal so nothing leaks as Unknown, stamps extensions {code, class, request_id} plus op and origin on admin only, scrubs the message for ClassInternal on front, and logs every error as a structured line
func Presenter(s Surface, opts ...Option) graphql.ErrorPresenterFunc {
	cfg := resolveOptions(opts)
	return func(ctx context.Context, err error) *gqlerror.Error {
		gqlErr := graphql.DefaultErrorPresenter(ctx, err)
		if gqlErr == nil {
			return nil
		}

		code := apperr.CodeOf(err)
		class := apperr.ClassOf(err)
		if class == apperr.ClassUnknown && errors.Is(err, sql.ErrNoRows) {
			// an unmapped ErrNoRows is a lookup miss, not a server fault - repos that map it to a domain
			// sentinel take precedence via ClassOf above, this is the net under the ones that don't
			class = apperr.ClassNotFound
			if code == "" {
				code = "NOT_FOUND"
			}
		}
		if class == apperr.ClassUnknown {
			class = apperr.ClassInternal
		}
		if code == "" && class == apperr.ClassInternal {
			code = "INTERNAL"
		}

		var op, origin string
		if ae, ok := apperr.AsAppError(err); ok {
			op = ae.Op
			if file, line := ae.Source(); line > 0 {
				origin = filepath.Base(file) + ":" + strconv.Itoa(line)
			}
		}

		if gqlErr.Extensions == nil {
			gqlErr.Extensions = make(map[string]any, 5)
		}
		if code != "" {
			gqlErr.Extensions["code"] = code
		}
		gqlErr.Extensions["class"] = class.String()
		if rid := httpx.RequestIDFromContext(ctx); rid != "" {
			gqlErr.Extensions["request_id"] = rid
		}

		// Dev-only "why forbidden" details - the auth guards return a denial type implementing this anonymous interface, and probing it here keeps graphqlx free of an auth import
		if cfg.devDetails {
			var denial interface {
				DenialDetails() (slug, channelID, locale string)
			}
			if errors.As(err, &denial) {
				slug, channelID, locale := denial.DenialDetails()
				if slug != "" {
					gqlErr.Extensions["denied_permission"] = slug
				}
				if channelID != "" {
					gqlErr.Extensions["denied_channel_id"] = channelID
				}
				if locale != "" {
					gqlErr.Extensions["denied_locale"] = locale
				}
			}
		}

		// Captured BEFORE the front-surface scrub below, which rewrites gqlErr.Message in place - and gqlErr IS err, so logging err afterwards recorded the sanitised "internal error" and threw the cause away, leaving server faults undiagnosable from logs alone
		logMessage := err.Error()

		switch s {
		case SurfaceAdmin:
			if op != "" {
				gqlErr.Extensions["op"] = op
			}
			if origin != "" {
				gqlErr.Extensions["origin"] = origin
			}
		case SurfaceFront:
			// Scrub the message for server-side faults so we don't leak "pq: connection refused" or similar - validation, not-found and conflict messages are caller-actionable and stay verbatim
			if class == apperr.ClassInternal || class == apperr.ClassUnavailable {
				gqlErr.Message = genericInternalMessage
			}
		}

		// Error level ONLY for server faults - the slog-to-Sentry bridge forwards every Error record,
		// so logging validation/conflict at Error shipped each shopper typo to Sentry through the side
		// door the class-gated reporter below exists to close
		logLevel := applog.From(ctx).Warn
		if class == apperr.ClassInternal || class == apperr.ClassUnavailable {
			logLevel = applog.From(ctx).Error
		}
		logLevel(
			"graphql error",
			"surface", s.String(),
			"code", code,
			"class", class.String(),
			"op", op,
			"origin", origin,
			"path", gqlErr.Path.String(),
			"err", logMessage,
		)

		// External reporter only for server-side faults - validation, not-found and conflict are caller-correctable and would just spam the dedup index - The reporter may be nil when no aggregator is configured
		if cfg.reporter != nil && (class == apperr.ClassInternal || class == apperr.ClassUnavailable) {
			if ae, ok := apperr.AsAppError(err); ok {
				cfg.reporter.CaptureAppError(ctx, ae)
			} else {
				// Wrap so the reporter still has a typed Error with the class we just resolved - it pivots on Class when computing fingerprints
				cfg.reporter.CaptureAppError(ctx, apperr.WrapClass(err, class, op))
			}
		}

		return gqlErr
	}
}
