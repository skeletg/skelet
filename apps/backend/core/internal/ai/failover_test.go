package ai

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// A provider whose answers depend on the model asked for - a status per model, or failures for the first n calls
type fakeProvider struct {
	mu      sync.Mutex
	status  map[string]int
	failFor map[string]int
	calls   []string
}

func (m *fakeProvider) asked(model string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		if c == model {
			n++
		}
	}
	return n
}

func (m *fakeProvider) serve(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.calls = append(m.calls, body.Model)
		status := m.status[body.Model]
		if m.failFor[body.Model] > 0 {
			m.failFor[body.Model]--
			status = 529
		}
		m.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"answered by ` + body.Model + `"}],"usage":{"input_tokens":3,"output_tokens":2}}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func failoverService(t *testing.T, url string, settings map[string]string) *Service {
	t.Helper()
	svc := stubService(t, providerAnthropic, "big", url)
	maps.Copy(svc.cfg.(stubCfg).strs, settings)
	return svc
}

var hello = []ChatMessage{{Role: "user", Text: "hi"}}

func TestAnOverloadedModelGetsOneMoreTryThenTheFallback(t *testing.T) {
	ctx := context.Background()
	m := &fakeProvider{status: map[string]int{"big": 529}}
	svc := failoverService(t, m.serve(t), map[string]string{FallbackModelPath: "small"})
	turn, err := svc.ChatWithTools(ctx, "sys", hello, nil, 100)
	if err != nil || turn.Text != "answered by small" || turn.Model != "small" {
		t.Fatalf("turn = %+v, %v - want the fallback's answer, saying so", turn, err)
	}
	if m.asked("big") != 2 || m.asked("small") != 1 {
		t.Errorf("calls %v - want the main model tried twice, then the fallback once", m.calls)
	}

	again := &fakeProvider{failFor: map[string]int{"big": 1}}
	svc = failoverService(t, again.serve(t), map[string]string{FallbackModelPath: "small"})
	if out, err := svc.Chat(ctx, "sys", hello, 100); err != nil || out != "answered by big" || again.asked("small") != 0 {
		t.Errorf("a blip = %q, %v, calls %v - want the main model's second try to answer", out, err, again.calls)
	}
}

func TestARequestTheProviderTurnedDownNeverGoesToTheFallback(t *testing.T) {
	m := &fakeProvider{status: map[string]int{"big": http.StatusBadRequest}}
	svc := failoverService(t, m.serve(t), map[string]string{FallbackModelPath: "small"})
	if _, err := svc.Chat(context.Background(), "sys", hello, 100); err == nil || len(m.calls) != 1 {
		t.Errorf("a 400 = %v after %v - want it returned at once, the fallback never asked", err, m.calls)
	}
}

func TestAModelThatKeepsFailingRestsWhileItsCallsGoStraightToTheFallback(t *testing.T) {
	ctx := context.Background()
	m := &fakeProvider{status: map[string]int{"big": 503}}
	svc := failoverService(t, m.serve(t), map[string]string{FallbackModelPath: "small"})
	for range breakerTrips {
		if _, err := svc.Chat(ctx, "sys", hello, 100); err != nil {
			t.Fatal(err)
		}
	}
	tried := m.asked("big")
	if out, err := svc.Chat(ctx, "sys", hello, 100); err != nil || out != "answered by small" || m.asked("big") != tried {
		t.Fatalf("while resting = %q, %v - want the fallback asked straight away", out, err)
	}

	m.mu.Lock()
	m.status = map[string]int{}
	m.mu.Unlock()
	svc.trips.mu.Lock()
	for _, tr := range svc.trips.trips {
		tr.until = time.Now().Add(-time.Second)
	}
	svc.trips.mu.Unlock()
	if out, err := svc.Chat(ctx, "sys", hello, 100); err != nil || out != "answered by big" {
		t.Errorf("after the rest = %q, %v - want the main model back", out, err)
	}
}

func TestNoFallbackIsStartedWithoutTimeToFinishIt(t *testing.T) {
	m := &fakeProvider{status: map[string]int{"big": 529}}
	svc := failoverService(t, m.serve(t), map[string]string{FallbackModelPath: "small"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := svc.Chat(ctx, "sys", hello, 100); err == nil || len(m.calls) != 1 {
		t.Errorf("a second left = %v after %v - want the failure returned, no retry or fallback that couldn't finish", err, m.calls)
	}
}

func TestShortWritingOnTheQuickModelFallsBackToTheMainOne(t *testing.T) {
	m := &fakeProvider{status: map[string]int{"quick": 529}}
	svc := failoverService(t, m.serve(t), map[string]string{QuickModelPath: "quick"})
	out, err := svc.Chat(Quick(context.Background()), "sys", hello, 100)
	if err != nil || out != "answered by big" {
		t.Errorf("quick model down = %q, %v, calls %v - want the main model to answer", out, err, m.calls)
	}
	if _, err := svc.Chat(context.Background(), "sys", hello, 100); err != nil || m.asked("big") != 2 {
		t.Errorf("an ordinary call = %v, calls %v", err, m.calls)
	}
}

func TestARetryWaitsWhatTheProviderAsksUpToASecond(t *testing.T) {
	if d := retryPauseFor(&providerError{status: 429, retryAfter: 30 * time.Second}); d != maxRetryAfter {
		t.Errorf("a 30s retry-after waits %v", d)
	}
	if d := retryPauseFor(&providerError{status: 529}); d != retryPause {
		t.Errorf("no retry-after waits %v", d)
	}
}
