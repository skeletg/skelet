package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/blockkit"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
)

func (r *mutationResolver) GenerateCMSPageSeo(ctx context.Context, pageID string, locale string) (*model.AISeoSuggestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	if r.AISvc == nil {
		return nil, errNotConfigured("ai")
	}
	pid, err := uuid.Parse(pageID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	page, err := r.CMSSvc.GetPage(ctx, pid, locale)
	if err != nil {
		return nil, err
	}
	if page.Content == nil {
		return nil, fmt.Errorf("page has no content for locale %q", locale)
	}
	var bodyText string
	if doc, ok := blockkit.ParseDocument(page.Content.Body); ok {
		bodyText = blockkit.PlainText(doc)
	}
	sug, err := r.AISvc.GenerateSEO(ctx, page.Content.Title, bodyText)
	if err != nil {
		return nil, err
	}
	return &model.AISeoSuggestion{MetaTitle: sug.MetaTitle, MetaDescription: sug.MetaDescription}, nil
}

func (r *mutationResolver) GenerateMediaAltText(ctx context.Context, assetID string) (string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "media.manage"); err != nil {
		return "", err
	}
	if r.AISvc == nil {
		return "", errNotConfigured("ai")
	}
	aid, err := uuid.Parse(assetID)
	if err != nil {
		return "", errInvalidID("id", err)
	}
	asset, err := r.MediaLibrarySvc.GetAsset(ctx, aid)
	if err != nil {
		return "", err
	}
	image, err := fetchImageBytes(ctx, asset.URL)
	if err != nil {
		return "", err
	}
	return r.AISvc.GenerateAltText(ctx, asset.MimeType, image)
}

func (r *mutationResolver) TranslateCMSPage(ctx context.Context, pageID string, sourceLocale string, targetLocale string) (*model.CMSPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.manage"); err != nil {
		return nil, err
	}
	if r.AISvc == nil {
		return nil, errNotConfigured("ai")
	}
	if strings.TrimSpace(sourceLocale) == "" || strings.TrimSpace(targetLocale) == "" || sourceLocale == targetLocale {
		return nil, fmt.Errorf("source and target locales must differ and be non-empty")
	}
	pid, err := uuid.Parse(pageID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	page, err := r.CMSSvc.GetPage(ctx, pid, sourceLocale)
	if err != nil {
		return nil, err
	}
	// Use the EXACT source-locale content (not GetPage's fallback Content).
	var src *cms.PageContent
	for i := range page.AllContent {
		if page.AllContent[i].Locale == sourceLocale {
			src = &page.AllContent[i]
			break
		}
	}
	if src == nil {
		return nil, fmt.Errorf("page has no content for source locale %q", sourceLocale)
	}

	// Header fields (title + 3 meta) then the body's translatable strings, in a
	// stable order ApplyTranslations maps straight back.
	doc, isDoc := blockkit.ParseDocument(src.Body)
	var bodyStrings []string
	if isDoc {
		bodyStrings = blockkit.TranslatableStrings(doc)
	}
	all := append([]string{src.Title, ptr.Deref(src.MetaTitle), ptr.Deref(src.MetaDescription), ptr.Deref(src.MetaKeywords)}, bodyStrings...)

	// Translate only the non-empty entries (empty meta fields stay empty).
	idx := make([]int, 0, len(all))
	nonEmpty := make([]string, 0, len(all))
	for i, s := range all {
		if strings.TrimSpace(s) != "" {
			idx = append(idx, i)
			nonEmpty = append(nonEmpty, s)
		}
	}
	translatedNonEmpty, err := r.AISvc.Translate(ctx, sourceLocale, targetLocale, nonEmpty)
	if err != nil {
		return nil, err
	}
	translated := make([]string, len(all))
	copy(translated, all)
	for j, i := range idx {
		translated[i] = translatedNonEmpty[j]
	}

	newBody := src.Body
	if isDoc {
		if raw, mErr := json.Marshal(blockkit.ApplyTranslations(doc, translated[4:])); mErr == nil {
			newBody = raw
		}
	}
	updated, err := r.CMSSvc.UpsertPageContent(ctx, pid, cms.PageContentInput{
		Locale:          targetLocale,
		Title:           translated[0],
		Body:            newBody,
		MetaTitle:       translated[1],
		MetaDescription: translated[2],
		MetaKeywords:    translated[3],
	})
	if err != nil {
		return nil, err
	}
	return toCMSPageModel(updated), nil
}

func (r *queryResolver) AiStatus(ctx context.Context) (*model.AIStatus, error) {
	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return &model.AIStatus{Enabled: r.AISvc != nil && r.AISvc.Available(ctx)}, nil
}
