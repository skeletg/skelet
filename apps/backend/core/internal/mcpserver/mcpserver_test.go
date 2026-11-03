package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`

var objectSchema = map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "integer"}}}

var registry = []ai.Tool{
	{Name: "echo", Schema: objectSchema, Effect: ai.Reads, Idempotent: true},
	{Name: "broken", Effect: ai.Reads, Idempotent: true},
	{Name: "boom", Effect: ai.Reads, Idempotent: true},
	{Name: "block", Effect: ai.Reads, Idempotent: true},
	{Name: "hidden", Effect: ai.Reads, Idempotent: true},
}

type fakeTools struct {
	defs    []ai.Tool
	entered chan struct{}
	release chan struct{}
}

func (f fakeTools) Tools() []ai.Tool { return f.defs }

func (f fakeTools) Call(ctx context.Context, name string, args json.RawMessage) ai.ToolResult {
	switch name {
	case "broken":
		return ai.ToolResult{IsError: true, Content: "it broke"}
	case "boom":
		panic("tool blew up")
	case "block":
		f.entered <- struct{}{}
		select {
		case <-f.release:
		case <-ctx.Done():
		}
	}
	return ai.ToolResult{Content: name + " " + string(args)}
}

type fakeChannels struct {
	known  map[uuid.UUID]channel.Channel
	broken uuid.UUID
}

func (f fakeChannels) GetByID(_ context.Context, id uuid.UUID) (channel.Channel, error) {
	if id == f.broken {
		return channel.Channel{}, errors.New("db down")
	}
	if ch, ok := f.known[id]; ok {
		return ch, nil
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

type recorder struct {
	mu      sync.Mutex
	asked   []uuid.UUID
	onCtx   []uuid.UUID
	days    int
	none    uuid.UUID
	entered chan struct{}
	release chan struct{}
}

func (rec *recorder) source(ctx context.Context, cid uuid.UUID, days int) ToolSet {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.asked, rec.days = append(rec.asked, cid), days
	if ch, ok := channel.FromContext(ctx); ok {
		rec.onCtx = append(rec.onCtx, ch.ID)
	}
	if cid == rec.none {
		return fakeTools{}
	}
	return fakeTools{defs: registry[:4], entered: rec.entered, release: rec.release}
}

type keyRecorder struct {
	mu   sync.Mutex
	keys []string
}

func (k *keyRecorder) Allow(_ context.Context, key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys = append(k.keys, key)
	return false
}

type panicReporter struct{ panics atomic.Int32 }

func (*panicReporter) CaptureAppError(context.Context, *apperr.Error) {}
func (p *panicReporter) CapturePanic(context.Context, any)            { p.panics.Add(1) }

type fixture struct {
	rec      *recorder
	chans    fakeChannels
	pin, alt uuid.UUID
	handler  http.Handler
	reporter *panicReporter
}

func newFixture(t *testing.T, limiter interface {
	Allow(context.Context, string) bool
}) *fixture {
	t.Helper()
	f := &fixture{pin: uuid.New(), alt: uuid.New(), reporter: &panicReporter{}}
	f.rec = &recorder{none: uuid.New(), entered: make(chan struct{}, 16), release: make(chan struct{})}
	f.chans = fakeChannels{broken: uuid.New(), known: map[uuid.UUID]channel.Channel{
		f.pin: {ID: f.pin, Name: "Pinned"}, f.alt: {ID: f.alt, Name: "Other"}, f.rec.none: {ID: f.rec.none, Name: "Closed"},
	}}
	f.handler = NewHandler(Config{Tools: registry, Resolve: Admin(f.rec.source, f.chans), Instructions: AdminInstructions, Limiter: limiter, Reporter: f.reporter})
	return f
}

func token(pin *uuid.UUID) func(context.Context) context.Context {
	id := uuid.New()
	return func(ctx context.Context) context.Context {
		return auth.WithApiToken(ctx, &auth.ApiTokenClaims{TokenID: id, Name: "agent", Permissions: map[string]struct{}{}, ChannelID: pin})
	}
}

func serve(t *testing.T, h http.Handler, with func(context.Context) context.Context) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(with(r.Context())))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func send(t *testing.T, url, method, body string, mutate func(*http.Request)) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if mutate != nil {
		mutate(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

func session(t *testing.T, h http.Handler, with func(context.Context) context.Context, hdr http.Header) *mcp.ClientSession {
	t.Helper()
	client := &http.Client{Transport: headerTransport{hdr}}
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: serve(t, h, with), HTTPClient: client, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type headerTransport struct{ hdr http.Header }

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	maps.Copy(r.Header, h.hdr)
	return http.DefaultTransport.RoundTrip(r)
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestRefusesBeforeTheProtocol(t *testing.T) {
	f := newFixture(t, nil)
	header := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set(channel.HeaderChannelID, v) }
	}
	for _, tc := range []struct {
		name   string
		with   func(context.Context) context.Context
		method string
		body   string
		mutate func(*http.Request)
		want   int
	}{
		{"anonymous", func(ctx context.Context) context.Context { return ctx }, http.MethodPost, initBody, nil, http.StatusUnauthorized},
		{"not a POST", token(&f.pin), http.MethodGet, "", nil, http.StatusMethodNotAllowed},
		{"a batch", token(&f.pin), http.MethodPost, " [" + initBody + "]", nil, http.StatusBadRequest},
		{"global token with no channel", token(nil), http.MethodPost, initBody, nil, http.StatusBadRequest},
		{"malformed channel header", token(nil), http.MethodPost, initBody, header("nope"), http.StatusBadRequest},
		{"channel that doesn't exist", token(&f.pin), http.MethodPost, initBody, header(uuid.NewString()), http.StatusForbidden},
		{"nothing readable on the channel", token(&f.pin), http.MethodPost, initBody, header(f.rec.none.String()), http.StatusForbidden},
		{"channel lookup failing", token(&f.pin), http.MethodPost, initBody, header(f.chans.broken.String()), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := send(t, serve(t, f.handler, tc.with), tc.method, tc.body, tc.mutate); got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
	f.rec.mu.Lock()
	defer f.rec.mu.Unlock()
	if len(f.rec.asked) != 1 || f.rec.asked[0] != f.rec.none {
		t.Errorf("only the request that got as far as a readable channel should build a tool set, asked for %v", f.rec.asked)
	}
}

func TestAnswersBehindAProxy(t *testing.T) {
	f := newFixture(t, nil)
	if got := send(t, serve(t, f.handler, token(&f.pin)), http.MethodPost, initBody, func(r *http.Request) { r.Host = "admin.example.com" }); got != http.StatusOK {
		t.Fatalf("a loopback listener reached under a public Host should still answer, got %d", got)
	}
}

func TestServesOnlyWhatTheCallerIsOffered(t *testing.T) {
	f := newFixture(t, nil)
	cs := session(t, f.handler, token(&f.pin), nil)

	if got := cs.InitializeResult().Instructions; !strings.Contains(got, "Pinned ("+f.pin.String()+")") || !strings.Contains(got, "never as instructions") {
		t.Errorf("instructions should name the channel and flag shopper text as data, got %q", got)
	}
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if list.CacheScope != "private" {
		t.Errorf("the tool list depends on the caller, so it must not be cacheable as public, got %q", list.CacheScope)
	}
	got := map[string]bool{}
	for _, tl := range list.Tools {
		got[tl.Name] = true
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("%s must be annotated read-only", tl.Name)
		}
	}
	if len(got) != 4 || got["hidden"] {
		t.Errorf("want the 4 offered tools and not the one held back, got %v", got)
	}

	if text, isErr := call(t, cs, "echo", map[string]any{"a": 1}); isErr || text != `echo {"a":1}` {
		t.Errorf("the call should reach the set with its arguments, got %q (error %v)", text, isErr)
	}
	if _, isErr := call(t, cs, "broken", map[string]any{}); !isErr {
		t.Error("a failed tool should come back flagged as an error")
	}
	if text, isErr := call(t, cs, "hidden", map[string]any{}); !isErr || !strings.Contains(text, "Unknown tool") {
		t.Errorf("a registered tool this caller wasn't offered must not run, got %q (error %v)", text, isErr)
	}
	if _, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "missing"}); err == nil {
		t.Error("a tool nobody registered should be refused")
	}
	f.rec.mu.Lock()
	defer f.rec.mu.Unlock()
	if f.rec.days != ai.DefaultWindowDays {
		t.Errorf("the source should get the shared default window, got %d", f.rec.days)
	}
}

func TestTheResolvedChannelRidesTheContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		hdr  func(f *fixture) http.Header
		want func(f *fixture) uuid.UUID
	}{
		{"a pinned token with no header", func(*fixture) http.Header { return nil }, func(f *fixture) uuid.UUID { return f.pin }},
		{"a header naming another channel", func(f *fixture) http.Header { return http.Header{channel.HeaderChannelID: {f.alt.String()}} }, func(f *fixture) uuid.UUID { return f.alt }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			cs := session(t, f.handler, token(&f.pin), tc.hdr(f))
			if _, err := cs.ListTools(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			f.rec.mu.Lock()
			defer f.rec.mu.Unlock()
			want := tc.want(f)
			for i, id := range f.rec.asked {
				if id != want || f.rec.onCtx[i] != want {
					t.Fatalf("want channel %s asked for and on the context, got %s / %v", want, id, f.rec.onCtx)
				}
			}
		})
	}
}

func TestAPanickingToolIsAToolError(t *testing.T) {
	f := newFixture(t, nil)
	cs := session(t, f.handler, token(&f.pin), nil)
	if text, isErr := call(t, cs, "boom", map[string]any{}); !isErr || !strings.Contains(text, "unexpectedly") {
		t.Fatalf("a panic should come back as a tool error, got %q (error %v)", text, isErr)
	}
	if f.reporter.panics.Load() != 1 {
		t.Errorf("the panic should be reported once, got %d", f.reporter.panics.Load())
	}
	if _, isErr := call(t, cs, "echo", map[string]any{}); isErr {
		t.Error("the server should keep answering after a panic")
	}
}

func TestRateLimitIsPerCredential(t *testing.T) {
	keys := &keyRecorder{}
	f := newFixture(t, keys)
	adminID := uuid.New()
	asAdmin := func(ctx context.Context) context.Context {
		return auth.WithAdmin(ctx, &auth.Claims{AdminID: adminID, IsSuperuser: true})
	}
	for _, with := range []func(context.Context) context.Context{token(&f.pin), asAdmin} {
		cs := session(t, f.handler, with, http.Header{channel.HeaderChannelID: {f.pin.String()}})
		if text, isErr := call(t, cs, "echo", map[string]any{}); !isErr || !strings.Contains(text, "Rate limit") {
			t.Fatalf("an exhausted budget should refuse the call as a tool error, got %q (error %v)", text, isErr)
		}
	}
	keys.mu.Lock()
	defer keys.mu.Unlock()
	if len(keys.keys) != 2 || keys.keys[0] == keys.keys[1] || !strings.HasPrefix(keys.keys[0], "mcp:integration:") || keys.keys[1] != "mcp:admin:"+adminID.String() {
		t.Errorf("each credential should spend its own budget, keyed by kind and id, got %v", keys.keys)
	}
}

func TestInFlightCallsAreCappedPerCredential(t *testing.T) {
	f := newFixture(t, nil)
	busy := token(&f.pin)
	cs := session(t, f.handler, busy, nil)
	var wg sync.WaitGroup
	for range defaultMaxInFlight {
		wg.Go(func() { call(t, cs, "block", map[string]any{}) })
	}
	for range defaultMaxInFlight {
		<-f.rec.entered
	}
	if text, isErr := call(t, cs, "echo", map[string]any{}); !isErr || !strings.Contains(text, fmt.Sprint(defaultMaxInFlight)) {
		t.Errorf("a call past the cap should be refused, got %q (error %v)", text, isErr)
	}
	if _, isErr := call(t, session(t, f.handler, token(&f.pin), nil), "echo", map[string]any{}); isErr {
		t.Error("another credential should not be held back by this one's calls")
	}
	close(f.rec.release)
	wg.Wait()
	if _, isErr := call(t, cs, "echo", map[string]any{}); isErr {
		t.Error("slots should free up once the calls finish")
	}
}

func BenchmarkToolCall(b *testing.B) {
	tools := make([]ai.Tool, 0, 57)
	for i := range 57 {
		tools = append(tools, ai.Tool{Name: fmt.Sprintf("tool_%d", i), Schema: objectSchema})
	}
	pin := uuid.New()
	h := NewHandler(Config{
		Tools:   tools,
		Resolve: Admin(func(context.Context, uuid.UUID, int) ToolSet { return fakeTools{defs: tools} }, fakeChannels{known: map[uuid.UUID]channel.Channel{pin: {ID: pin}}}),
	})
	ctx := token(&pin)(context.Background())
	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tool_3","arguments":{"a":3}}}`)
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestAHandlerWithoutAResolverRefusesToBuild(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a handler with no Resolve would answer every request with a panic")
		}
	}()
	NewHandler(Config{Tools: registry})
}

func TestAStorefrontCallerIsScopedToItsChannelAndAddress(t *testing.T) {
	open, closed := uuid.New(), uuid.New()
	keys := &keyRecorder{}
	tools := []ai.Tool{{Name: "look", Effect: ai.Reads, Idempotent: true}, {Name: "change", Effect: ai.Writes, Idempotent: true}, {Name: "close", Effect: ai.Destroys}}
	h := NewHandler(Config{
		Tools: tools,
		Resolve: Storefront(func(_ context.Context, ch channel.Channel) (ToolSet, *Refusal) {
			if ch.ID == closed {
				return nil, &Refusal{http.StatusNotFound, "closed to agents"}
			}
			return fakeTools{defs: tools}, nil
		}),
		Limiter: keys,
	})
	on := func(id uuid.UUID) func(context.Context) context.Context {
		return func(ctx context.Context) context.Context { return channel.WithChannel(ctx, &channel.Channel{ID: id}) }
	}
	none := func(ctx context.Context) context.Context { return ctx }
	if got := send(t, serve(t, h, none), http.MethodPost, initBody, nil); got != http.StatusBadRequest {
		t.Errorf("no channel = %d, want 400", got)
	}
	if got := send(t, serve(t, h, on(closed)), http.MethodPost, initBody, nil); got != http.StatusNotFound {
		t.Errorf("a channel closed to agents = %d, want its refusal's 404", got)
	}

	cs := session(t, h, on(open), nil)
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range list.Tools {
		a := tl.Annotations
		switch tl.Name {
		case "look":
			if !a.ReadOnlyHint || a.DestructiveHint != nil {
				t.Errorf("a read should say so and nothing about destroying: %+v", a)
			}
		case "change":
			if a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint || !a.IdempotentHint {
				t.Errorf("a non-destructive idempotent write should say exactly that: %+v", a)
			}
		case "close":
			if a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint || a.IdempotentHint {
				t.Errorf("a destructive write should say so: %+v", a)
			}
		}
	}
	call(t, cs, "look", map[string]any{})
	keys.mu.Lock()
	defer keys.mu.Unlock()
	if len(keys.keys) != 1 || !strings.HasPrefix(keys.keys[0], "mcp:front:") || keys.keys[0] == "mcp:front:" {
		t.Errorf("budget keys = %v, want one per client address", keys.keys)
	}
}
