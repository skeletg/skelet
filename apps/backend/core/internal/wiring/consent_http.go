package wiring

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
)

// The public consent links mailed to shoppers - an unsubscribe and a signup confirmation
// GET only ever shows a page with a button, because mail scanners and link previewers follow every link in a message, and a GET that acted would unsubscribe or subscribe people who never clicked

const unsubscribePath = "/unsubscribe/"

type consentWriter interface {
	SetConsent(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) (consent.Consent, error)
}

type signupConfirmer interface {
	ConfirmationPending(ctx context.Context, token string) (bool, error)
	Confirm(ctx context.Context, token string) error
}

type requestAllower func(r *http.Request) bool

var consentPage = template.Must(template.New("consent").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="robots" content="noindex"><title>{{.Title}}</title></head>
<body style="font-family:sans-serif;text-align:center;padding:48px 16px;">
<h1>{{.Title}}</h1><p>{{.Body}}</p>
{{if .Button}}<form method="post"><button type="submit" style="font-size:16px;padding:10px 24px;cursor:pointer;">{{.Button}}</button></form>{{end}}
</body></html>`))

func writeConsentPage(w http.ResponseWriter, status int, title, body, button string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = consentPage.Execute(w, struct{ Title, Body, Button string }{title, body, button})
}

func pageTooMany(w http.ResponseWriter) {
	writeConsentPage(w, http.StatusTooManyRequests, "Too many requests", "Please try again in a minute.", "")
}

func pageFailed(w http.ResponseWriter) {
	writeConsentPage(w, http.StatusInternalServerError, "Something went wrong", "We couldn't complete that just now. Please try the link again in a few minutes.", "")
}

// POST opts out, from the page's own button or from a mailbox provider's RFC 8058 one-click request (body List-Unsubscribe=One-Click), which needs no page
func unsubscribeHandler(verify func(string) (uuid.UUID, error), consents consentWriter, allow requestAllower) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, HEAD, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !allow(r) {
			pageTooMany(w)
			return
		}
		customerID, err := verify(strings.TrimPrefix(r.URL.Path, unsubscribePath))
		if err != nil {
			writeConsentPage(w, http.StatusBadRequest, "Link invalid", "This unsubscribe link is invalid or has expired.", "")
			return
		}
		if r.Method != http.MethodPost {
			writeConsentPage(w, http.StatusOK, "Unsubscribe", "Stop receiving marketing emails from us? Order and account emails will still arrive.", "Unsubscribe")
			return
		}
		if _, err := consents.SetConsent(r.Context(), customerID, consentkind.MarketingEmail, false, consent.SourceUnsubscribeLink); err != nil {
			slog.Error("unsubscribe: record opt-out", "customer", customerID, "err", err)
			pageFailed(w)
			return
		}
		writeConsentPage(w, http.StatusOK, "You're unsubscribed", "You will no longer receive marketing emails from us.", "")
	}
}

func signupConfirmHandler(forms signupConfirmer, allow requestAllower) http.HandlerFunc {
	invalid := func(w http.ResponseWriter) {
		writeConsentPage(w, http.StatusBadRequest, "Link invalid", "This confirmation link is invalid, already used or has expired. Sign up again to get a new one.", "")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, HEAD, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !allow(r) {
			pageTooMany(w)
			return
		}
		token := strings.TrimPrefix(r.URL.Path, customer.SignupConfirmPath)
		if r.Method != http.MethodPost {
			ok, err := forms.ConfirmationPending(r.Context(), token)
			switch {
			case err != nil:
				slog.Error("signup confirm: look up link", "err", err)
				pageFailed(w)
			case !ok:
				invalid(w)
			default:
				writeConsentPage(w, http.StatusOK, "Confirm your subscription", "Press the button to start receiving our emails. You can unsubscribe at any time.", "Confirm subscription")
			}
			return
		}
		if err := forms.Confirm(r.Context(), token); err != nil {
			if errors.Is(err, signupform.ErrConfirmationInvalid) {
				invalid(w)
				return
			}
			slog.Error("signup confirm: subscribe", "err", err)
			pageFailed(w)
			return
		}
		writeConsentPage(w, http.StatusOK, "You're subscribed", "Thanks for confirming - our emails are on their way.", "")
	}
}
