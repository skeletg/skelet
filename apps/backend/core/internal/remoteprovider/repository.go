package remoteprovider

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/sqlc-dev/pqtype"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateInput, signingSecret string) (Provider, error) {
	var ch uuid.NullUUID
	if input.ChannelID != nil {
		ch = uuid.NullUUID{UUID: *input.ChannelID, Valid: true}
	}
	row, err := r.queries.CreateRemoteProvider(ctx, db.CreateRemoteProviderParams{
		Kind:          string(input.Kind),
		Slug:          input.Slug,
		Name:          input.Name,
		TargetUrl:     input.TargetURL,
		SigningSecret: signingSecret,
		Config:        configToJSON(input.Config),
		TimeoutMs:     input.TimeoutMs,
		CacheTtlS:     input.CacheTTLs,
		FailurePolicy: string(input.FailurePolicy),
		ChannelID:     ch,
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Provider{}, ErrSlugTaken
		}
		return Provider{}, fmt.Errorf("create remote provider: %w", err)
	}
	return toProvider(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Provider, error) {
	row, err := r.queries.GetRemoteProviderByID(ctx, id)
	if err != nil {
		return Provider{}, ErrNotFound
	}
	return toProvider(row), nil
}

func (r *postgresRepository) GetBySlug(ctx context.Context, kind Kind, slug string, channelID *uuid.UUID) (Provider, error) {
	p := db.GetRemoteProviderBySlugParams{Kind: string(kind), Slug: slug}
	if channelID != nil {
		p.ChannelID = uuid.NullUUID{UUID: *channelID, Valid: true}
	}
	row, err := r.queries.GetRemoteProviderBySlug(ctx, p)
	if err != nil {
		return Provider{}, ErrNotFound
	}
	return toProvider(row), nil
}

func (r *postgresRepository) List(ctx context.Context) ([]Provider, error) {
	rows, err := r.queries.ListRemoteProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list remote providers: %w", err)
	}
	return toProviders(rows), nil
}

func (r *postgresRepository) ListActiveByKind(ctx context.Context, kind Kind, channelID *uuid.UUID) ([]Provider, error) {
	var ch uuid.NullUUID
	if channelID != nil {
		ch = uuid.NullUUID{UUID: *channelID, Valid: true}
	}
	rows, err := r.queries.ListActiveRemoteProvidersByKind(ctx, db.ListActiveRemoteProvidersByKindParams{
		Kind:      string(kind),
		ChannelID: ch,
	})
	if err != nil {
		return nil, fmt.Errorf("list active remote providers: %w", err)
	}
	return toProviders(rows), nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateInput) (Provider, error) {
	current, err := r.GetByID(ctx, input.ID)
	if err != nil {
		return Provider{}, err
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	targetURL := current.TargetURL
	if input.TargetURL != nil {
		targetURL = *input.TargetURL
	}
	config := current.Config
	if input.Config != nil {
		config = input.Config
	}
	timeoutMs := current.TimeoutMs
	if input.TimeoutMs != nil {
		timeoutMs = *input.TimeoutMs
	}
	cacheTTLs := current.CacheTTLs
	if input.CacheTTLs != nil {
		cacheTTLs = *input.CacheTTLs
	}
	policy := current.FailurePolicy
	if input.FailurePolicy != nil {
		policy = *input.FailurePolicy
	}
	row, err := r.queries.UpdateRemoteProvider(ctx, db.UpdateRemoteProviderParams{
		ID:            input.ID,
		Name:          name,
		TargetUrl:     targetURL,
		Config:        configToJSON(config),
		TimeoutMs:     timeoutMs,
		CacheTtlS:     cacheTTLs,
		FailurePolicy: string(policy),
	})
	if err != nil {
		return Provider{}, fmt.Errorf("update remote provider: %w", err)
	}
	return toProvider(row), nil
}

func (r *postgresRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) (Provider, error) {
	row, err := r.queries.SetRemoteProviderActive(ctx, db.SetRemoteProviderActiveParams{
		ID:       id,
		IsActive: active,
	})
	if err != nil {
		return Provider{}, fmt.Errorf("set remote provider active: %w", err)
	}
	return toProvider(row), nil
}

func (r *postgresRepository) UpdateSecret(ctx context.Context, id uuid.UUID, secret string) (Provider, error) {
	row, err := r.queries.UpdateRemoteProviderSecret(ctx, db.UpdateRemoteProviderSecretParams{
		ID:            id,
		SigningSecret: secret,
	})
	if err != nil {
		return Provider{}, fmt.Errorf("update remote provider secret: %w", err)
	}
	return toProvider(row), nil
}

func (r *postgresRepository) GetSecret(ctx context.Context, id uuid.UUID) (string, error) {
	secret, err := r.queries.GetRemoteProviderSecret(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get remote provider secret: %w", err)
	}
	return secret, nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteRemoteProvider(ctx, id); err != nil {
		return fmt.Errorf("delete remote provider: %w", err)
	}
	return nil
}

func configToJSON(config []byte) pqtype.NullRawMessage {
	if len(config) == 0 {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: config, Valid: true}
}

func toProvider(row db.RemoteProvider) Provider {
	p := Provider{
		ID:            row.ID,
		Kind:          Kind(row.Kind),
		Slug:          row.Slug,
		Name:          row.Name,
		TargetURL:     row.TargetUrl,
		TimeoutMs:     row.TimeoutMs,
		CacheTTLs:     row.CacheTtlS,
		FailurePolicy: FailurePolicy(row.FailurePolicy),
		IsActive:      row.IsActive,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.Config.Valid {
		p.Config = row.Config.RawMessage
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		p.ChannelID = &id
	}
	return p
}

func toProviders(rows []db.RemoteProvider) []Provider {
	out := make([]Provider, len(rows))
	for i, row := range rows {
		out[i] = toProvider(row)
	}
	return out
}
