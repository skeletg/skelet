package emailtemplate

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Service) ListBlocks(ctx context.Context, channelID uuid.UUID) ([]Snippet, error) {
	return s.repo.ListBlocks(ctx, channelID)
}

func (s *Service) GetBlock(ctx context.Context, channelID uuid.UUID, slug string) (Snippet, bool, error) {
	return s.repo.GetBlock(ctx, channelID, slug)
}

// Validates and upserts a reusable block, the unified component - it may declare a field schema and either a composed-block body or an admin-authored HTML one
// Snippets may nest refs, with a render-time visited-set and depth guard preventing cycles
func (s *Service) SaveBlock(ctx context.Context, sn Snippet) error {
	if strings.TrimSpace(sn.Slug) == "" {
		return fmt.Errorf("block slug is required")
	}
	if isBuiltinType(strings.TrimSpace(sn.Slug)) {
		return fmt.Errorf("slug %q collides with a built-in block type", sn.Slug)
	}
	if strings.TrimSpace(sn.Name) == "" {
		return fmt.Errorf("block name is required")
	}
	if err := validateFields(sn.Fields); err != nil {
		return err
	}
	if sn.Scripted && strings.TrimSpace(sn.HTMLTemplate) != "" {
		if err := checkScripted(sn); err != nil {
			return err
		}
	}
	// An HTML body is admin-trusted and sanitized at render, while a composed body validates each block
	if strings.TrimSpace(sn.HTMLTemplate) == "" {
		for i, b := range sn.Blocks {
			if err := validateBlock(b, true); err != nil {
				return fmt.Errorf("block %d: %w", i+1, err)
			}
		}
	}
	return s.repo.SaveBlock(ctx, sn)
}

func (s *Service) DeleteBlock(ctx context.Context, channelID uuid.UUID, slug string) error {
	return s.repo.DeleteBlock(ctx, channelID, slug)
}
