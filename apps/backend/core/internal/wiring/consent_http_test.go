package wiring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
)

type optOutLog struct {
	calls []consent.Change
	err   error
}

func (o *optOutLog) SetConsent(_ context.Context, id uuid.UUID, kind string, optedIn bool, source string) (consent.Consent, error) {
	o.calls = append(o.calls, consent.Change{CustomerID: id, Kind: kind, OptedIn: optedIn, Source: source})
	return consent.Consent{}, o.err
}

var unsubCustomer = uuid.New()

func verifyOnly(good string) func(string) (uuid.UUID, error) {
	return func(tok string) (uuid.UUID, error) {
		if tok != good {
			return uuid.Nil, errors.New("bad token")
		}
		return unsubCustomer, nil
	}
}

func allowAll(*http.Request) bool { return true }

func serveUnsub(h http.Handler, method, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, unsubscribePath+token, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUnsubscribeGetOnlyAsks(t *testing.T) {
	log := &optOutLog{}
	h := unsubscribeHandler(verifyOnly("tok"), log, allowAll)
	for _, m := range []string{http.MethodGet, http.MethodHead} {
		rec := serveUnsub(h, m, "tok", "")
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", m, rec.Code)
		}
	}
	if body := serveUnsub(h, http.MethodGet, "tok", "").Body.String(); !strings.Contains(body, `<form method="post">`) {
		t.Errorf("the GET page has no confirm button:\n%s", body)
	}
	if len(log.calls) != 0 {
		t.Errorf("a GET recorded %d consent changes, want 0", len(log.calls))
	}
}

func TestUnsubscribePostOptsOut(t *testing.T) {
	for name, body := range map[string]string{
		"the page's button":  "",
		"RFC 8058 one-click": "List-Unsubscribe=One-Click",
	} {
		t.Run(name, func(t *testing.T) {
			log := &optOutLog{}
			rec := serveUnsub(unsubscribeHandler(verifyOnly("tok"), log, allowAll), http.MethodPost, "tok", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST = %d, want 200", rec.Code)
			}
			want := consent.Change{CustomerID: unsubCustomer, Kind: consentkind.MarketingEmail, OptedIn: false, Source: consent.SourceUnsubscribeLink}
			if len(log.calls) != 1 || log.calls[0] != want {
				t.Errorf("consent writes = %+v, want exactly %+v", log.calls, want)
			}
		})
	}
}

func TestUnsubscribeRefusals(t *testing.T) {
	t.Run("bad token", func(t *testing.T) {
		log := &optOutLog{}
		rec := serveUnsub(unsubscribeHandler(verifyOnly("tok"), log, allowAll), http.MethodPost, "forged", "")
		if rec.Code != http.StatusBadRequest || len(log.calls) != 0 {
			t.Errorf("forged token = %d with %d writes, want 400 and none", rec.Code, len(log.calls))
		}
	})
	t.Run("rate limited", func(t *testing.T) {
		log := &optOutLog{}
		rec := serveUnsub(unsubscribeHandler(verifyOnly("tok"), log, func(*http.Request) bool { return false }), http.MethodPost, "tok", "")
		if rec.Code != http.StatusTooManyRequests || len(log.calls) != 0 {
			t.Errorf("over budget = %d with %d writes, want 429 and none", rec.Code, len(log.calls))
		}
	})
	t.Run("other methods", func(t *testing.T) {
		rec := serveUnsub(unsubscribeHandler(verifyOnly("tok"), &optOutLog{}, allowAll), http.MethodPut, "tok", "")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("PUT = %d, want 405", rec.Code)
		}
	})
}

func TestUnsubscribeWriteFailureIsAServerError(t *testing.T) {
	log := &optOutLog{err: errors.New("db down")}
	rec := serveUnsub(unsubscribeHandler(verifyOnly("tok"), log, allowAll), http.MethodPost, "tok", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("write failure = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "invalid") {
		t.Error("a database failure was reported as an invalid link")
	}
}

type confirmLog struct {
	pending    bool
	pendingErr error
	confirmErr error
	confirmed  []string
}

func (c *confirmLog) ConfirmationPending(context.Context, string) (bool, error) {
	return c.pending, c.pendingErr
}

func (c *confirmLog) Confirm(_ context.Context, token string) error {
	c.confirmed = append(c.confirmed, token)
	return c.confirmErr
}

func serveConfirm(h http.Handler, method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, customer.SignupConfirmPath+"tok", nil))
	return rec
}

func TestSignupConfirmGetOnlyAsks(t *testing.T) {
	c := &confirmLog{pending: true}
	rec := serveConfirm(signupConfirmHandler(c, allowAll), http.MethodGet)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<form method="post">`) {
		t.Errorf("GET = %d, want 200 with a confirm button", rec.Code)
	}
	if len(c.confirmed) != 0 {
		t.Error("a GET confirmed the subscription")
	}

	c.pending = false
	if rec := serveConfirm(signupConfirmHandler(c, allowAll), http.MethodGet); rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "<form") {
		t.Errorf("GET on a dead link = %d, want 400 and no button", rec.Code)
	}
	c.pendingErr = errors.New("db down")
	if rec := serveConfirm(signupConfirmHandler(c, allowAll), http.MethodGet); rec.Code != http.StatusInternalServerError {
		t.Errorf("GET with the lookup failing = %d, want 500", rec.Code)
	}
}

func TestSignupConfirmPost(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"confirmed", nil, http.StatusOK},
		{"dead link", form.ErrConfirmationInvalid, http.StatusBadRequest},
		{"write failed", errors.New("db down"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &confirmLog{confirmErr: tc.err}
			rec := serveConfirm(signupConfirmHandler(c, allowAll), http.MethodPost)
			if rec.Code != tc.want {
				t.Errorf("POST = %d, want %d", rec.Code, tc.want)
			}
			if len(c.confirmed) != 1 || c.confirmed[0] != "tok" {
				t.Errorf("confirmed %v, want the link's token once", c.confirmed)
			}
		})
	}
	c := &confirmLog{}
	if rec := serveConfirm(signupConfirmHandler(c, func(*http.Request) bool { return false }), http.MethodPost); rec.Code != http.StatusTooManyRequests || len(c.confirmed) != 0 {
		t.Errorf("over budget = %d with %d confirms, want 429 and none", rec.Code, len(c.confirmed))
	}
}
