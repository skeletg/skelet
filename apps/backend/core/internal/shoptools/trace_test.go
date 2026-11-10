package shoptools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestLangfuseGetsTheShoppersWordsOnlyForAConversationTheStoreKeeps(t *testing.T) {
	for name, tc := range map[string]struct {
		keep, signalsOff, consent bool
		wantWords                 bool
	}{
		"kept":                 {keep: true, consent: true, wantWords: true},
		"store keeps none":     {keep: false, consent: true},
		"shopper didn't agree": {keep: true, consent: false},
		"Signals is off":       {keep: true, consent: true, signalsOff: true},
	} {
		rec := tracetest.NewSpanRecorder()
		aitrace.Install(aitrace.Fixed(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))))
		s, _ := transcriptRig(&fakeModel{reply: "We ship to Norway."}, tc.keep)
		s.deps.SignalsOff = tc.signalsOff
		who := Shopper{Address: "ip"}
		if tc.consent {
			who.Conversation = uuid.New()
		}
		if _, err := s.Assist(shopperCtx(), shop, who, ask, "/shipping", Notify{}); err != nil {
			t.Fatal(err)
		}
		aitrace.Install(nil)

		spans := rec.Ended()
		if len(spans) != 1 {
			t.Fatalf("%s: %d spans", name, len(spans))
		}
		got := map[attribute.Key]string{}
		for _, a := range spans[0].Attributes() {
			got[a.Key] = a.Value.String()
		}
		page, sent := got["langfuse.observation.metadata.page"]
		if got["langfuse.trace.name"] != "Shopping assistant" || sent != tc.wantWords || (sent && page != "/shipping") || got["langfuse.observation.metadata.kept"] != boolText(tc.wantWords) {
			t.Errorf("%s: %v", name, got)
		}
		words := got["langfuse.observation.input"] == "Two mugs please" && got["langfuse.observation.output"] != "" && got["langfuse.session.id"] != "" && got["langfuse.user.id"] == "visitor:visitor-1"
		if words != tc.wantWords {
			t.Errorf("%s: words, session and person sent = %v, want %v (%v)", name, words, tc.wantWords, got)
		}
		if !tc.wantWords && (got["langfuse.observation.input"] != "" || got["langfuse.session.id"] != "" || got["langfuse.user.id"] != "") {
			t.Errorf("%s: something of the shopper's went: %v", name, got)
		}
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type langfuseSettings map[string]string

func (l langfuseSettings) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return l[path] == "true", nil
}

func (l langfuseSettings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return l[path], nil
}

// A stand-in for Langfuse that keeps every score request it's sent
func fakeLangfuse(t *testing.T) (*aitrace.Sink, func() string) {
	t.Helper()
	var mu sync.Mutex
	var scores []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/scores" {
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			scores = append(scores, string(raw))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	sink := aitrace.New(langfuseSettings{aitrace.EnabledPath: "true", aitrace.HostPath: srv.URL, aitrace.PublicKeyPath: "pk", aitrace.SecretKeyPath: "sk"})
	aitrace.Install(sink)
	t.Cleanup(func() { aitrace.Install(nil) })
	return sink, func() string {
		_ = sink.Shutdown(context.Background())
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(scores, "\n")
	}
}

func TestATurnIsScoredAndItsCartRemembersIt(t *testing.T) {
	carts := &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}
	own := webCart(carts, shop.ID)
	s, _, _ := settledRig(&fakeModel{calls: []ai.ToolCall{addMugs}}, carts)
	_, sent := fakeLangfuse(t)
	if _, err := s.Assist(context.Background(), shop, guest(&own), ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	trace := carts.traces[own]
	got := sent()
	if len(trace) != 32 || !strings.Contains(got, `"traceId":"`+trace+`"`) {
		t.Fatalf("cart remembers trace %q, scores sent %s", trace, got)
	}
	for _, want := range []string{`"name":"unanswered","traceId":"` + trace + `","value":0`, `"name":"filled_cart","traceId":"` + trace + `","value":1`} {
		if !strings.Contains(got, want) {
			t.Errorf("scores %s lack %s", got, want)
		}
	}
}

func TestAnOrderFromTheCartScoresTheTurnThatFilledIt(t *testing.T) {
	_, sent := fakeLangfuse(t)
	if err := onOrderPlaced(context.Background(), &order.OrderPlaced{OrderNumber: "1042", AssistedBy: "assistant", AssistTrace: "4bf92f3577b34da6a3ce929d0e0e4736"}); err != nil {
		t.Fatal(err)
	}
	if err := onOrderPlaced(context.Background(), &order.OrderPlaced{OrderNumber: "1043", AssistedBy: "agent"}); err != nil {
		t.Fatal(err)
	}
	got := sent()
	if !strings.Contains(got, `"comment":"Order 1042"`) || !strings.Contains(got, `"name":"led_to_order"`) || strings.Contains(got, "1043") {
		t.Errorf("scores %s", got)
	}
}

func TestThePromptVersionFollowsWhatTheModelIsGiven(t *testing.T) {
	base := assistantPrompt(shop, voice{})
	v := promptVersion(base, assistantToolsDigest())
	if !strings.HasPrefix(v, "assistant-") || len(v) != len("assistant-")+8 || v != promptVersion(base, assistantToolsDigest()) {
		t.Fatalf("version %q", v)
	}
	if promptVersion(assistantPrompt(shop, voice{tone: "brief"}), assistantToolsDigest()) == v {
		t.Error("a new tone kept the version")
	}
	if promptVersion(base, toolsDigest(assistantTools[:len(assistantTools)-1])) == v {
		t.Error("a tool less kept the version")
	}
}
