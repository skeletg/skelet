// Package aitrace sends the store's AI calls to Langfuse on its own OpenTelemetry pipeline, apart from the server's
// tracing - Langfuse never sees HTTP or database spans, and the operator's collector never sees what shoppers asked
package aitrace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
)

const (
	EnabledPath   = "ai/langfuse/enabled"
	HostPath      = "ai/langfuse/host"
	PublicKeyPath = "ai/langfuse/public_key"
	SecretKeyPath = "ai/langfuse/secret_key"

	DefaultHost = "https://cloud.langfuse.com"
	otelPath    = "/api/public/otel/v1/traces"
	scoresPath  = "/api/public/scores"
	maxScoring  = 8
	recheck     = 30 * time.Second
	maxText     = 20000
)

type Settings interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

var idle = noop.NewTracerProvider().Tracer("")

type Sink struct {
	cfg     Settings
	mu      sync.Mutex
	built   string
	tp      *sdktrace.TracerProvider
	tracer  trace.Tracer
	checked time.Time
	warned  string
	host    string // where scores go, with auth - set while the pipeline runs
	auth    string
	http    *http.Client
	scoring chan struct{}
	pending sync.WaitGroup
}

func New(cfg Settings) *Sink {
	return &Sink{cfg: cfg, tracer: idle, http: safeClient(), scoring: make(chan struct{}, maxScoring)}
}

func safeClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: urlguard.SafeTransport(urlguard.AllowPrivateFromEnv())}
}

func Fixed(tp trace.TracerProvider) *Sink {
	return &Sink{tracer: tp.Tracer("skelet/ai"), checked: time.Now().Add(100 * 365 * 24 * time.Hour)}
}

func (s *Sink) current(ctx context.Context) trace.Tracer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.checked) < recheck {
		return s.tracer
	}
	s.checked = time.Now()
	endpoint, auth, ok := s.read(ctx)
	if !ok {
		s.swap(nil, "")
		s.host, s.auth = "", ""
		return s.tracer
	}
	s.host, s.auth = strings.TrimSuffix(endpoint, otelPath), auth
	if sig := endpoint + "|" + auth; sig != s.built {
		exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithHTTPClient(safeClient()),
			otlptracehttp.WithHeaders(map[string]string{"Authorization": "Basic " + auth, "x-langfuse-ingestion-version": "4"}))
		if err != nil {
			s.warn(endpoint, "aitrace: can't reach Langfuse at that address, so AI calls aren't sent", err)
			s.swap(nil, "")
			return s.tracer
		}
		res := resource.NewSchemaless(attribute.String("service.name", "skelet"))
		s.swap(sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res)), sig)
	}
	return s.tracer
}

func (s *Sink) read(ctx context.Context) (endpoint, auth string, ok bool) {
	on, err := s.cfg.GetBool(ctx, EnabledPath, config.DefaultScope)
	if err != nil || !on {
		return "", "", false
	}
	get := func(path string) string {
		v, _ := s.cfg.GetString(ctx, path, config.DefaultScope)
		return strings.TrimSpace(v)
	}
	host, public, secret := strings.TrimRight(get(HostPath), "/"), get(PublicKeyPath), s.secret(ctx)
	if host == "" {
		host = DefaultHost
	}
	u, err := url.Parse(host)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		s.warn(host, "aitrace: the Langfuse address isn't an http(s) URL, so AI calls aren't sent", err)
		return "", "", false
	}
	if public == "" || secret == "" {
		s.warn("keys", "aitrace: Langfuse is on but a key is missing, so AI calls aren't sent", nil)
		return "", "", false
	}
	return host + otelPath, base64.StdEncoding.EncodeToString([]byte(public + ":" + secret)), true
}

type entryReader interface {
	Get(ctx context.Context, path string, scope config.Scope) (config.Entry, error)
}

func (s *Sink) secret(ctx context.Context) string {
	er, ok := s.cfg.(entryReader)
	if !ok {
		v, _ := s.cfg.GetString(ctx, SecretKeyPath, config.DefaultScope)
		return strings.TrimSpace(v)
	}
	key, err := er.Get(ctx, SecretKeyPath, config.DefaultScope)
	if err != nil {
		return ""
	}
	host, err := er.Get(ctx, HostPath, config.DefaultScope)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(config.BoundSecret(key, host))
}

func (s *Sink) swap(tp *sdktrace.TracerProvider, sig string) {
	if old := s.tp; old != nil && old != tp {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = old.Shutdown(ctx)
		}()
	}
	s.tp, s.built = tp, sig
	if tp == nil {
		s.tracer = idle
		return
	}
	s.tracer = tp.Tracer("skelet/ai")
	s.warned = ""
}

func (s *Sink) warn(key, msg string, err error) {
	if s.warned == key {
		return
	}
	s.warned = key
	slog.Warn(msg, "err", err)
}

func (s *Sink) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	tp := s.tp
	s.tp, s.built, s.tracer, s.host = nil, "", idle, ""
	s.mu.Unlock()
	s.pending.Wait()
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}

var installed atomic.Pointer[Sink]

func Install(s *Sink) { installed.Store(s) }

func tracer(ctx context.Context) trace.Tracer {
	if s := installed.Load(); s != nil {
		return s.current(ctx)
	}
	return idle
}

type contentKey struct{}

// WithContent says whether what's written in this context may be sent - only a conversation the store keeps may
func WithContent(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, contentKey{}, allowed)
}

func contentAllowed(ctx context.Context) bool {
	v, _ := ctx.Value(contentKey{}).(bool)
	return v
}

type Kind string

const (
	Agent      Kind = "agent"
	Generation Kind = "generation"
	Tool       Kind = "tool"
	Embedding  Kind = "embedding"
	Span       Kind = "span"
)

type Obs struct {
	span    trace.Span
	content bool
}

type parentKey struct{}

type versionKey struct{}

func WithVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, versionKey{}, version)
}

// open keeps the observation under its own key, never as the active span - HTTP and database spans below would join this pipeline otherwise
func open(ctx context.Context, name string, kind Kind, opts ...trace.SpanStartOption) (context.Context, *Obs) {
	base := context.Background()
	if parent, ok := ctx.Value(parentKey{}).(trace.Span); ok {
		base = trace.ContextWithSpan(base, parent)
	}
	opts = append(opts, trace.WithAttributes(attribute.String("langfuse.observation.type", string(kind))))
	if v, _ := ctx.Value(versionKey{}).(string); v != "" {
		opts = append(opts, trace.WithAttributes(attribute.String("langfuse.version", v)))
	}
	_, span := tracer(ctx).Start(base, name, opts...)
	return context.WithValue(ctx, parentKey{}, span), &Obs{span: span, content: contentAllowed(ctx)}
}

func Start(ctx context.Context, name string, kind Kind) (context.Context, *Obs) {
	return open(ctx, name, kind)
}

func Child(ctx context.Context, name string, kind Kind) (context.Context, *Obs) {
	if _, ok := ctx.Value(parentKey{}).(trace.Span); !ok {
		return ctx, &Obs{span: trace.SpanFromContext(context.Background())}
	}
	return open(ctx, name, kind)
}

func StartAt(ctx context.Context, name string, kind Kind, at time.Time) (context.Context, *Obs) {
	return open(ctx, name, kind, trace.WithTimestamp(at))
}

func (o *Obs) on() bool { return o != nil && o.span.IsRecording() }

func (o *Obs) KeepsText() bool { return o.on() && o.content }

func (o *Obs) Trace(name, session, user string, tags ...string) {
	if !o.on() {
		return
	}
	o.span.SetAttributes(attribute.String("langfuse.trace.name", name))
	if len(tags) > 0 {
		o.span.SetAttributes(attribute.StringSlice("langfuse.trace.tags", tags))
	}
	if !o.content {
		return
	}
	if session != "" {
		o.span.SetAttributes(attribute.String("langfuse.session.id", session))
	}
	if user != "" {
		o.span.SetAttributes(attribute.String("langfuse.user.id", user))
	}
}

func (o *Obs) Meta(key string, value any) {
	if !o.on() {
		return
	}
	k := "langfuse.observation.metadata." + key
	switch v := value.(type) {
	case string:
		o.span.SetAttributes(attribute.String(k, v))
	case bool:
		o.span.SetAttributes(attribute.Bool(k, v))
	case int:
		o.span.SetAttributes(attribute.Int(k, v))
	case int64:
		o.span.SetAttributes(attribute.Int64(k, v))
	default:
		o.span.SetAttributes(attribute.String(k, text(v)))
	}
}

// Input and Output carry what was written, and are dropped unless the conversation is one the store keeps
func (o *Obs) Input(v any) {
	if o.on() && o.content {
		o.span.SetAttributes(attribute.String("langfuse.observation.input", text(v)))
	}
}

func (o *Obs) Output(v any) {
	if o.on() && o.content {
		o.span.SetAttributes(attribute.String("langfuse.observation.output", text(v)))
	}
}

func (o *Obs) Model(name string) {
	if o.on() && name != "" {
		o.span.SetAttributes(attribute.String("gen_ai.request.model", name))
	}
}

func (o *Obs) Usage(input, output, cacheRead, cacheWrite int64) {
	if !o.on() {
		return
	}
	details, _ := json.Marshal(map[string]int64{"input": input, "output": output, "cache_read_input_tokens": cacheRead, "cache_creation_input_tokens": cacheWrite})
	o.span.SetAttributes(
		attribute.Int64("gen_ai.usage.input_tokens", input+cacheRead+cacheWrite),
		attribute.Int64("gen_ai.usage.output_tokens", output),
		attribute.String("langfuse.observation.usage_details", string(details)),
	)
}

func (o *Obs) Fail(err error) {
	if !o.on() || err == nil {
		return
	}
	o.span.SetAttributes(attribute.String("langfuse.observation.level", "ERROR"), attribute.String("langfuse.observation.status_message", clip(err.Error())))
	o.span.SetStatus(codes.Error, clip(err.Error()))
}

// TraceID outlives the observation, so an outcome known later can still score it
func (o *Obs) TraceID() string {
	if o == nil {
		return ""
	}
	if sc := o.span.SpanContext(); sc.IsValid() && sc.IsSampled() {
		return sc.TraceID().String()
	}
	return ""
}

func (o *Obs) End() {
	if o != nil {
		o.span.End()
	}
}

func (o *Obs) EndAt(at time.Time) {
	if o != nil {
		o.span.End(trace.WithTimestamp(at))
	}
}

func text(v any) string {
	if s, ok := v.(string); ok {
		return clip(s)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return clip(string(raw))
}

func clip(s string) string {
	if len(s) <= maxText {
		return s
	}
	n := maxText
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

type Outcome struct {
	Name    string
	Value   bool
	Comment string
}

// Score sends outcomes for a trace in the background - an id per trace and name, so a replayed event scores once
func Score(ctx context.Context, traceID string, outcomes ...Outcome) {
	if s := installed.Load(); s != nil && traceID != "" && len(outcomes) > 0 {
		s.score(ctx, traceID, outcomes)
	}
}

func (s *Sink) score(ctx context.Context, traceID string, outcomes []Outcome) {
	s.current(ctx)
	s.mu.Lock()
	host, auth := s.host, s.auth
	s.mu.Unlock()
	if host == "" || s.http == nil {
		return
	}
	body := make([]map[string]any, len(outcomes))
	for i, o := range outcomes {
		v := 0
		if o.Value {
			v = 1
		}
		body[i] = map[string]any{"id": traceID + "-" + o.Name, "traceId": traceID, "name": o.Name, "value": v, "dataType": "BOOLEAN"}
		if o.Comment != "" {
			body[i]["comment"] = o.Comment
		}
	}
	select {
	case s.scoring <- struct{}{}:
	default:
		return
	}
	s.pending.Add(1)
	go func() {
		defer func() { <-s.scoring; s.pending.Done() }()
		ctx := context.WithoutCancel(ctx)
		status := s.post(ctx, host+scoresPath, auth, body)
		// a Langfuse older than the batch endpoint takes one score a request
		if status >= 400 && status < 500 && len(body) > 1 {
			for _, one := range body {
				status = s.post(ctx, host+scoresPath, auth, one)
			}
		}
		if status >= 300 {
			slog.Warn("aitrace: Langfuse refused a score", "trace", traceID, "status", status)
		}
	}()
}

func (s *Sink) post(ctx context.Context, url, auth string, body any) int {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+auth)
	res, err := s.http.Do(req)
	if err != nil {
		slog.Warn("aitrace: a score didn't reach Langfuse", "err", err)
		return 0
	}
	_ = res.Body.Close()
	return res.StatusCode
}
