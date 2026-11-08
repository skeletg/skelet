package emailtemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Translation is a library email's version in one language - its subject and content, while the name, group and slot
// stay the email's own
type Translation struct {
	EmailID   uuid.UUID
	Locale    string
	Subject   string
	Preheader string
	Blocks    []Block
	Styles    EmailStyles
	UpdatedAt time.Time
}

var (
	ErrTranslationLocale = errors.New("a translation names the locale it is for, like de or de-at")
	ErrTranslationEmpty  = errors.New("a translation has content - delete it to send the email as written")
)

// A BCP 47 tag as the storefront sends it - a language, then optional region or script parts
var localeShape = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8}){0,3}$`)

// Locale codes are stored lowercase, as the storefront sends them
func normalizeLocale(code string) string { return strings.ToLower(strings.TrimSpace(code)) }

func (s *Service) ListTranslations(ctx context.Context, emailID uuid.UUID) ([]Translation, error) {
	return s.repo.ListTranslations(ctx, emailID)
}

func (s *Service) SaveTranslation(ctx context.Context, t Translation) (Translation, error) {
	t.Locale = normalizeLocale(t.Locale)
	if !localeShape.MatchString(t.Locale) {
		return Translation{}, ErrTranslationLocale
	}
	if len(t.Blocks) == 0 {
		return Translation{}, ErrTranslationEmpty
	}
	if err := s.ValidateContent(t.Blocks, t.Styles); err != nil {
		return Translation{}, err
	}
	return s.repo.UpsertTranslation(ctx, t)
}

func (s *Service) DeleteTranslation(ctx context.Context, emailID uuid.UUID, locale string) error {
	return s.repo.DeleteTranslation(ctx, emailID, normalizeLocale(locale))
}

// Localize is the email as a shopper in that locale gets it - its exact version, else the language's (de-at reads de),
// else the email as written, which the false says. A translation without a subject keeps the email's
func (s *Service) Localize(ctx context.Context, e LibraryEmail, locale string) (LibraryEmail, bool) {
	locale = normalizeLocale(locale)
	if locale == "" || e.ID == uuid.Nil {
		return e, false
	}
	base, _, _ := strings.Cut(locale, "-")
	found, err := s.repo.TranslationsIn(ctx, e.ID, []string{locale, base})
	if err != nil {
		log.Printf("[email] translations of %s: %v - sending the email as written", e.ID, err)
		return e, false
	}
	t, ok := pickTranslation(found, locale)
	if !ok {
		return e, false
	}
	if t.Subject != "" {
		e.Subject = t.Subject
	}
	e.Preheader, e.Blocks, e.Styles = t.Preheader, t.Blocks, t.Styles
	return e, true
}

func pickTranslation(all []Translation, locale string) (Translation, bool) {
	base, _, _ := strings.Cut(locale, "-")
	for _, want := range []string{locale, base} {
		for _, t := range all {
			if t.Locale == want {
				return t, true
			}
		}
	}
	return Translation{}, false
}

// --- repo impl ---

func (r *repo) ListTranslations(ctx context.Context, emailID uuid.UUID) ([]Translation, error) {
	rows, err := r.q.ListEmailTranslations(ctx, emailID)
	if err != nil {
		return nil, err
	}
	out := make([]Translation, 0, len(rows))
	for _, row := range rows {
		t, err := toTranslation(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *repo) TranslationsIn(ctx context.Context, emailID uuid.UUID, locales []string) ([]Translation, error) {
	rows, err := r.q.ListEmailTranslationsIn(ctx, db.ListEmailTranslationsInParams{EmailID: emailID, Locales: locales})
	if err != nil {
		return nil, err
	}
	out := make([]Translation, 0, len(rows))
	for _, row := range rows {
		t, err := toTranslation(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *repo) UpsertTranslation(ctx context.Context, t Translation) (Translation, error) {
	blocks, styles, err := marshalLibrary(LibraryEmail{Blocks: t.Blocks, Styles: t.Styles})
	if err != nil {
		return Translation{}, err
	}
	row, err := r.q.UpsertEmailTranslation(ctx, db.UpsertEmailTranslationParams{
		EmailID: t.EmailID, Locale: t.Locale, Subject: t.Subject, Preheader: t.Preheader, Blocks: blocks, Styles: styles,
	})
	if err != nil {
		return Translation{}, fmt.Errorf("save translation: %w", err)
	}
	return toTranslation(row)
}

func (r *repo) DeleteTranslation(ctx context.Context, emailID uuid.UUID, locale string) error {
	_, err := r.q.DeleteEmailTranslation(ctx, db.DeleteEmailTranslationParams{EmailID: emailID, Locale: locale})
	return err
}

func toTranslation(row db.EmailLibraryTranslation) (Translation, error) {
	t := Translation{EmailID: row.EmailID, Locale: row.Locale, Subject: row.Subject, Preheader: row.Preheader, UpdatedAt: row.UpdatedAt}
	if len(row.Blocks) > 0 {
		if err := json.Unmarshal(row.Blocks, &t.Blocks); err != nil {
			return Translation{}, err
		}
	}
	if len(row.Styles) > 0 {
		if err := json.Unmarshal(row.Styles, &t.Styles); err != nil {
			return Translation{}, err
		}
	}
	return t, nil
}
