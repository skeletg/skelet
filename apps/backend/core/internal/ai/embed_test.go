package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func vector(n int, fill float32) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = fill
	}
	return v
}

// An OpenAI-shaped /embeddings that answers in reverse order, as the API is free to, with a vector per input
func embeddingsServer(t *testing.T, size int, refuse string, seen *[]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "wrong path or key", http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		*seen = append(*seen, body)
		if _, asked := body[refuse]; asked && refuse != "" {
			http.Error(w, `{"error":"unexpected field: `+refuse+`"}`, http.StatusBadRequest)
			return
		}
		input := body["input"].([]any)
		data := make([]map[string]any, len(input))
		for i := range input {
			j := len(input) - 1 - i
			data[i] = map[string]any{"index": j, "embedding": vector(size, float32(j))}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7}})
	}))
}

func embedder(t *testing.T, base, model string) *Embedder {
	t.Helper()
	t.Setenv("APP_ENV", "development") // the stub listens on loopback, which the SSRF guard allows only in development
	return NewService(stubCfg{strs: map[string]string{cfgEmbedModel: model, cfgEmbedBaseURL: base + "/v1", cfgEmbedAPIKey: "k"}}).Embedder()
}

func TestEmbedAsksForTheStoresSizeAndKeepsTheOrder(t *testing.T) {
	var seen []map[string]any
	srv := embeddingsServer(t, EmbeddingDimensions, "", &seen)
	defer srv.Close()
	vecs, tokens, err := embedder(t, srv.URL, "text-embedding-3-small").Embed(t.Context(), ForDocument, []string{"first", "second", strings.Repeat("x", MaxEmbedChars+50)})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 || vecs[0][0] != 0 || vecs[1][0] != 1 || vecs[2][0] != 2 || tokens != 7 {
		t.Errorf("vectors out of order or tokens lost: %v %v %v, %d", vecs[0][0], vecs[1][0], vecs[2][0], tokens)
	}
	body := seen[0]
	if body["model"] != "text-embedding-3-small" || body["dimensions"] != float64(EmbeddingDimensions) {
		t.Errorf("asked with %v", body)
	}
	if _, sent := body["encoding_format"]; sent {
		t.Error("asked for an encoding_format - some providers refuse the word, and every one gives floats by default")
	}
	if got := body["input"].([]any)[2].(string); len([]rune(got)) != MaxEmbedChars {
		t.Errorf("a long text went out at %d characters", len([]rune(got)))
	}
}

func TestAServerThatRefusesTheSizeFieldIsAskedWithoutIt(t *testing.T) {
	var seen []map[string]any
	srv := embeddingsServer(t, EmbeddingDimensions, "dimensions", &seen)
	defer srv.Close()
	e := embedder(t, srv.URL, "embed-1")
	for range 2 {
		if _, _, err := e.Embed(t.Context(), ForDocument, []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("%d requests, want the refusal, its retry, then straight without", len(seen))
	}
	if _, asked := seen[2]["dimensions"]; asked {
		t.Error("the endpoint that refused the field was asked with it again")
	}
}

func TestAModelOfAnotherSizeIsRefused(t *testing.T) {
	var seen []map[string]any
	srv := embeddingsServer(t, 768, "", &seen)
	defer srv.Close()
	if _, _, err := embedder(t, srv.URL, "nomic-embed-text").Embed(t.Context(), ForDocument, []string{"a"}); !errors.Is(err, ErrEmbeddingSize) {
		t.Errorf("err = %v, want ErrEmbeddingSize", err)
	}
}

func TestEmbeddingsStayOffWithoutAModelOrWithAIOff(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer srv.Close()
	off := embedder(t, srv.URL, "")
	if off.Model(t.Context()) != "" {
		t.Error("a store with no model reads as on")
	}
	if _, _, err := off.Embed(t.Context(), ForDocument, []string{"a"}); err == nil {
		t.Error("embedding with no model went through")
	}
	t.Setenv("AI_ENABLED", "false")
	if NewService(stubCfg{strs: map[string]string{cfgEmbedModel: "m"}}).Embedder().Model(t.Context()) != "" {
		t.Error("AI_ENABLED=false left embeddings on")
	}
	if _, _, err := embedder(t, srv.URL, "m").Embed(t.Context(), ForDocument, make([]string, MaxEmbedBatch+1)); err == nil {
		t.Error("an oversized batch was sent")
	}
	if calls.Load() != 0 {
		t.Errorf("%d requests reached the provider", calls.Load())
	}
}

func TestAServerIsToldADocumentFromAQuestionUntilItTurnsTheFieldDown(t *testing.T) {
	var seen []map[string]any
	srv := embeddingsServer(t, EmbeddingDimensions, "", &seen)
	defer srv.Close()
	e := embedder(t, srv.URL, "embed-1")
	for _, p := range []Purpose{ForDocument, ForQuery} {
		if _, _, err := e.Embed(t.Context(), p, []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 2 || seen[0]["input_type"] != "document" || seen[1]["input_type"] != "query" {
		t.Errorf("asked with %v", seen)
	}

	var strict []map[string]any
	other := embeddingsServer(t, EmbeddingDimensions, "input_type", &strict)
	defer other.Close()
	e = embedder(t, other.URL, "embed-1")
	for range 2 {
		if _, _, err := e.Embed(t.Context(), ForQuery, []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, asked := strict[2]["input_type"]; len(strict) != 3 || asked || strict[2]["dimensions"] == nil {
		t.Errorf("a server that refuses the purpose = %v - want it asked again without, never with again, the size still asked", strict)
	}
}

func TestOnlyAnAnswerAboutTheTextCountsAsRefused(t *testing.T) {
	for status, want := range map[int]bool{400: true, 413: true, 422: true, 401: false, 403: false, 404: false, 408: false, 429: false, 500: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":"no"}`, status)
		}))
		_, _, err := embedder(t, srv.URL, "m").Embed(t.Context(), ForDocument, []string{"a shirt"})
		srv.Close()
		if err == nil || errors.Is(err, ErrEmbeddingRefused) != want {
			t.Errorf("HTTP %d = %v - want refused %v", status, err, want)
		}
	}
}
