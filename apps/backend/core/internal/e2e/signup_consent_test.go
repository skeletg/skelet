package e2e

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/e2ekit"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
)

var confirmLink = regexp.MustCompile(`/subscribe/confirm/([A-Za-z0-9_-]+)`)

type marketingState struct {
	customerID uuid.UUID
	optedIn    bool
	source     string
	events     int
}

func (h *harness) marketingConsent(t *testing.T, channelID uuid.UUID, addr string) (marketingState, bool) {
	t.Helper()
	var s marketingState
	err := h.D.DB.QueryRow(`SELECT c.id, COALESCE(cc.opted_in, false), COALESCE(cc.source, ''),
		(SELECT count(*) FROM customer_consent_events e WHERE e.customer_id = c.id)
		FROM customers c LEFT JOIN customer_consents cc ON cc.customer_id = c.id AND cc.kind = 'marketing_email'
		WHERE c.channel_id = $1 AND c.email = $2`, channelID, addr).Scan(&s.customerID, &s.optedIn, &s.source, &s.events)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false
	}
	if err != nil {
		t.Fatalf("read consent: %v", err)
	}
	return s, true
}

func TestSignupFormDoubleOptInAndOneClickUnsubscribe(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 1000)
	ts := httptest.NewServer(h.D.Root)
	defer ts.Close()

	created, err := h.D.Form.Create(h.Ctx, signupform.SaveInput{
		ChannelID: f.Channel.ID, Slug: "e2e-doi-" + uuid.NewString()[:8], Name: "Newsletter", Status: "active",
	})
	if err != nil {
		t.Fatalf("create form: %v", err)
	}
	addr := "doi-" + uuid.NewString()[:8] + "@e2e.test"
	h.Mail.Reset()
	status, _, body := tRequest(t, ts, http.MethodPost, "/t/forms/submit", f.Channel.ID.String(), fmt.Sprintf(`{"form":%q,"email":%q}`, created.Slug, addr))
	if status != http.StatusOK {
		t.Fatalf("submit = %d %q", status, body)
	}
	if _, exists := h.marketingConsent(t, f.Channel.ID, addr); exists {
		t.Fatal("the submit created a contact before the address was confirmed")
	}
	mail := h.Mail.To(addr)
	if len(mail) != 1 {
		t.Fatalf("confirmation mails = %d, want 1 (subjects %v)", len(mail), h.Mail.SubjectsTo(addr))
	}
	m := confirmLink.FindStringSubmatch(mail[0].TextBody)
	if m == nil {
		t.Fatalf("no confirmation link in the mail:\n%s", mail[0].TextBody)
	}
	path := m[0]

	if status, _, _ := tRequest(t, ts, http.MethodGet, path, "", ""); status != http.StatusOK {
		t.Fatalf("GET confirm page = %d, want 200", status)
	}
	if _, exists := h.marketingConsent(t, f.Channel.ID, addr); exists {
		t.Fatal("opening the link subscribed the address without the button")
	}
	if status, _, _ := tRequest(t, ts, http.MethodPost, path, "", ""); status != http.StatusOK {
		t.Fatalf("POST confirm = %d, want 200", status)
	}
	st, exists := h.marketingConsent(t, f.Channel.ID, addr)
	if !exists || !st.optedIn || st.source != "signup_form" || st.events != 1 {
		t.Fatalf("after confirming: %+v (exists %v), want opted in from signup_form with one history entry", st, exists)
	}
	if status, _, _ := tRequest(t, ts, http.MethodPost, path, "", ""); status != http.StatusBadRequest {
		t.Errorf("replayed confirm = %d, want 400 - the link is single-use", status)
	}

	cfg, _ := e2ekit.Config()
	unsub := "/unsubscribe/" + cfg.EmailLinks.SignUnsubscribe(st.customerID)
	if status, _, _ := tRequest(t, ts, http.MethodGet, unsub, "", ""); status != http.StatusOK {
		t.Fatalf("GET unsubscribe page = %d, want 200", status)
	}
	if st, _ := h.marketingConsent(t, f.Channel.ID, addr); !st.optedIn {
		t.Fatal("opening the unsubscribe link opted out without the button")
	}
	if status, _, _ := tRequest(t, ts, http.MethodPost, unsub, "", "List-Unsubscribe=One-Click", "Content-Type", "application/x-www-form-urlencoded"); status != http.StatusOK {
		t.Fatalf("one-click unsubscribe = %d, want 200", status)
	}
	if st, _ := h.marketingConsent(t, f.Channel.ID, addr); st.optedIn || st.source != "unsubscribe_link" || st.events != 2 {
		t.Errorf("after one-click: %+v, want opted out from the unsubscribe link with two history entries", st)
	}
}
