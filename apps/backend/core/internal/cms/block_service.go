package cms

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
)

type blockStore interface {
	CreateBlockType(ctx context.Context, key, name, icon string, fields json.RawMessage, html string) (BlockType, error)
	CreateSnippet(ctx context.Context, slug, name string, blocks json.RawMessage) (Snippet, error)
	DeleteBlockType(ctx context.Context, id uuid.UUID) error
	DeleteSnippet(ctx context.Context, id uuid.UUID) error
	GetBlockTypeByID(ctx context.Context, id uuid.UUID) (BlockType, error)
	GetBlockTypeByKey(ctx context.Context, key string) (BlockType, error)
	GetSnippetByID(ctx context.Context, id uuid.UUID) (Snippet, error)
	GetSnippetBySlug(ctx context.Context, slug string) (Snippet, error)
	ListBlockTypes(ctx context.Context) ([]BlockType, error)
	ListSnippets(ctx context.Context) ([]Snippet, error)
	UpdateBlockType(ctx context.Context, id uuid.UUID, name, icon string, fields json.RawMessage, html string) (BlockType, error)
	UpdateSnippet(ctx context.Context, id uuid.UUID, name string, blocks json.RawMessage) (Snippet, error)
}

// CreateSnippet / GetSnippet / ListSnippets / UpdateSnippet / DeleteSnippet - a reusable section, a saved block subtree
// `blocks` is a { version, blocks } Document JSON, the same shape a page body carries
func (s *blockService) CreateSnippet(ctx context.Context, slug, name string, blocks json.RawMessage) (Snippet, error) {
	if err := validateSlug(slug); err != nil {
		return Snippet{}, err
	}
	return s.repo.CreateSnippet(ctx, slug, name, blocks)
}

func (s *blockService) GetSnippet(ctx context.Context, id uuid.UUID) (Snippet, error) {
	return s.repo.GetSnippetByID(ctx, id)
}

func (s *blockService) ListSnippets(ctx context.Context) ([]Snippet, error) {
	return s.repo.ListSnippets(ctx)
}

func (s *blockService) UpdateSnippet(ctx context.Context, id uuid.UUID, name string, blocks json.RawMessage) (Snippet, error) {
	return s.repo.UpdateSnippet(ctx, id, name, blocks)
}

func (s *blockService) DeleteSnippet(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSnippet(ctx, id)
}

// Resolves a snippet slug to its block subtree, for the front SnippetResolver expanding `ref` blocks server-side - ok=false when the slug is unknown or its body isn't a block document
func (s *blockService) GetSnippetBlocks(ctx context.Context, slug string) ([]blockkit.Block, bool) {
	sn, err := s.repo.GetSnippetBySlug(ctx, slug)
	if err != nil {
		return nil, false
	}
	doc, ok := blockkit.ParseDocument(sn.Blocks)
	if !ok {
		return nil, false
	}
	return doc.Blocks, true
}

// CreateBlockType / GetBlockType / ListBlockTypes / UpdateBlockType / DeleteBlockType - admin-defined custom block types, a field schema plus an HTML template
func (s *blockService) CreateBlockType(ctx context.Context, key, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	return s.repo.CreateBlockType(ctx, key, name, icon, fields, html)
}

func (s *blockService) GetBlockType(ctx context.Context, id uuid.UUID) (BlockType, error) {
	return s.repo.GetBlockTypeByID(ctx, id)
}

func (s *blockService) ListBlockTypes(ctx context.Context) ([]BlockType, error) {
	return s.repo.ListBlockTypes(ctx)
}

func (s *blockService) UpdateBlockType(ctx context.Context, id uuid.UUID, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	return s.repo.UpdateBlockType(ctx, id, name, icon, fields, html)
}

func (s *blockService) DeleteBlockType(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteBlockType(ctx, id)
}

// Builds a checker recognizing the admin's custom block-type keys, so a page using one validates on save - best-effort, since a load error yields a checker that rejects all custom types
// On *core so pageService and pageTypeService can both reach it
func (s *core) customTypeChecker(ctx context.Context) blockkit.CustomTypeChecker {
	types, err := s.repo.ListBlockTypes(ctx)
	known := make(map[string]bool, len(types))
	if err == nil {
		for _, t := range types {
			known[t.Key] = true
		}
	}
	return func(typeKey string) bool { return known[typeKey] }
}

// Looks up a custom block type by key, for the front CustomRenderer, which resolves a custom block lazily and only when present
func (s *blockService) GetBlockTypeByKey(ctx context.Context, key string) (BlockType, bool) {
	bt, err := s.repo.GetBlockTypeByKey(ctx, key)
	if err != nil {
		return BlockType{}, false
	}
	return bt, true
}
