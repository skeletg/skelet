package front

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

func streamRequest(method, contentType, body string) *http.Request {
	r := httptest.NewRequest(method, "/api/front/assistant", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return r
}

func streamLines(t *testing.T, body string) []map[string]json.RawMessage {
	t.Helper()
	var out []map[string]json.RawMessage
	for line := range strings.SplitSeq(strings.TrimSpace(body), "\n") {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q isn't JSON: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestTheStreamSendsStepsAndTheCartBeforeTheReply(t *testing.T) {
	cart := uuid.New()
	var got streamInput
	h := assistantStream(func(_ context.Context, in streamInput, n shoptools.Notify) (*model.AssistantReply, error) {
		got = in
		n.Step(shoptools.Step{Tool: "search_products", Detail: "mugs"})
		n.Cart(cart)
		return &model.AssistantReply{Reply: "Added two mugs."}, nil
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, streamRequest(http.MethodPost, "application/json; charset=utf-8", `{"messages":[{"role":"user","content":"two mugs"}],"page":"/mugs"}`))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/x-ndjson" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, headers %v", w.Code, w.Header())
	}
	if len(got.Messages) != 1 || got.Page == nil || *got.Page != "/mugs" {
		t.Errorf("the turn was asked with %+v", got)
	}
	lines := streamLines(t, w.Body.String())
	if len(lines) != 3 || lines[0]["step"] == nil || lines[1]["cart"] == nil || lines[2]["reply"] == nil {
		t.Fatalf("lines = %s", w.Body.String())
	}
	if !strings.Contains(string(lines[1]["cart"]), cart.String()) {
		t.Errorf("cart line = %s", lines[1]["cart"])
	}
}

func TestTheStreamEndsOnTheErrorCodeTheMutationWouldGive(t *testing.T) {
	for err, want := range map[error]string{shoptools.ErrAssistantLimited: "ASSISTANT_RATE_LIMITED", errors.New("db down"): "INTERNAL"} {
		h := assistantStream(func(context.Context, streamInput, shoptools.Notify) (*model.AssistantReply, error) { return nil, err })
		w := httptest.NewRecorder()
		h.ServeHTTP(w, streamRequest(http.MethodPost, "application/json", `{"messages":[]}`))
		lines := streamLines(t, w.Body.String())
		if w.Code != http.StatusOK || len(lines) != 1 || !strings.Contains(string(lines[0]["error"]), want) {
			t.Errorf("%v: %d %s", err, w.Code, w.Body.String())
		}
	}
}

func TestTheStreamRefusesWhatABrowserCouldSendFromAnotherSite(t *testing.T) {
	asked := false
	h := assistantStream(func(context.Context, streamInput, shoptools.Notify) (*model.AssistantReply, error) {
		asked = true
		return &model.AssistantReply{}, nil
	})
	for name, tc := range map[string]struct {
		method, contentType, body string
		want                      int
	}{
		"a GET":             {http.MethodGet, "application/json", "", http.StatusMethodNotAllowed},
		"a plain-text form": {http.MethodPost, "text/plain", `{"messages":[]}`, http.StatusUnsupportedMediaType},
		"no content type":   {http.MethodPost, "", `{"messages":[]}`, http.StatusUnsupportedMediaType},
		"not JSON":          {http.MethodPost, "application/json", `{nope`, http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, streamRequest(tc.method, tc.contentType, tc.body))
		if w.Code != tc.want {
			t.Errorf("%s answered %d, want %d", name, w.Code, tc.want)
		}
	}
	if asked {
		t.Error("a refused request still asked the assistant")
	}
}
