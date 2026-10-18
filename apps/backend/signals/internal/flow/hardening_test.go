package flow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	emailpkg "github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
)

type failingMailbox struct {
	err   error
	tries int
}

func (m *failingMailbox) Send(context.Context, emailpkg.Message) error {
	m.tries++
	return m.err
}

type capturedBounces struct{ addrs []string }

func (c *capturedBounces) Capture(_ context.Context, addr string, _ error) {
	c.addrs = append(c.addrs, addr)
}

type sendLog struct{ specs []sendlog.Spec }

func (l *sendLog) Sent(_ context.Context, s sendlog.Spec) error {
	l.specs = append(l.specs, s)
	return nil
}

func mailingService(repo *fakeRepo, sender emailpkg.Sender, sends sendTracker) *Service {
	return NewService(repo, emailtemplate.NewService(nil, nil), optedIn{}, fixedStore{active: true}, sender, nil, sends, nil, false, emailtrack.NewSigner([]byte("k")))
}

// A relay that is down for a moment must not cost a customer their password-reset mail - the run waits on its node and
// tries again on the backoff ladder, and only fails once the ladder runs out
func TestAFailedSendRetriesOnItsNodeUntilTheLadderRunsOut(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	box := &failingMailbox{err: errors.New("dial tcp: i/o timeout")}
	log := &sendLog{}
	svc := mailingService(repo, box, log)
	bounces := &capturedBounces{}
	svc.SetBounces(bounces)
	f := mailingFlow(repo, true)
	run := FlowRun{ID: uuid.New(), FlowID: f.ID, CustomerID: uuid.New(), CurrentNodeID: "m", Status: RunActive}
	for attempt := 1; attempt < maxSendAttempts; attempt++ {
		if err := svc.advanceWithFlow(ctx, run, f, true); err != nil {
			t.Fatal(err)
		}
		run = repo.runs[run.ID]
		if run.Status != RunActive || run.CurrentNodeID != "m" || run.SendAttempts != attempt || run.ResumeAt == nil || !run.ResumeAt.After(time.Now()) {
			t.Fatalf("after failure %d: %+v - want the run parked on its send node, attempt counted, due later", attempt, run)
		}
	}
	if err := svc.advanceWithFlow(ctx, run, f, true); err != nil {
		t.Fatal(err)
	}
	if run = repo.runs[run.ID]; run.Status != RunFailed || !strings.Contains(run.Error, "not delivered") {
		t.Fatalf("after the last rung: status %s, error %q - want failed", run.Status, run.Error)
	}
	if len(log.specs) != 0 {
		t.Errorf("logged %d sends for mail the relay never took", len(log.specs))
	}
	if len(bounces.addrs) != maxSendAttempts {
		t.Errorf("bounce loop heard %d failures, want every one", len(bounces.addrs))
	}
}

func TestOnlyFailuresWorthRetryingAreRetried(t *testing.T) {
	permanent := errors.New("550 5.1.1 user unknown")
	temporary := errors.New("421 4.7.0 try again later")
	unknown := errors.New("connection reset by peer")
	for name, tc := range map[string]struct {
		err           error
		transactional bool
		want          bool
	}{
		"permanent transactional": {permanent, true, false},
		"temporary marketing":     {temporary, false, true},
		"unknown transactional":   {unknown, true, true},
		"unknown marketing":       {unknown, false, false},
	} {
		if got := retryable(tc.transactional, tc.err); got != tc.want {
			t.Errorf("%s: retryable = %v, want %v", name, got, tc.want)
		}
	}
}

func TestADeliveredSendResetsTheLadderAndIsLogged(t *testing.T) {
	repo := newFakeRepo()
	log := &sendLog{}
	svc := mailingService(repo, &mailbox{}, log)
	f := mailingFlow(repo, true)
	run := FlowRun{ID: uuid.New(), FlowID: f.ID, CustomerID: uuid.New(), CurrentNodeID: "m", Status: RunActive, SendAttempts: 3}
	if err := svc.advanceWithFlow(context.Background(), run, f, true); err != nil {
		t.Fatal(err)
	}
	if got := repo.runs[run.ID]; got.Status != RunCompleted || got.SendAttempts != 0 {
		t.Errorf("after a delivered retry: %+v, want completed with the ladder reset", got)
	}
	if len(log.specs) != 1 {
		t.Errorf("logged %d sends, want the one delivered", len(log.specs))
	}
}

// A webhook node must never hand an outside URL the one-time links an account flow carries
func TestWebhooksNeverCarrySignInOrResetLinks(t *testing.T) {
	var got map[string]any
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	t.Cleanup(target.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	svc := NewService(newFakeRepo(), nil, nil, nil, nil, nil, nil, nil, true, emailtrack.NewSigner([]byte("k")))
	runCtx, _ := json.Marshal(map[string]any{
		signalsapi.PayloadResetLink: "https://shop/reset/secret", signalsapi.PayloadSignInLink: "https://shop/in/secret",
		signalsapi.PayloadVerifyLink: "v", signalsapi.PayloadConfirmLink: "c", signalsapi.PayloadForgotLink: "f", signalsapi.PayloadOfferLink: "o",
		signalsapi.PayloadProductName: "Tire",
	})
	svc.execWebhook(context.Background(), Node{Data: NodeData{URL: srv.URL}}, FlowRun{FlowID: uuid.New(), Context: runCtx})
	c, _ := got["context"].(map[string]any)
	if c["product_name"] != "Tire" {
		t.Fatalf("webhook context = %v, want the ordinary keys kept", c)
	}
	for _, k := range linkPayloadKeys {
		if _, leaked := c[k]; leaked {
			t.Errorf("webhook received %s", k)
		}
	}
	if redirected {
		t.Error("the webhook followed a redirect with the run's context")
	}
}

func TestAccountFlowsCantCallOutAndOnlyOrderOrAccountFlowsAreTransactional(t *testing.T) {
	s := validator()
	webhook := draft(Node{ID: "t", Type: NodeTrigger}, Node{ID: "w", Type: NodeWebhook, Data: NodeData{URL: "https://hooks.example/x"}})
	webhook.TriggerType = TriggerPasswordReset
	if err := s.validate(webhook); !errors.Is(err, ErrInvalid) {
		t.Errorf("a webhook on the password-reset flow saved: %v", err)
	}
	marketing := withConfig(draft(Node{ID: "t", Type: NodeTrigger}), `{"transactional":true}`)
	marketing.TriggerType = TriggerCartAbandoned
	if err := s.validate(marketing); !errors.Is(err, ErrInvalid) {
		t.Errorf("a cart-abandonment flow saved as transactional: %v", err)
	}
	stored := Flow{TriggerType: TriggerCartAbandoned, TriggerConfig: json.RawMessage(`{"transactional":true}`)}
	if flowTransactional(stored) {
		t.Error("a stored marketing flow claiming transactional would skip consent and suppression")
	}
	if !flowTransactional(Flow{TriggerType: TriggerOrderPlaced, TriggerConfig: json.RawMessage(`{"transactional":true}`)}) {
		t.Error("the order receipt flow lost its transactional standing")
	}
}

func TestACorruptEnrollmentFilterEnrollsNoOne(t *testing.T) {
	repo := newFakeRepo()
	f := countingFlow(repo, TriggerCartAbandoned)
	f.TriggerConfig = json.RawMessage(`{"enrollmentFilter":`)
	repo.flows[f.ID] = f
	svc := &Service{repo: repo}
	if _, took, err := svc.enroll(context.Background(), f, uuid.New(), "", nil, "", false); took || err == nil {
		t.Errorf("enroll took=%v err=%v - a filter that won't parse must enroll no one, loudly", took, err)
	}
}

func TestSetStatusOnAMissingFlowIsNotFound(t *testing.T) {
	svc := &Service{repo: newFakeRepo()}
	if _, err := svc.SetStatus(context.Background(), uuid.New(), StatusPaused); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

type slowClaims struct {
	*fakeRepo
	runs []FlowRun
}

func (r slowClaims) ClaimRunnable(context.Context, int32, time.Duration) ([]FlowRun, error) {
	return r.runs, nil
}

// Runs past half the lease are left to lapse, or another replica's claim would take them while this batch still sends
func TestABatchStopsBeforeItsLeaseRunsOut(t *testing.T) {
	repo := newFakeRepo()
	f := countingFlow(repo, TriggerOrderPlaced)
	runs := []FlowRun{{ID: uuid.New(), FlowID: f.ID, CurrentNodeID: "t"}, {ID: uuid.New(), FlowID: f.ID, CurrentNodeID: "t"}}
	defer func(prev time.Duration) { batchBudget = prev }(batchBudget)
	batchBudget = -time.Second
	svc := &Service{repo: slowClaims{repo, runs}}
	n, _ := svc.processBatch(context.Background())
	if n != 0 || len(repo.runs) != 0 {
		t.Errorf("advanced %d runs (%d written) after the budget was gone", n, len(repo.runs))
	}
}
