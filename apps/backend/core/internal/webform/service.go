package webform

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Publisher is the slice of the event bus this package emits to
type Publisher interface {
	Publish(ctx context.Context, e events.Event)
}

// ConfigReader reads the (global) captcha + master-switch config at request time
type ConfigReader interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

// FileStore presigns uploads and authoritatively inspects a stored object (size + content type) so the server can validate a file the
// client claims to have uploaded - Backed by the media Storage; optional (nil = uploads disabled / strict validation skipped)
type FileStore interface {
	PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (url string, err error)
	Head(ctx context.Context, objectKey string) (size int64, mime string, err error)
	MaxUploadBytes() int64
}

type Service struct {
	repo    Repository
	cfg     ConfigReader
	bus     Publisher
	captcha *CaptchaVerifier
	files   FileStore
}

func New(repo Repository, cfg ConfigReader) *Service {
	return &Service{repo: repo, cfg: cfg, captcha: NewCaptchaVerifier(cfg)}
}

// SetDeps wires the submit-time collaborators after construction (avoids an
// import cycle with the event bus / media service)
func (s *Service) SetDeps(bus Publisher, files FileStore) {
	s.bus = bus
	s.files = files
}

// uploadTTL bounds how long a presigned form-upload PUT URL is valid
const uploadTTL = 15 * time.Minute

// PresignUpload issues a presigned PUT URL for a file field on a form - The object key is server-built under a per-form/field prefix from a
// sanitized filename (no path traversal) - Returns the URL + the object key the client echoes back on submit
func (s *Service) PresignUpload(ctx context.Context, formID uuid.UUID, fieldKey, filename, contentType string, sizeBytes int64) (url, objectKey string, err error) {
	if s.files == nil {
		return "", "", fmt.Errorf("%w: uploads are not configured", ErrInvalidSubmission)
	}
	form, err := s.repo.Get(ctx, formID)
	if err != nil {
		return "", "", err
	}
	if form.Status != StatusActive {
		return "", "", ErrFormInactive
	}
	// The field must exist and be a file field
	var field *Field
	for i := range form.Fields {
		if form.Fields[i].Key == fieldKey && form.Fields[i].Type == FieldFile {
			field = &form.Fields[i]
			break
		}
	}
	if field == nil {
		return "", "", fmt.Errorf("%w: unknown file field", ErrInvalidSubmission)
	}
	// The declared size gets signed into the PUT URL, so this cap is enforceable - an
	// anonymous caller could otherwise fill the bucket through a public form
	if sizeBytes <= 0 {
		return "", "", fmt.Errorf("%w: file size is required", ErrInvalidSubmission)
	}
	maxBytes := s.files.MaxUploadBytes()
	if field.MaxFileBytes > 0 && field.MaxFileBytes < maxBytes {
		maxBytes = field.MaxFileBytes
	}
	if sizeBytes > maxBytes {
		return "", "", fmt.Errorf("%w: file exceeds the %d byte limit", ErrInvalidSubmission, maxBytes)
	}
	objectKey = fmt.Sprintf("webforms/%s/%s/%s-%s", form.ID, fieldKey, uuid.New(), sanitizeFilename(filename))
	url, err = s.files.PresignUpload(ctx, objectKey, contentType, sizeBytes, uploadTTL)
	if err != nil {
		return "", "", fmt.Errorf("presign upload: %w", err)
	}
	return url, objectKey, nil
}

var unsafeFilename = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if _, base, found := strings.CutLast(name, "/"); found {
		name = base // strip any path component
	}
	name = unsafeFilename.ReplaceAllString(name, "_")
	name = strings.TrimLeft(name, ".") // no leading dots (hidden/traversal)
	if name == "" {
		name = "upload"
	}
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	return name
}

// --- admin CRUD ---

func (s *Service) Create(ctx context.Context, channelID uuid.UUID, in SaveInput) (Form, error) {
	in = normalizeSaveInput(in)
	if in.Name == "" {
		return Form{}, fmt.Errorf("%w: name is required", ErrInvalidSchema)
	}
	if err := validateRedirectURL(in.Settings.RedirectURL); err != nil {
		return Form{}, err
	}
	if err := ValidateSchema(in.Fields); err != nil {
		return Form{}, err
	}
	return s.repo.Create(ctx, channelID, in)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in SaveInput) (Form, error) {
	in = normalizeSaveInput(in)
	if err := validateRedirectURL(in.Settings.RedirectURL); err != nil {
		return Form{}, err
	}
	if err := ValidateSchema(in.Fields); err != nil {
		return Form{}, err
	}
	// Slug is immutable after create (keeps CMS block references stable): reuse
	// the stored slug regardless of input
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Form{}, err
	}
	in.Slug = existing.Slug
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Form, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Form, error) {
	return s.repo.List(ctx, channelID, includeDeleted)
}

// SoftDelete marks the row deleted the form - The storefront stops serving it (the by-slug lookup filters deleted) and
// it drops out of the dashboard list, but its submissions stay readable
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

func (s *Service) Restore(ctx context.Context, id uuid.UUID) error {
	return s.repo.Restore(ctx, id)
}

func (s *Service) ListSubmissions(ctx context.Context, formID uuid.UUID, limit, offset int32) ([]Submission, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListSubmissions(ctx, formID, limit, offset)
}

func (s *Service) GetSubmission(ctx context.Context, id uuid.UUID) (Submission, error) {
	return s.repo.GetSubmission(ctx, id)
}

func (s *Service) CountSubmissions(ctx context.Context, formID uuid.UUID) (int64, error) {
	return s.repo.CountSubmissions(ctx, formID)
}

// --- public projection (storefront) ---

// PublicForm is the storefront-safe view: fields + display settings + the captcha provider/site-key, with all
// admin-only settings (notify emails, confirmation email internals, flow/consent config) stripped
type PublicForm struct {
	Form
	CaptchaProvider string
	CaptchaSiteKey  string
}

func (s *Service) PublicForm(ctx context.Context, id uuid.UUID) (PublicForm, error) {
	// cms/forms/enabled is the feature's master switch - off means the storefront renders no form at all, which is what the label has always claimed. Checked before per-form status so switching the feature off doesn't require deactivating every form one by one
	if enabled, err := s.cfg.GetBool(ctx, CfgEnabled, config.DefaultScope); err == nil && !enabled {
		return PublicForm{}, ErrFormInactive
	}
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return PublicForm{}, err
	}
	if f.Status != StatusActive {
		return PublicForm{}, ErrFormInactive
	}
	// Strip admin-only settings; keep only what the widget renders
	f.Settings = FormSettings{
		SubmitLabel:    f.Settings.SubmitLabel,
		SuccessMessage: f.Settings.SuccessMessage,
		RedirectURL:    f.Settings.RedirectURL,
		RequireCaptcha: f.Settings.RequireCaptcha,
	}
	out := PublicForm{Form: f}
	if f.Settings.RequireCaptcha {
		provider, _ := s.cfg.GetString(ctx, CfgCaptchaProvider, config.DefaultScope)
		provider = strings.TrimSpace(strings.ToLower(provider))
		if provider != "" && provider != "none" {
			out.CaptchaProvider = provider
			out.CaptchaSiteKey, _ = s.cfg.GetString(ctx, CfgCaptchaSiteKey, config.DefaultScope)
		}
	}
	return out, nil
}

// --- submission ---

type SubmitInput struct {
	FormID       uuid.UUID
	ChannelID    uuid.UUID
	Values       map[string]any
	Files        []SubmissionFile
	Honeypot     string
	CaptchaToken string
	CustomerID   *uuid.UUID
	RemoteIP     string
	Meta         SubmissionMeta
}

type SubmitResult struct {
	SuccessMessage string
	RedirectURL    string
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	// 1) Honeypot: a real user never fills the hidden field
	if strings.TrimSpace(in.Honeypot) != "" {
		return SubmitResult{}, ErrSpamRejected
	}

	// Same switch on the write path - a form id captured before the feature was switched off must not
	// keep accepting submissions
	if enabled, cerr := s.cfg.GetBool(ctx, CfgEnabled, config.DefaultScope); cerr == nil && !enabled {
		return SubmitResult{}, ErrFormInactive
	}

	form, err := s.repo.Get(ctx, in.FormID)
	if err != nil {
		return SubmitResult{}, err
	}
	if form.Status != StatusActive {
		return SubmitResult{}, ErrFormInactive
	}
	if form.ChannelID != in.ChannelID {
		// The form belongs to another channel
		return SubmitResult{}, ErrFormNotFound
	}

	// 2) CAPTCHA (when the form requires it) - Unconfigured provider → allow but warn
	if form.Settings.RequireCaptcha && s.captcha != nil {
		ok, configured, verr := s.captcha.Verify(ctx, in.CaptchaToken, in.RemoteIP)
		switch {
		case verr != nil:
			slog.ErrorContext(ctx, "webform: captcha verify error", "form", form.ID, "err", verr)
			return SubmitResult{}, ErrSpamRejected
		case !configured:
			slog.WarnContext(ctx, "webform: form requires captcha but no provider is configured; allowing", "form", form.ID)
		case !ok:
			return SubmitResult{}, ErrSpamRejected
		}
	}

	if in.Values == nil {
		in.Values = map[string]any{}
	}

	// Recompute visible fields server-side from the submitted values, so a client cannot bypass a required field by hiding it
	// Hidden fields are never trusted or stored
	visible := VisibleFields(form.Fields, in.Values)

	// 4) Build the cleaned answer set (visible input fields only; hidden fields
	// take their server-side default)
	answers := make(map[string]any, len(visible))
	for _, f := range visible {
		if f.Type == FieldHidden {
			answers[f.Key] = f.DefaultValue
			continue
		}
		if v, ok := in.Values[f.Key]; ok {
			answers[f.Key] = v
		}
	}

	// 5) Validate
	fieldErrs := validateAnswers(visible, answers)
	files, fileErrs := s.validateFiles(ctx, visible, in.Files)
	fieldErrs = append(fieldErrs, fileErrs...)
	if len(fieldErrs) > 0 {
		return SubmitResult{}, &SubmissionError{Fields: fieldErrs}
	}

	// 6) Persist
	sub, err := s.repo.RecordSubmission(ctx, Submission{
		FormID:     form.ID,
		ChannelID:  form.ChannelID,
		Answers:    answers,
		Files:      files,
		CustomerID: in.CustomerID,
		Meta:       in.Meta,
	})
	if err != nil {
		return SubmitResult{}, err
	}

	// 7) Fire the post-commit event (notification + emails + flow)
	if s.bus != nil {
		s.bus.Publish(ctx, &FormSubmitted{
			SubmissionID:   sub.ID,
			FormID:         form.ID,
			ChannelID:      form.ChannelID,
			FormName:       form.Name,
			FormSlug:       form.Slug,
			Answers:        answers,
			SubmitterEmail: submitterEmail(form, answers),
			NotifyEmails:   form.Settings.NotifyEmails,
			Confirmation:   form.Settings.Confirmation,
			FlowTrigger:    form.Settings.FlowTrigger,
			Summary:        summarize(form, answers),
		})
	}

	msg := form.Settings.SuccessMessage
	if msg == "" {
		msg = "Thanks - your response has been received."
	}
	return SubmitResult{SuccessMessage: msg, RedirectURL: form.Settings.RedirectURL}, nil
}

// --- validation ---

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func validateAnswers(visible []Field, answers map[string]any) []FieldError {
	var errs []FieldError
	add := func(key, msg string) { errs = append(errs, FieldError{Field: key, Message: msg}) }

	for _, f := range visible {
		if f.Type == FieldFile || f.Type == FieldHidden {
			continue // files validated separately; hidden are server-set
		}
		v := answers[f.Key]
		empty := isEmptyAnswer(v)

		if f.Type == FieldConsent {
			if f.Required && !truthy(v) {
				add(f.Key, "This must be checked.")
			}
			continue
		}
		if f.Type == FieldMatrix {
			validateMatrix(f, v, add)
			continue
		}
		if f.Type == FieldRepeater {
			validateRepeater(f, v, add)
			continue
		}
		if empty {
			if f.Required {
				add(f.Key, "This field is required.")
			}
			continue
		}

		switch f.Type {
		case FieldEmail:
			if sv, ok := v.(string); ok && !emailRe.MatchString(strings.TrimSpace(sv)) {
				add(f.Key, "Enter a valid email address.")
			}
		case FieldURL:
			if sv, ok := v.(string); ok {
				if u, err := url.Parse(strings.TrimSpace(sv)); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
					add(f.Key, "Enter a valid URL.")
				}
			}
		case FieldNumber:
			n, ok := toFloat(v)
			if !ok {
				add(f.Key, "Enter a number.")
				break
			}
			if f.Validation != nil {
				if f.Validation.Min != nil && n < *f.Validation.Min {
					add(f.Key, fmt.Sprintf("Must be at least %v.", *f.Validation.Min))
				}
				if f.Validation.Max != nil && n > *f.Validation.Max {
					add(f.Key, fmt.Sprintf("Must be at most %v.", *f.Validation.Max))
				}
			}
		case FieldSelect, FieldRadio:
			if sv, ok := v.(string); ok && !optionAllowed(f, sv) {
				add(f.Key, "Select a valid option.")
			}
		case FieldCheckboxGroup:
			for _, sv := range toStringList(v) {
				if !optionAllowed(f, sv) {
					add(f.Key, "Select valid options.")
					break
				}
			}
		}

		// Length + pattern apply to string-valued fields
		if sv, ok := v.(string); ok && f.Validation != nil {
			sv = strings.TrimSpace(sv)
			if f.Validation.MinLength != nil && len([]rune(sv)) < *f.Validation.MinLength {
				add(f.Key, fmt.Sprintf("Must be at least %d characters.", *f.Validation.MinLength))
			}
			if f.Validation.MaxLength != nil && len([]rune(sv)) > *f.Validation.MaxLength {
				add(f.Key, fmt.Sprintf("Must be at most %d characters.", *f.Validation.MaxLength))
			}
			if f.Validation.Pattern != "" {
				if re, err := regexp.Compile(f.Validation.Pattern); err == nil && !re.MatchString(sv) {
					add(f.Key, "Invalid format.")
				}
			}
		}
	}
	return errs
}

// validateMatrix enforces per-row completeness (when required) and that every
// selection is a defined column
func validateMatrix(f Field, v any, add func(key, msg string)) {
	m, _ := v.(map[string]any)
	for _, row := range f.Rows {
		rv := m[row.Key]
		if isEmptyAnswer(rv) {
			if f.Required {
				label := row.Label
				if label == "" {
					label = row.Key
				}
				add(f.Key, "Please answer: "+label)
			}
			continue
		}
		for _, sel := range toStringList(rv) {
			if !columnAllowed(f, sel) {
				add(f.Key, "Invalid selection in the grid.")
				break
			}
		}
	}
}

func columnAllowed(f Field, v string) bool {
	for _, c := range f.Columns {
		if c.Value == v {
			return true
		}
	}
	return false
}

// validateRepeater enforces the instance-count bounds and validates each
// instance's visible sub-fields (conditional visibility recomputed per item)
func validateRepeater(f Field, v any, add func(key, msg string)) {
	items, _ := v.([]any)
	minItems := 0
	if f.Required && f.MinItems == nil {
		minItems = 1
	}
	if f.MinItems != nil {
		minItems = *f.MinItems
	}
	if len(items) < minItems {
		add(f.Key, fmt.Sprintf("Add at least %d.", minItems))
	}
	if f.MaxItems != nil && len(items) > *f.MaxItems {
		add(f.Key, fmt.Sprintf("Add at most %d.", *f.MaxItems))
	}
	for i, it := range items {
		itemMap, ok := it.(map[string]any)
		if !ok {
			continue
		}
		visSub := VisibleFields(f.SubFields, itemMap)
		for _, e := range validateAnswers(visSub, itemMap) {
			add(f.Key, fmt.Sprintf("Item %d: %s", i+1, e.Message))
		}
	}
}

func (s *Service) validateFiles(ctx context.Context, visible []Field, provided []SubmissionFile) ([]SubmissionFile, []FieldError) {
	var errs []FieldError
	byField := map[string]SubmissionFile{}
	for _, pf := range provided {
		byField[pf.FieldKey] = pf
	}
	var kept []SubmissionFile
	for _, f := range visible {
		if f.Type != FieldFile {
			continue
		}
		pf, ok := byField[f.Key]
		if !ok || strings.TrimSpace(pf.ObjectKey) == "" {
			if f.Required {
				errs = append(errs, FieldError{Field: f.Key, Message: "A file is required."})
			}
			continue
		}
		// Authoritative size/type check against the actual stored object
		if s.files != nil {
			size, mime, err := s.files.Head(ctx, pf.ObjectKey)
			if err != nil {
				errs = append(errs, FieldError{Field: f.Key, Message: "Upload could not be verified."})
				continue
			}
			if f.MaxFileBytes > 0 && size > f.MaxFileBytes {
				errs = append(errs, FieldError{Field: f.Key, Message: "File is too large."})
				continue
			}
			if len(f.AcceptMimes) > 0 && !mimeAllowed(f.AcceptMimes, mime) {
				errs = append(errs, FieldError{Field: f.Key, Message: "File type is not allowed."})
				continue
			}
			pf.Size = size
			pf.MimeType = mime
		}
		kept = append(kept, pf)
	}
	return kept, errs
}

// --- helpers ---

func normalizeSaveInput(in SaveInput) SaveInput {
	in.Name = strings.TrimSpace(in.Name)
	if in.Slug == "" {
		in.Slug = slugify(in.Name)
	} else {
		in.Slug = slugify(in.Slug)
	}
	if !in.Status.IsValid() {
		in.Status = StatusDraft
	}
	in.Settings.RedirectURL = strings.TrimSpace(in.Settings.RedirectURL)
	return in
}

// validateRedirectURL keeps a form's post-submit redirect from turning into an open redirect - the storefront hands it straight to window.location.href, so allow only a same-origin path (single leading slash, never a protocol-relative // or /\ ) or an absolute http(s) URL, and refuse javascript:, data: and bare hosts
func validateRedirectURL(raw string) error {
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "/") {
		if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
			return fmt.Errorf("%w: redirect URL must not be protocol-relative", ErrInvalidSchema)
		}
		return nil
	}
	if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: redirect URL must be a path or an http(s) URL", ErrInvalidSchema)
	}
	return nil
}

func optionAllowed(f Field, v string) bool {
	for _, o := range f.Options {
		if o.Value == v {
			return true
		}
	}
	return false
}

func mimeAllowed(accept []string, mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	for _, a := range accept {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == mime {
			return true
		}
		// support "image/*" wildcards
		if strings.HasSuffix(a, "/*") && strings.HasPrefix(mime, strings.TrimSuffix(a, "*")) {
			return true
		}
	}
	return false
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "on" || s == "yes"
	}
	return false
}

func toStringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	}
	return nil
}

func submitterEmail(form Form, answers map[string]any) string {
	// Prefer an explicit confirmation target, else the first email field
	if key := form.Settings.Confirmation.ToFieldKey; key != "" {
		if s, ok := answers[key].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	for _, f := range form.Fields {
		if f.Type == FieldEmail {
			if s, ok := answers[f.Key].(string); ok && s != "" {
				if _, err := mail.ParseAddress(s); err == nil {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func summarize(form Form, answers map[string]any) string {
	var b strings.Builder
	for _, f := range form.Fields {
		if f.Type.IsLayout() || f.Type == FieldHidden || f.Type == FieldSection {
			continue
		}
		v, ok := answers[f.Key]
		if !ok || isEmptyAnswer(v) {
			continue
		}
		label := f.Label
		if label == "" {
			label = f.Key
		}
		fmt.Fprintf(&b, "%s: %s\n", label, stringifyAnswer(v))
	}
	return b.String()
}

func stringifyAnswer(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "Yes"
		}
		return "No"
	case []string:
		return strings.Join(t, ", ")
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			if s := stringifyAnswer(e); strings.TrimSpace(s) != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		// matrix rows / repeater item: "key=value" pairs (sorted for stability)
		parts := make([]string, 0, len(t))
		for k, val := range t {
			parts = append(parts, k+"="+stringifyAnswer(val))
		}
		sort.Strings(parts)
		return strings.Join(parts, "; ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
