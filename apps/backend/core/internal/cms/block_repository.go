package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func toSnippet(r db.CmsSnippet) Snippet {
	return Snippet{ID: r.ID, Slug: r.Slug, Name: r.Name, Blocks: r.Blocks, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *postgresRepository) CreateSnippet(ctx context.Context, slug, name string, blocks json.RawMessage) (Snippet, error) {
	row, err := r.queries.CreateCMSSnippet(ctx, db.CreateCMSSnippetParams{Slug: slug, Name: name, Blocks: blocks})
	if err != nil {
		if isUniqueViolation(err) {
			return Snippet{}, ErrSlugTaken
		}
		return Snippet{}, fmt.Errorf("create cms snippet: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) GetSnippetByID(ctx context.Context, id uuid.UUID) (Snippet, error) {
	row, err := r.queries.GetCMSSnippetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrSnippetNotFound
		}
		return Snippet{}, fmt.Errorf("get cms snippet: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) GetSnippetBySlug(ctx context.Context, slug string) (Snippet, error) {
	row, err := r.queries.GetCMSSnippetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrSnippetNotFound
		}
		return Snippet{}, fmt.Errorf("get cms snippet by slug: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) ListSnippets(ctx context.Context) ([]Snippet, error) {
	rows, err := r.queries.ListCMSSnippets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cms snippets: %w", err)
	}
	out := make([]Snippet, len(rows))
	for i, row := range rows {
		out[i] = toSnippet(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateSnippet(ctx context.Context, id uuid.UUID, name string, blocks json.RawMessage) (Snippet, error) {
	row, err := r.queries.UpdateCMSSnippet(ctx, db.UpdateCMSSnippetParams{
		ID:     id,
		Name:   sql.NullString{String: name, Valid: true},
		Blocks: pqtype.NullRawMessage{RawMessage: blocks, Valid: blocks != nil},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrSnippetNotFound
		}
		return Snippet{}, fmt.Errorf("update cms snippet: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) DeleteSnippet(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSSnippet(ctx, id); err != nil {
		return fmt.Errorf("delete cms snippet: %w", err)
	}
	return nil
}

func toBlockType(r db.CmsBlockType) BlockType {
	return BlockType{ID: r.ID, Key: r.Key, Name: r.Name, Icon: r.Icon, Fields: r.Fields, HTMLTemplate: r.HtmlTemplate, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *postgresRepository) CreateBlockType(ctx context.Context, key, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	row, err := r.queries.CreateCMSBlockType(ctx, db.CreateCMSBlockTypeParams{Key: key, Name: name, Icon: icon, Fields: fields, HtmlTemplate: html})
	if err != nil {
		if isUniqueViolation(err) {
			return BlockType{}, ErrSlugTaken // the block type's key is its slug
		}
		return BlockType{}, fmt.Errorf("create cms block type: %w", err)
	}
	return toBlockType(row), nil
}

func (r *postgresRepository) GetBlockTypeByID(ctx context.Context, id uuid.UUID) (BlockType, error) {
	row, err := r.queries.GetCMSBlockTypeByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockType{}, ErrBlockTypeNotFound
		}
		return BlockType{}, fmt.Errorf("get cms block type: %w", err)
	}
	return toBlockType(row), nil
}

func (r *postgresRepository) ListBlockTypes(ctx context.Context) ([]BlockType, error) {
	rows, err := r.queries.ListCMSBlockTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cms block types: %w", err)
	}
	out := make([]BlockType, len(rows))
	for i, row := range rows {
		out[i] = toBlockType(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateBlockType(ctx context.Context, id uuid.UUID, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	row, err := r.queries.UpdateCMSBlockType(ctx, db.UpdateCMSBlockTypeParams{
		ID:           id,
		Name:         sql.NullString{String: name, Valid: true},
		Icon:         sql.NullString{String: icon, Valid: true},
		Fields:       pqtype.NullRawMessage{RawMessage: fields, Valid: fields != nil},
		HtmlTemplate: sql.NullString{String: html, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockType{}, ErrBlockTypeNotFound
		}
		return BlockType{}, fmt.Errorf("update cms block type: %w", err)
	}
	return toBlockType(row), nil
}

func (r *postgresRepository) DeleteBlockType(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSBlockType(ctx, id); err != nil {
		return fmt.Errorf("delete cms block type: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetBlockTypeByKey(ctx context.Context, key string) (BlockType, error) {
	row, err := r.queries.GetCMSBlockTypeByKey(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockType{}, ErrBlockTypeNotFound
		}
		return BlockType{}, fmt.Errorf("get cms block type by key: %w", err)
	}
	return toBlockType(row), nil
}
