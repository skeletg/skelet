package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func pickLocalized[T any](all []T, localeOf func(T) string, locale, defLocale string) (T, bool) {
	for _, c := range all {
		if localeOf(c) == locale {
			return c, true
		}
	}
	if defLocale != "" && defLocale != locale {
		for _, c := range all {
			if localeOf(c) == defLocale {
				return c, true
			}
		}
	}
	if len(all) > 0 {
		return all[0], true
	}
	var zero T
	return zero, false
}

// Dropping the default-locale row while translations remain would leave them nothing to fall back to
func (s *core) refuseDefaultLocaleDelete(ctx context.Context, locale string, existing func() ([]string, error)) error {
	def := s.resolveDefaultLocale(ctx)
	if def == "" || locale != def {
		return nil
	}
	codes, err := existing()
	if err != nil {
		return err
	}
	for _, c := range codes {
		if c != def {
			return ErrCannotDeleteDefaultLocale
		}
	}
	return nil
}

func localesOf[T any](rows []T, localeOf func(T) string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = localeOf(r)
	}
	return out
}

// Product rows plus their content in locale (default-locale fallback), for structure reads that show member products - requireAll makes a missing row an error
func (s *core) loadProductsWithContent(ctx context.Context, ids []uuid.UUID, locale string, requireAll bool) (map[uuid.UUID]Product, map[uuid.UUID]ProductContent, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Product{}, map[uuid.UUID]ProductContent{}, nil
	}
	rows, err := s.repo.ListProductRowsByIDs(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("load products: %w", err)
	}
	if requireAll {
		for _, id := range ids {
			if _, ok := rows[id]; !ok {
				return nil, nil, fmt.Errorf("load product %s: %w", id, ErrProductNotFound)
			}
		}
	}
	content, err := s.LoadProductsContent(ctx, ids, locale)
	if err != nil {
		return nil, nil, fmt.Errorf("load product content: %w", err)
	}
	return rows, content, nil
}

func uniqueIDs(n int, at func(i int) uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, n)
	out := make([]uuid.UUID, 0, n)
	for i := range n {
		id := at(i)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// An unknown locale is refused up front rather than surfacing as a foreign-key violation - no-op without a validator wired
func (s *core) ensureLocale(ctx context.Context, code string) error {
	if s.locales == nil {
		return nil
	}
	return s.locales.EnsureExists(ctx, code)
}
