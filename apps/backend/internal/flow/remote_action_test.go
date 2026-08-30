package flow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

func remoteFlowStack(t *testing.T, url string) *Service {
	t.Helper()
	rpSvc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	rpSvc.AllowPrivateTargets = true
	if _, err := rpSvc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindFlowAction, Slug: "enrich", Name: "Enrich", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	s := &Service{}
	s.SetRemoteActions(rpSvc, remoteprovider.NewInvoker(rpSvc, nil))
	return s
}

func TestExecRemoteActionMergesContextAndBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"context": {"tier": "gold", "score": 9}, "branch": "yes"}`))
	}))
	defer srv.Close()

	s := remoteFlowStack(t, srv.URL)
	run := &FlowRun{FlowID: uuid.New(), CustomerID: uuid.New(), Context: json.RawMessage(`{"existing": true}`)}
	node := Node{ID: "n1", Type: NodeRemoteAction, Data: NodeData{Provider: "enrich"}}

	branch := s.execRemoteAction(context.Background(), node, run)
	if branch != "yes" {
		t.Fatalf("branch = %q, want yes", branch)
	}
	var merged map[string]any
	if err := json.Unmarshal(run.Context, &merged); err != nil {
		t.Fatal(err)
	}
	if merged["tier"] != "gold" || merged["existing"] != true {
		t.Fatalf("context not merged (existing preserved + new keys): %v", merged)
	}
}

func TestExecRemoteActionFailOpen(t *testing.T) {
	s := remoteFlowStack(t, "http://127.0.0.1:9/down")
	run := &FlowRun{FlowID: uuid.New(), CustomerID: uuid.New(), Context: json.RawMessage(`{"a": 1}`)}
	node := Node{ID: "n1", Type: NodeRemoteAction, Data: NodeData{Provider: "enrich"}}

	branch := s.execRemoteAction(context.Background(), node, run)
	if branch != "" {
		t.Fatalf("down provider should pick no branch, got %q", branch)
	}
	// context untouched on failure
	if string(run.Context) != `{"a": 1}` {
		t.Fatalf("context mutated on failure: %s", run.Context)
	}
}

func TestExecRemoteActionNoProviderConfigured(t *testing.T) {
	s := &Service{} // no remote providers wired
	run := &FlowRun{FlowID: uuid.New(), CustomerID: uuid.New()}
	if b := s.execRemoteAction(context.Background(), Node{Data: NodeData{Provider: "enrich"}}, run); b != "" {
		t.Fatalf("unwired service should no-op, got branch %q", b)
	}
}

func TestMergeContext(t *testing.T) {
	// patch overrides + adds
	got := mergeContext(json.RawMessage(`{"a":1,"b":2}`), json.RawMessage(`{"b":3,"c":4}`))
	var m map[string]any
	_ = json.Unmarshal(got, &m)
	if m["a"] != float64(1) || m["b"] != float64(3) || m["c"] != float64(4) {
		t.Fatalf("bad merge: %v", m)
	}
	// non-object patch is ignored, base preserved
	base := json.RawMessage(`{"a":1}`)
	if string(mergeContext(base, json.RawMessage(`"nope"`))) != `{"a":1}` {
		t.Fatal("non-object patch should be ignored")
	}
	// empty base + patch works
	got = mergeContext(nil, json.RawMessage(`{"x":1}`))
	_ = json.Unmarshal(got, &m)
	if m["x"] != float64(1) {
		t.Fatalf("empty base merge failed: %v", m)
	}
}
