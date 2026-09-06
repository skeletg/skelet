// Package ai is a thin, provider-pluggable LLM client for the CMS content assistant - SEO meta, image alt-text, structure-preserving page translation - and it is SUGGESTION-ONLY, since callers drop results into editable fields a human reviews and saves
// Ships dark: disabled by default and a no-op until an operator picks a provider and sets a key, with AI_ENABLED=false as the hard kill switch above the DB config that also hides every dashboard AI surface and skips the AI scheduler jobs and review-classify subscriber
// Two providers hand-rolled over net/http, no SDK: "anthropic" for the Claude Messages API, "openai" for any OpenAI-compatible Chat Completions endpoint (Azure, Gemini's OpenAI endpoint, OpenRouter, Groq, Together, local Ollama / LM Studio / vLLM), selected by base_url
package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/otelx"
	"github.com/skeletg/skelet/apps/backend/internal/webhook"
)

var ErrNotConfigured = errors.New("ai: not configured (enable AI and set a provider + API key in Settings → AI)") // resolvers surface it as a friendly "AI not configured"

var ErrDisabled = errors.New("ai: disabled on this server (AI_ENABLED=false)") // unlike ErrNotConfigured, an operator can't fix this from the admin UI

const (
	anthropicVersion = "2023-06-01"
	maxBodyChars     = 6000    // cost guard on page text fed to the model
	maxImageBytes    = 5 << 20 // 5 MiB - common per-image ceiling
)

// The slice of *config.Service this package needs - enabled flag, provider, decrypted API key, model and base URL are all read at request time, so the admin UI can change them without a redeploy
type ConfigReader interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

type Service struct {
	cfg        ConfigReader
	http       *http.Client
	envEnabled bool // AI_ENABLED hard switch - read once at construction, restart to change
}

func NewService(cfg ConfigReader) *Service {
	// base_url is operator-configured to reach local LLM servers, so requests go through the SSRF-guarded transport that refuses private and internal addresses at dial time, like every other admin-configured outbound path
	allowPrivate := os.Getenv("APP_ENV") == "development" // only dev, where pointing at localhost is the intended workflow
	envEnabled := true                                    // parsed here, not in cmd/server, so every binary constructing the service respects it - CLI reclassify included
	if v := os.Getenv("AI_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			envEnabled = b
		}
	}
	return &Service{
		cfg:        cfg,
		http:       &http.Client{Timeout: 60 * time.Second, Transport: otelx.HTTPTransport(webhook.SafeTransport(allowPrivate))},
		envEnabled: envEnabled,
	}
}

func (s *Service) Enabled() bool { return s.envEnabled } // the AI_ENABLED env switch, not the DB toggle - feature queries and job registration key off this, Available() folds in both

type providerCfg struct {
	provider       string
	apiKey         string
	model          string
	visionModel    string
	baseURL        string
	baseConfigured bool // a base_url was explicitly set (allows keyless local endpoints)
}

func (s *Service) get(ctx context.Context, path string) string {
	if s.cfg == nil {
		return ""
	}
	v, err := s.cfg.GetString(ctx, path, config.DefaultScope)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func (s *Service) enabled(ctx context.Context) bool {
	if !s.envEnabled || s.cfg == nil {
		return false
	}
	v, err := s.cfg.GetBool(ctx, cfgEnabled, config.DefaultScope)
	return err == nil && v
}

func (s *Service) providerConfig(ctx context.Context) providerCfg {
	prov := s.get(ctx, cfgProvider)
	if prov == "" {
		prov = providerAnthropic
	}
	key := s.get(ctx, cfgAPIKey) // key precedence: config → generic AI_API_KEY env → provider-specific env
	if key == "" {
		key = strings.TrimSpace(os.Getenv("AI_API_KEY"))
	}
	if key == "" {
		switch prov {
		case providerOpenAI:
			key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		default:
			key = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
	}
	rawBase := s.get(ctx, cfgBaseURL)
	base := rawBase
	if base == "" {
		base = defaultBaseURL(prov)
	}
	model := s.get(ctx, cfgModel)
	if model == "" {
		model = defaultModel
	}
	return providerCfg{
		provider:       prov,
		apiKey:         key,
		model:          model,
		visionModel:    s.get(ctx, cfgVisionModel),
		baseURL:        strings.TrimRight(base, "/"),
		baseConfigured: rawBase != "",
	}
}

func defaultBaseURL(provider string) string {
	if provider == providerOpenAI {
		return "https://api.openai.com/v1"
	}
	return "https://api.anthropic.com"
}

func (pc providerCfg) hasCredentials() bool {
	return pc.apiKey != "" || pc.baseConfigured // an explicit base_url counts, for keyless local endpoints like Ollama
}

// Enabled AND credentials present - the dashboard shows or hides Generate buttons on this, and mutations re-check
func (s *Service) Available(ctx context.Context) bool {
	return s.enabled(ctx) && s.providerConfig(ctx).hasCredentials()
}

// ---- provider-neutral request ----

type contentBlock struct {
	text  string
	image *imageData
}

type imageData struct {
	mime string
	b64  string
}

func textBlock(s string) contentBlock { return contentBlock{text: s} }

func (s *Service) complete(ctx context.Context, model, system string, content []contentBlock, maxTokens int) (string, error) {
	if !s.envEnabled {
		return "", ErrDisabled
	}
	if !s.enabled(ctx) {
		return "", ErrNotConfigured
	}
	pc := s.providerConfig(ctx)
	if !pc.hasCredentials() {
		return "", ErrNotConfigured
	}
	switch pc.provider {
	case providerOpenAI:
		return s.openaiComplete(ctx, pc, model, system, content, maxTokens)
	default:
		return s.anthropicComplete(ctx, pc, model, system, content, maxTokens)
	}
}

// Sends a JSON body to url with the given headers and returns the response bytes, wrapping non-2xx as an error with a bounded snippet
func (s *Service) post(ctx context.Context, url string, headers map[string]string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ai: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ai: %s HTTP %d: %s", hostOf(url), resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func hostOf(url string) string {
	u := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	if before, _, ok := strings.Cut(u, "/"); ok {
		return before
	}
	return u
}

// ---- Anthropic (Messages API) ----

func (s *Service) anthropicComplete(ctx context.Context, pc providerCfg, model, system string, content []contentBlock, maxTokens int) (string, error) {
	blocks := make([]map[string]any, 0, len(content))
	for _, b := range content {
		if b.image != nil {
			blocks = append(blocks, map[string]any{
				"type":   "image",
				"source": map[string]any{"type": "base64", "media_type": b.image.mime, "data": b.image.b64},
			})
		} else {
			blocks = append(blocks, map[string]any{"type": "text", "text": b.text})
		}
	}
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   []map[string]any{{"role": "user", "content": blocks}},
	}
	data, err := s.post(ctx, pc.baseURL+"/v1/messages", map[string]string{
		"x-api-key":         pc.apiKey,
		"anthropic-version": anthropicVersion,
	}, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("ai: decode: %w", err)
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// ---- OpenAI-compatible (Chat Completions) ----

func (s *Service) openaiComplete(ctx context.Context, pc providerCfg, model, system string, content []contentBlock, maxTokens int) (string, error) {
	messages := make([]map[string]any, 0, 2)
	if system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}
	parts := make([]map[string]any, 0, len(content))
	for _, b := range content {
		if b.image != nil {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", b.image.mime, b.image.b64)},
			})
		} else {
			parts = append(parts, map[string]any{"type": "text", "text": b.text})
		}
	}
	messages = append(messages, map[string]any{"role": "user", "content": parts})
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   messages,
	}
	headers := map[string]string{}
	if pc.apiKey != "" {
		headers["Authorization"] = "Bearer " + pc.apiKey
	}
	data, err := s.post(ctx, pc.baseURL+"/chat/completions", headers, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("ai: decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("ai: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// ---- High-level generation (provider-agnostic) ----

type SEOSuggestion struct { // suggestion only - a human reviews and saves it
	MetaTitle       string
	MetaDescription string
}

func (s *Service) GenerateSEO(ctx context.Context, title, bodyText string) (SEOSuggestion, error) { // bodyText gets truncated as a cost guard
	system := "You are an SEO assistant for an e-commerce CMS. Given a page's title and content, " +
		"write a compelling, keyword-relevant SEO meta title (max 60 characters) and meta description " +
		"(140-160 characters). Respond with ONLY minified JSON of the form " +
		`{"metaTitle":"...","metaDescription":"..."} - no prose, no code fences.`
	user := fmt.Sprintf("Page title: %s\n\nPage content:\n%s", title, truncate(bodyText, maxBodyChars))
	raw, err := s.complete(ctx, s.providerConfig(ctx).model, system, []contentBlock{textBlock(user)}, 300)
	if err != nil {
		return SEOSuggestion{}, err
	}
	var parsed struct {
		MetaTitle       string `json:"metaTitle"`
		MetaDescription string `json:"metaDescription"`
	}
	if err := json.Unmarshal([]byte(stripFences(raw)), &parsed); err != nil {
		return SEOSuggestion{MetaDescription: raw}, nil // no clean JSON - fall back to the raw text as the description
	}
	return SEOSuggestion{
		MetaTitle:       strings.TrimSpace(parsed.MetaTitle),
		MetaDescription: strings.TrimSpace(parsed.MetaDescription),
	}, nil
}

// Up to n subject-line options for an email, given a short description of its content - an empty slice on any parse failure, so the caller shows nothing rather than erroring the editor
func (s *Service) SuggestEmailSubjects(ctx context.Context, context string, n int) ([]string, error) {
	if n <= 0 || n > 10 {
		n = 5
	}
	system := fmt.Sprintf("You are an email-marketing copywriter. Given a description of an email, "+
		"write %d short, high-open-rate subject lines (each under 60 characters, varied in angle, "+
		"no spam-trigger words, no surrounding quotes). Respond with ONLY a minified JSON array of "+
		"strings - no prose, no code fences.", n)
	raw, err := s.complete(ctx, s.providerConfig(ctx).model, system, []contentBlock{textBlock(truncate(context, maxBodyChars))}, 300)
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal([]byte(stripFences(raw)), &out); err != nil {
		return nil, nil // model didn't return clean JSON - no suggestions
	}
	cleaned := make([]string, 0, len(out))
	for _, s := range out {
		if t := strings.TrimSpace(s); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	return cleaned, nil
}

// Rewrites a block of email copy per an instruction like "make it punchier" or "friendlier tone"
func (s *Service) RewriteEmailText(ctx context.Context, text, instruction string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("ai: nothing to rewrite")
	}
	if strings.TrimSpace(instruction) == "" {
		instruction = "improve the clarity and impact"
	}
	system := "You are an email-marketing copywriter. Rewrite the user's email copy to " + instruction +
		". Keep any {{merge.tags}} intact. Respond with ONLY the rewritten copy - no prose, no quotes, no code fences."
	raw, err := s.complete(ctx, s.providerConfig(ctx).model, system, []contentBlock{textBlock(truncate(text, maxBodyChars))}, 600)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripFences(raw)), nil
}

// Writes concise alt text for an image sent as base64, using the configured vision model and falling back to the main one
func (s *Service) GenerateAltText(ctx context.Context, mimeType string, image []byte) (string, error) {
	if len(image) == 0 {
		return "", fmt.Errorf("ai: empty image")
	}
	if len(image) > maxImageBytes {
		return "", fmt.Errorf("ai: image too large (%d bytes, max %d)", len(image), maxImageBytes)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("ai: not an image (%s)", mimeType)
	}
	pc := s.providerConfig(ctx)
	model := pc.visionModel
	if model == "" {
		model = pc.model
	}
	system := "You write concise, descriptive alt text for images on an e-commerce site. " +
		"Describe the image in at most 125 characters for screen readers and SEO. " +
		"Respond with ONLY the alt text - no surrounding quotes and no prefix like 'Image of'."
	content := []contentBlock{
		{image: &imageData{mime: mimeType, b64: base64.StdEncoding.EncodeToString(image)}},
		textBlock("Write alt text for this image."),
	}
	return s.complete(ctx, model, system, content, 150)
}

// Translates a list of strings between locales, returning the same length in the same order so a caller can map them back onto a structure - used by structure-preserving page translation
func (s *Service) Translate(ctx context.Context, sourceLocale, targetLocale string, items []string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("ai: marshal items: %w", err)
	}
	system := fmt.Sprintf(
		"You are a professional translator. Translate each string in the given JSON array from locale %q to locale %q. "+
			"Preserve meaning, tone, and any inline HTML, placeholders, or URLs unchanged. "+
			"Respond with ONLY a JSON array of exactly %d translated strings in the same order - no prose, no code fences.",
		sourceLocale, targetLocale, len(items))
	raw, err := s.complete(ctx, s.providerConfig(ctx).model, system, []contentBlock{textBlock(string(itemsJSON))}, 4096)
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal([]byte(stripFences(raw)), &out); err != nil {
		return nil, fmt.Errorf("ai: translate parse: %w", err)
	}
	if len(out) != len(items) {
		return nil, fmt.Errorf("ai: translate returned %d items, expected %d", len(out), len(items))
	}
	return out, nil
}

type ChatMessage struct {
	Role        string // "user", "assistant" or "tool"
	Text        string
	ToolCalls   []ToolCall   // assistant turn: tools the model asked to call
	ToolResults []ToolResult // tool turn: results returned to the model
}

func chatRole(r string) string {
	if r == "assistant" {
		return "assistant"
	}
	return "user"
}

func (s *Service) Chat(ctx context.Context, system string, messages []ChatMessage, maxTokens int) (string, error) { // text-only - ChatWithTools is the tool-aware form
	if !s.enabled(ctx) {
		return "", ErrNotConfigured
	}
	pc := s.providerConfig(ctx)
	if !pc.hasCredentials() {
		return "", ErrNotConfigured
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("ai: no messages")
	}
	if pc.provider == providerOpenAI {
		return s.openaiChat(ctx, pc, system, messages, maxTokens)
	}
	return s.anthropicChat(ctx, pc, system, messages, maxTokens)
}

func (s *Service) anthropicChat(ctx context.Context, pc providerCfg, system string, messages []ChatMessage, maxTokens int) (string, error) {
	msgs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		msgs = append(msgs, map[string]any{"role": chatRole(m.Role), "content": m.Text})
	}
	payload := map[string]any{
		"model":      pc.model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   msgs,
	}
	data, err := s.post(ctx, pc.baseURL+"/v1/messages", map[string]string{
		"x-api-key":         pc.apiKey,
		"anthropic-version": anthropicVersion,
	}, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("ai: decode: %w", err)
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func (s *Service) openaiChat(ctx context.Context, pc providerCfg, system string, messages []ChatMessage, maxTokens int) (string, error) {
	msgs := make([]map[string]any, 0, len(messages)+1)
	if system != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": system})
	}
	for _, m := range messages {
		msgs = append(msgs, map[string]any{"role": chatRole(m.Role), "content": m.Text})
	}
	payload := map[string]any{"model": pc.model, "max_tokens": maxTokens, "messages": msgs}
	headers := map[string]string{}
	if pc.apiKey != "" {
		headers["Authorization"] = "Bearer " + pc.apiKey
	}
	data, err := s.post(ctx, pc.baseURL+"/chat/completions", headers, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("ai: decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("ai: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// An operator-defined topic the classifier may assign - it lives here, not in the review package, so `ai` stays a leaf and the review service maps its own records onto this shape (same for ModerationTagOption below)
type ReviewCategoryOption struct {
	Slug        string
	Name        string
	Description string
}

type ModerationTagOption struct { // one operator-defined moderation concern the classifier may flag
	Slug        string
	Name        string
	Description string
}

// A suggestion only, since the moderator still decides
type ModerationVerdict struct {
	Reason     string
	Categories []string // topic category slugs (validated against the supplied options)
	Tags       []string // moderation-concern slugs (validated against the supplied options)
	Parsed     bool     // whether the response was understood at all - "understood, nothing applies" is Parsed true with empty slices
}

// Parsed=false on an unparseable response, and the caller leaves the review as-is
func (s *Service) ModerateReview(ctx context.Context, title, body string, categories []ReviewCategoryOption, tags []ModerationTagOption) (ModerationVerdict, error) {
	var b strings.Builder
	b.WriteString("You are a review-moderation assistant for an e-commerce store. ")
	if len(tags) > 0 {
		b.WriteString("Flag any moderation concerns that apply by returning their slugs in a \"tags\" array " +
			"(return an empty array if the review is a clean, genuine product opinion with no concerns):\n")
		for _, t := range tags {
			fmt.Fprintf(&b, "- %s: %s\n", t.Slug, t.Description)
		}
	} else {
		b.WriteString("Return an empty \"tags\" array. ")
	}
	catField := ""
	if len(categories) > 0 {
		b.WriteString("Also assign ALL categories that clearly apply by their slugs in a \"categories\" array " +
			"(a review can belong to several; return an empty array if none fit):\n")
		for _, c := range categories {
			fmt.Fprintf(&b, "- %s: %s\n", c.Slug, c.Description)
		}
		catField = `,"categories":["<slug>", ...]`
	}
	b.WriteString(" Respond with ONLY minified JSON " +
		`{"tags":["<slug>", ...],"reason":"<one short sentence>"` + catField + `} - no prose, no code fences.`)

	user := fmt.Sprintf("Title: %s\n\nReview: %s", title, truncate(body, maxBodyChars))
	raw, err := s.complete(ctx, s.providerConfig(ctx).model, b.String(), []contentBlock{textBlock(user)}, 300)
	if err != nil {
		return ModerationVerdict{}, err
	}
	var parsed struct {
		Tags       []string `json:"tags"`
		Reason     string   `json:"reason"`
		Categories []string `json:"categories"`
	}
	if err := json.Unmarshal([]byte(stripFences(raw)), &parsed); err != nil {
		return ModerationVerdict{}, nil // Parsed=false
	}
	out := ModerationVerdict{Reason: strings.TrimSpace(parsed.Reason), Parsed: true}
	// The model can hallucinate slugs, so each list is validated against its own options - categories against categories, tags against tags
	out.Categories = validateSlugs(parsed.Categories, func(i int) string { return categories[i].Slug }, len(categories))
	out.Tags = validateSlugs(parsed.Tags, func(i int) string { return tags[i].Slug }, len(tags))
	return out, nil
}

func validateSlugs(in []string, slugAt func(int) string, n int) []string { // trims, drops empties and dupes, keeps only allowed slugs - order-stable
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" || seen[s] || !slugAllowed(s, slugAt, n) {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func slugAllowed(slug string, slugAt func(int) string, n int) bool {
	for i := range n {
		if slugAt(i) == slug {
			return true
		}
	}
	return false
}

// A balanced 2-3 sentence "what customers say" summary from a product's reviews - the joined input is truncated as a cost guard
func (s *Service) SummarizeReviews(ctx context.Context, productName string, reviews []string) (string, error) {
	if len(reviews) == 0 {
		return "", fmt.Errorf("ai: no reviews to summarize")
	}
	system := "You summarize customer reviews for an e-commerce product page. Write a balanced, neutral 2-3 sentence " +
		"\"what customers say\" summary covering common praise AND common complaints. Use ONLY information present in the " +
		"reviews - invent nothing, cite no numbers you can't see. Plain text, no markdown, no preamble."
	user := fmt.Sprintf("Product: %s\n\nReviews:\n%s", productName, truncate(strings.Join(reviews, "\n---\n"), 8000))
	return s.complete(ctx, s.providerConfig(ctx).model, system, []contentBlock{textBlock(user)}, 250)
}

// Drafts a short, professional merchant reply to a review - a suggestion the merchant edits before posting
func (s *Service) DraftReviewReply(ctx context.Context, productName, title, body string, rating int) (string, error) {
	system := "You draft a brief, professional, on-brand reply from the store to a customer review. Thank the customer, " +
		"acknowledge specifics they raised, and be helpful. Do NOT make promises (refunds, discounts) or invent policies. " +
		"2-4 sentences, plain text, no preamble, no signature placeholder."
	user := fmt.Sprintf("Product: %s\nRating: %d/5\nReview title: %s\nReview: %s", productName, rating, title, truncate(body, maxBodyChars))
	return s.complete(ctx, s.providerConfig(ctx).model, system, []contentBlock{textBlock(user)}, 300)
}

// ---- helpers ----

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// Removes a leading ``` or ```json fence and trailing ``` when the model wrapped its JSON despite instructions
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}
