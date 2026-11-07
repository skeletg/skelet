package aitrace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type settings map[string]string

func (s settings) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return s[path] == "true", nil
}

func (s settings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return s[path], nil
}

// recording installs a sink whose spans land in memory, as if Langfuse were connected
func recording(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	Install(Fixed(tp))
	t.Cleanup(func() { Install(nil) })
	return rec
}

func attrs(s sdktrace.ReadOnlySpan) map[attribute.Key]string {
	out := map[attribute.Key]string{}
	for _, a := range s.Attributes() {
		out[a.Key] = a.Value.String()
	}
	return out
}

func TestWhatWasWrittenGoesOnlyWhereTheConversationIsKept(t *testing.T) {
	for _, kept := range []bool{false, true} {
		rec := recording(t)
		ctx := WithContent(context.Background(), kept)
		ctx, turn := Start(ctx, "Shopping assistant", Agent)
		turn.Trace("Shopping assistant", "conv-1", "customer:42", "storefront")
		turn.Input("do you ship to Norway?")
		_, gen := Start(ctx, "Model call", Generation)
		gen.Model("claude-opus-5-5")
		gen.Usage(100, 20, 300, 0)
		gen.Output("We do, in a week.")
		gen.Fail(nil)
		gen.End()
		turn.Meta("cart_changed", true)
		turn.End()

		spans := rec.Ended()
		if len(spans) != 2 || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() {
			t.Fatalf("kept=%v: want the model call inside the turn, got %d spans", kept, len(spans))
		}
		g, root := attrs(spans[0]), attrs(spans[1])
		if g["langfuse.observation.type"] != "generation" || root["langfuse.observation.type"] != "agent" || root["langfuse.trace.name"] != "Shopping assistant" {
			t.Errorf("kept=%v: types %v / %v", kept, g, root)
		}
		if g["gen_ai.request.model"] != "claude-opus-5-5" || g["gen_ai.usage.input_tokens"] != "400" || !strings.Contains(g["langfuse.observation.usage_details"], `"cache_read_input_tokens":300`) {
			t.Errorf("kept=%v: model and tokens always go: %v", kept, g)
		}
		if root["langfuse.observation.metadata.cart_changed"] != "true" {
			t.Errorf("kept=%v: flags always go: %v", kept, root)
		}
		for _, k := range []attribute.Key{"langfuse.observation.input", "langfuse.session.id", "langfuse.user.id"} {
			if _, has := root[k]; has != kept {
				t.Errorf("kept=%v: %s present=%v", kept, k, has)
			}
		}
		if _, has := g["langfuse.observation.output"]; has != kept {
			t.Errorf("kept=%v: the reply went=%v", kept, has)
		}
	}
}

func TestAFailureSaysWhatWentWrong(t *testing.T) {
	rec := recording(t)
	_, gen := Start(context.Background(), "Model call", Generation)
	gen.Fail(context.DeadlineExceeded)
	gen.End()
	if a := attrs(rec.Ended()[0]); a["langfuse.observation.level"] != "ERROR" || a["langfuse.observation.status_message"] != "context deadline exceeded" {
		t.Errorf("attrs %v", a)
	}
}

func TestNothingIsRecordedWithoutASink(t *testing.T) {
	Install(nil)
	_, obs := Start(WithContent(context.Background(), true), "x", Span)
	if obs.on() {
		t.Fatal("recording with no sink installed")
	}
	obs.Input("anything")
	obs.Usage(1, 1, 1, 1)
	obs.End()
}

func TestClipKeepsCharactersWhole(t *testing.T) {
	long := strings.Repeat("é", maxText)
	if got := clip(long); !strings.HasSuffix(got, "…") || len(got) > maxText+len("…") || strings.ContainsRune(got, '�') {
		t.Errorf("clip cut a character: %q", got[len(got)-8:])
	}
}

// The real pipeline: settings point it at a server standing in for Langfuse, and a trace arrives there signed with the keys
func TestTheSinkSendsToLangfuseWithTheProjectsKeys(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_WEBHOOK_TARGETS", "true") // the stand-in Langfuse listens on loopback
	var mu sync.Mutex
	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := New(settings{EnabledPath: "true", HostPath: srv.URL + "/", PublicKeyPath: "pk-lf-1", SecretKeyPath: "sk-lf-2"})
	Install(sink)
	t.Cleanup(func() { Install(nil) })
	_, obs := Start(context.Background(), "Model call", Generation)
	if !obs.on() {
		t.Fatal("not recording with Langfuse on and both keys set")
	}
	obs.End()
	if err := sink.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("nothing reached the server")
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-lf-1:sk-lf-2"))
	if r := got[0]; r.URL.Path != "/api/public/otel/v1/traces" || r.Header.Get("Authorization") != want {
		t.Errorf("sent to %s with %q", r.URL.Path, r.Header.Get("Authorization"))
	}
}

func TestTheSinkStaysQuietUntilItsSetUp(t *testing.T) {
	for name, s := range map[string]settings{
		"off":            {EnabledPath: "false", PublicKeyPath: "pk", SecretKeyPath: "sk"},
		"no secret key":  {EnabledPath: "true", PublicKeyPath: "pk"},
		"not an address": {EnabledPath: "true", HostPath: "cloud.langfuse.com", PublicKeyPath: "pk", SecretKeyPath: "sk"},
	} {
		Install(New(s))
		if _, obs := Start(context.Background(), "x", Span); obs.on() {
			t.Errorf("%s: recording", name)
		}
	}
	Install(nil)
}

// A server span above stays the server's, and an HTTP client call below never sees the observation as its parent
func TestObservationsStayOutOfTheServersTracing(t *testing.T) {
	rec := recording(t)
	server := sdktrace.NewTracerProvider().Tracer("server")
	ctx, request := server.Start(context.Background(), "HTTP POST /graphql")
	defer request.End()
	ctx, obs := Start(ctx, "Shopping assistant", Agent)
	if got := trace.SpanFromContext(ctx); got != request {
		t.Error("the observation became the context's active span, so the HTTP and database spans under it would join Langfuse's trace")
	}
	obs.End()
	if root := rec.Ended()[0]; root.Parent().IsValid() {
		t.Error("the Langfuse trace hangs off the server's span instead of starting its own")
	}
}

func TestAVersionTagsEveryObservationUnderIt(t *testing.T) {
	rec := recording(t)
	ctx, turn := Start(WithVersion(context.Background(), "assistant-1a2b3c4d"), "Shopping assistant", Agent)
	_, gen := Start(ctx, "Model call", Generation)
	if gen.TraceID() == "" || gen.TraceID() != turn.TraceID() {
		t.Errorf("trace ids %q and %q - want one trace", gen.TraceID(), turn.TraceID())
	}
	gen.End()
	turn.End()
	for _, s := range rec.Ended() {
		if attrs(s)["langfuse.version"] != "assistant-1a2b3c4d" {
			t.Errorf("%s lacks the version: %v", s.Name(), attrs(s))
		}
	}
	if (&Obs{span: trace.SpanFromContext(context.Background())}).TraceID() != "" {
		t.Error("an observation nothing records has a trace id to score")
	}
}

func TestOutcomesGoToLangfuseAsBooleanScores(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_WEBHOOK_TARGETS", "true") // the stand-in Langfuse listens on loopback
	var mu sync.Mutex
	scored := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/scores" {
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			scored[r.Header.Get("Authorization")] = string(raw)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	sink := New(settings{EnabledPath: "true", HostPath: srv.URL, PublicKeyPath: "pk", SecretKeyPath: "sk"})
	Install(sink)
	t.Cleanup(func() { Install(nil) })

	Score(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736", Outcome{Name: "unanswered", Value: false}, Outcome{Name: "led_to_order", Value: true, Comment: "Order 1042"})
	Score(context.Background(), "", Outcome{Name: "unanswered", Value: true})
	if err := sink.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	body, ok := scored["Basic "+base64.StdEncoding.EncodeToString([]byte("pk:sk"))]
	if !ok || len(scored) != 1 {
		t.Fatalf("scores sent %v - want one request, signed with the keys", scored)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil || len(got) != 2 {
		t.Fatalf("body %s", body)
	}
	if got[0]["id"] != "4bf92f3577b34da6a3ce929d0e0e4736-unanswered" || got[0]["value"] != 0.0 || got[0]["dataType"] != "BOOLEAN" ||
		got[1]["value"] != 1.0 || got[1]["comment"] != "Order 1042" || got[1]["traceId"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("scores %v", got)
	}

	off := New(settings{EnabledPath: "false", HostPath: srv.URL, PublicKeyPath: "pk", SecretKeyPath: "sk"})
	Install(off)
	Score(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736", Outcome{Name: "unanswered", Value: true})
	_ = off.Shutdown(context.Background())
	if len(scored) != 1 {
		t.Error("scored with Langfuse off")
	}
}

func TestAnOlderLangfuseGetsTheScoresOneAtATime(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_WEBHOOK_TARGETS", "true")
	var mu sync.Mutex
	var singles []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var one map[string]any
		if json.Unmarshal(raw, &one) != nil {
			w.WriteHeader(http.StatusBadRequest) // an array - this version wants one score
			return
		}
		mu.Lock()
		singles = append(singles, one)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	sink := New(settings{EnabledPath: "true", HostPath: srv.URL, PublicKeyPath: "pk", SecretKeyPath: "sk"})
	Install(sink)
	t.Cleanup(func() { Install(nil) })
	Score(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736", Outcome{Name: "unanswered", Value: false}, Outcome{Name: "filled_cart", Value: true})
	if err := sink.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(singles) != 2 || singles[0]["name"] != "unanswered" || singles[1]["name"] != "filled_cart" {
		t.Errorf("scores sent one by one: %v", singles)
	}
}
