package adminprefs

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

const (
	maxKeyLen     = 128
	maxValueBytes = 32 << 10 // preferences are UI state, not documents - 32KB catches a runaway payload
)

// Lowercase namespaced keys like "table:catalog.products", "views:/orders", "nav:pins"
var keyPattern = regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9:./_-]*$`)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, adminID uuid.UUID) ([]Preference, error) {
	return s.repo.List(ctx, adminID)
}

// Strings reads a preference the dashboard stores as a JSON array of strings, for the server-side consumers of a client-owned blob (the inbox's muted categories) A missing key and a value the client wrote in some other shape both read as none: these are opaque UI blobs, and one hand-edited entry must not take a feature down
func (s *Service) Strings(ctx context.Context, adminID uuid.UUID, key string) ([]string, error) {
	raw, err := s.repo.Get(ctx, adminID, key)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil
	}
	return out, nil
}

// Upserts one preference - a nil value deletes the key, so the client has one call for both
func (s *Service) Set(ctx context.Context, adminID uuid.UUID, key string, value any) error {
	if len(key) == 0 || len(key) > maxKeyLen || !keyPattern.MatchString(key) {
		return fmt.Errorf("invalid preference key")
	}
	if value == nil {
		return s.repo.Delete(ctx, adminID, key)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode preference: %w", err)
	}
	if len(raw) > maxValueBytes {
		return fmt.Errorf("preference value too large")
	}
	return s.repo.Set(ctx, adminID, key, raw)
}
