package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const (
	cfgEmbedModel   = "ai/embeddings/model"
	cfgEmbedBaseURL = "ai/embeddings/base_url"
	cfgEmbedAPIKey  = "ai/embeddings/api_key"

	EmbeddingDimensions = 1024
	MaxEmbedBatch       = 64
	MaxEmbedChars       = 6000
)

func init() {
	config.RegisterGroup(config.Group{Section: "ai", Slug: "embeddings", Label: "Embeddings", SortOrder: 3})
	config.Register(config.Definition{Path: cfgEmbedModel, Type: config.TypeString,
		Label:       "Embedding model (blank = off), e.g. text-embedding-3-small, voyage-3.5, mxbai-embed-large",
		Description: fmt.Sprintf("Turns product and page text into vectors so the shopping assistant can find things by meaning. It has to give vectors of %d numbers - asked for where the provider can resize them. It's a provider of its own, since the one that writes the answers may not make embeddings. Changing the model embeds everything again.", EmbeddingDimensions)})
	config.Register(config.Definition{Path: cfgEmbedBaseURL, Type: config.TypeString,
		Label: "Embeddings API base URL (blank = OpenAI) - any OpenAI-compatible /embeddings endpoint: Voyage, Azure, Ollama, LM Studio"})
	config.Register(config.Definition{Path: cfgEmbedAPIKey, Type: config.TypeSecret,
		Label: "Embeddings API key (blank for a local server; OPENAI_API_KEY is used for OpenAI's own endpoint when this is empty)"})
}

type Embedder struct {
	svc     *Service
	refused sync.Map // base|model|field: an optional field a server turned down, never asked of it again
}

func (s *Service) Embedder() *Embedder { return &Embedder{svc: s} }

type embedCfg struct {
	model, base, key string
}

func (e *Embedder) cfg(ctx context.Context) embedCfg {
	c := embedCfg{model: e.svc.get(ctx, cfgEmbedModel), base: strings.TrimRight(e.svc.get(ctx, cfgEmbedBaseURL), "/"), key: e.svc.secret(ctx, cfgEmbedAPIKey, cfgEmbedBaseURL)}
	if c.base == "" {
		c.base = defaultBaseURL(providerOpenAI)
		if c.key == "" {
			c.key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	}
	return c
}

func (e *Embedder) Model(ctx context.Context) string {
	if !e.svc.envEnabled {
		return ""
	}
	return e.cfg(ctx).model
}

var ErrEmbeddingSize = errors.New("ai: the embedding model answers in a different size")

// ErrEmbeddingRefused is the provider turning the request down as asked - asking again won't help, unlike a 429 or a 5xx
var ErrEmbeddingRefused = errors.New("ai: the embedding provider refused the request")

type refused struct{ error }

func (r refused) Is(target error) bool { return target == ErrEmbeddingRefused }
func (r refused) Unwrap() error        { return r.error }

// Only answers about the text itself - a bad key, a wrong model or a timeout fails the whole pass instead
func refusedText(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// Purpose tells a search question from a document, which some providers find more with - asked of every server until
// one turns the field down
type Purpose string

const (
	ForDocument Purpose = "document"
	ForQuery    Purpose = "query"
)

// The request fields a server may not take, and the words its refusal names them by
var optionalEmbedFields = map[string]string{"dimensions": "dimension", "input_type": "input_type"}

func (e *Embedder) Embed(ctx context.Context, purpose Purpose, texts []string) ([][]float32, int64, error) {
	c := e.cfg(ctx)
	switch {
	case !e.svc.envEnabled || c.model == "":
		return nil, 0, errors.New("ai: embeddings are off")
	case len(texts) == 0:
		return nil, 0, nil
	case len(texts) > MaxEmbedBatch:
		return nil, 0, fmt.Errorf("ai: %d texts in one embedding request, the most is %d", len(texts), MaxEmbedBatch)
	}
	input := make([]string, len(texts))
	for i, t := range texts {
		input[i] = truncateRunes(t, MaxEmbedChars)
	}
	ctx, obs := aitrace.Child(ctx, "Embedding", aitrace.Embedding)
	defer obs.End()
	obs.Model(c.model)
	obs.Meta("texts", len(texts))
	obs.Meta("purpose", string(purpose))
	ask := func(field string) bool {
		_, no := e.refused.Load(c.base + "|" + c.model + "|" + field)
		return !no
	}
	var (
		vecs   [][]float32
		tokens int64
		err    error
	)
	for range len(optionalEmbedFields) + 1 {
		var status int
		vecs, tokens, status, err = e.post(ctx, c, purpose, input, ask)
		field := turnedDown(status, err, ask)
		if field == "" {
			break
		}
		e.refused.Store(c.base+"|"+c.model+"|"+field, true)
	}
	obs.Usage(tokens, 0, 0, 0)
	obs.Fail(err)
	return vecs, tokens, err
}

// turnedDown is the optional field a refusal names, so the request goes again without it
func turnedDown(status int, err error, asked func(string) bool) string {
	if err == nil || status < 400 || status >= 500 {
		return ""
	}
	msg := strings.ToLower(err.Error())
	for field, word := range optionalEmbedFields {
		if asked(field) && strings.Contains(msg, word) {
			return field
		}
	}
	return ""
}

func (e *Embedder) post(ctx context.Context, c embedCfg, purpose Purpose, input []string, ask func(string) bool) ([][]float32, int64, int, error) {
	// no encoding_format: floats are every provider's default, and some refuse the word
	body := map[string]any{"model": c.model, "input": input}
	if purpose != "" && ask("input_type") {
		body["input_type"] = string(purpose)
	}
	if ask("dimensions") {
		body["dimensions"] = EmbeddingDimensions
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/embeddings", bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	res, err := e.svc.http.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("ai: embeddings request to %s: %w", hostOf(c.base), err)
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, 0, res.StatusCode, err
	}
	if res.StatusCode != http.StatusOK {
		err := fmt.Errorf("ai: embeddings %d from %s: %s", res.StatusCode, hostOf(c.base), truncateRunes(strings.TrimSpace(string(payload)), 300))
		if refusedText(res.StatusCode) {
			err = refused{err}
		}
		return nil, 0, res.StatusCode, err
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int64 `json:"prompt_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, 0, res.StatusCode, fmt.Errorf("ai: embeddings answer from %s isn't JSON: %w", hostOf(c.base), err)
	}
	if len(out.Data) != len(input) {
		return nil, 0, res.StatusCode, fmt.Errorf("ai: asked for %d embeddings, got %d", len(input), len(out.Data))
	}
	vecs := make([][]float32, len(input))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) || vecs[d.Index] != nil {
			return nil, 0, res.StatusCode, errors.New("ai: embeddings answer with a missing or repeated index")
		}
		if len(d.Embedding) != EmbeddingDimensions {
			return nil, 0, res.StatusCode, fmt.Errorf("%w: %d numbers, the store keeps %d - pick a model that gives %d", ErrEmbeddingSize, len(d.Embedding), EmbeddingDimensions, EmbeddingDimensions)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, max(out.Usage.TotalTokens, out.Usage.PromptTokens), res.StatusCode, nil
}
