package wiring

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

type nopMail struct{}

func (nopMail) Send(context.Context, email.Message) error { return nil }

type orderLog struct {
	mu    sync.Mutex
	steps []string
}

func (o *orderLog) add(s string) { o.mu.Lock(); o.steps = append(o.steps, s); o.mu.Unlock() }

type slowRecorder struct{ log *orderLog }

func (r slowRecorder) RecordSent(context.Context, uuid.UUID, mailsend.Spec) error {
	time.Sleep(50 * time.Millisecond)
	r.log.add("recorded")
	return nil
}

func TestPendingSendReportsLandBeforeTheSignalsClientCloses(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	steps := &orderLog{}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { steps.add("client closed"); return nil }})
	s := mailsend.New(nopMail{}, nil)
	s.SetRecorder(slowRecorder{log: steps})
	drainSendReportsOnStop(lc, s)
	lc.RequireStart()

	if err := s.Send(context.Background(), mailsend.Spec{From: "shop@example.test", To: "a@example.test"}, "Hi", "<p>x</p>", "x"); err != nil {
		t.Fatal(err)
	}
	lc.RequireStop()
	if len(steps.steps) != 2 || steps.steps[0] != "recorded" {
		t.Fatalf("stop ran %v, want the report recorded before the client closed", steps.steps)
	}
}
