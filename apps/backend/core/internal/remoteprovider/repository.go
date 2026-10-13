package remoteprovider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

type postgresRepository struct {
	queries *db.Queries
	sealer  Sealer
}

// Seals what sits in the database - satisfied by *secrets.Cipher, whose Decrypt passes unsealed plaintext through so rows written before a key was set still read
type Sealer interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(stored string) (string, error)
}

var ErrSealedWithoutKey = errors.New("remote provider: value is encrypted but no encryption key is configured")

// A nil sealer stores plaintext - dev without CONFIG_ENCRYPTION_KEY
func NewRepository(queries *db.Queries, sealer Sealer) Repository {
	return &postgresRepository{queries: queries, sealer: sealer}
}

func (r *postgresRepository) seal(plain string) (string, error) {
	if r.sealer == nil || plain == "" {
		return plain, nil
	}
	return r.sealer.Encrypt(plain)
}

func (r *postgresRepository) open(stored string) (string, error) {
	if !secrets.IsEncrypted(stored) {
		return stored, nil
	}
	if r.sealer == nil {
		return "", ErrSealedWithoutKey
	}
	return r.sealer.Decrypt(stored)
}

func (r *postgresRepository) sealConfig(cfg []byte) (sql.NullString, error) {
	if len(cfg) == 0 {
		return sql.NullString{}, nil
	}
	sealed, err := r.seal(string(cfg))
	if err != nil {
		return sql.NullString{}, fmt.Errorf("seal config: %w", err)
	}
	return sql.NullString{String: sealed, Valid: true}, nil
}

func (r *postgresRepository) Create(ctx context.Context, input CreateInput, signingSecret string) (Provider, error) {
	var ch uuid.NullUUID
	if input.ChannelID != nil {
		ch = uuid.NullUUID{UUID: *input.ChannelID, Valid: true}
	}
	sealedSecret, err := r.seal(signingSecret)
	if err != nil {
		return Provider{}, fmt.Errorf("seal signing secret: %w", err)
	}
	cfg, err := r.sealConfig(input.Config)
	if err != nil {
		return Provider{}, err
	}
	row, err := r.queries.CreateRemoteProvider(ctx, db.CreateRemoteProviderParams{
		Kind:          string(input.Kind),
		Slug:          input.Slug,
		Name:          input.Name,
		TargetUrl:     input.TargetURL,
		SigningSecret: sealedSecret,
		Config:        cfg,
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
	return r.toProvider(row)
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Provider, error) {
	row, err := r.queries.GetRemoteProviderByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, ErrNotFound
	}
	if err != nil {
		return Provider{}, fmt.Errorf("get remote provider: %w", err)
	}
	return r.toProvider(row)
}

func (r *postgresRepository) GetBySlug(ctx context.Context, kind Kind, slug string, channelID *uuid.UUID) (Provider, error) {
	p := db.GetRemoteProviderBySlugParams{Kind: string(kind), Slug: slug}
	if channelID != nil {
		p.ChannelID = uuid.NullUUID{UUID: *channelID, Valid: true}
	}
	row, err := r.queries.GetRemoteProviderBySlug(ctx, p)
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, ErrNotFound
	}
	if err != nil {
		return Provider{}, fmt.Errorf("get remote provider by slug: %w", err)
	}
	return r.toProvider(row)
}

func (r *postgresRepository) List(ctx context.Context) ([]Provider, error) {
	rows, err := r.queries.ListRemoteProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list remote providers: %w", err)
	}
	return r.toProviders(rows)
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
	return r.toProviders(rows)
}

func (r *postgresRepository) ListActiveForChannel(ctx context.Context, channelID *uuid.UUID) ([]Provider, error) {
	var ch uuid.NullUUID
	if channelID != nil {
		ch = uuid.NullUUID{UUID: *channelID, Valid: true}
	}
	rows, err := r.queries.ListActiveRemoteProvidersForChannel(ctx, ch)
	if err != nil {
		return nil, fmt.Errorf("list active remote providers for channel: %w", err)
	}
	return r.toProviders(rows)
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
	cfg, err := r.sealConfig(config)
	if err != nil {
		return Provider{}, err
	}
	row, err := r.queries.UpdateRemoteProvider(ctx, db.UpdateRemoteProviderParams{
		ID:            input.ID,
		Name:          name,
		TargetUrl:     targetURL,
		Config:        cfg,
		TimeoutMs:     timeoutMs,
		CacheTtlS:     cacheTTLs,
		FailurePolicy: string(policy),
	})
	if err != nil {
		return Provider{}, fmt.Errorf("update remote provider: %w", err)
	}
	return r.toProvider(row)
}

func (r *postgresRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) (Provider, error) {
	row, err := r.queries.SetRemoteProviderActive(ctx, db.SetRemoteProviderActiveParams{
		ID:       id,
		IsActive: active,
	})
	if err != nil {
		return Provider{}, fmt.Errorf("set remote provider active: %w", err)
	}
	return r.toProvider(row)
}

func (r *postgresRepository) UpdateSecret(ctx context.Context, id uuid.UUID, secret string) (Provider, error) {
	sealed, err := r.seal(secret)
	if err != nil {
		return Provider{}, fmt.Errorf("seal signing secret: %w", err)
	}
	row, err := r.queries.UpdateRemoteProviderSecret(ctx, db.UpdateRemoteProviderSecretParams{
		ID:            id,
		SigningSecret: sealed,
	})
	if err != nil {
		return Provider{}, fmt.Errorf("update remote provider secret: %w", err)
	}
	return r.toProvider(row)
}

func (r *postgresRepository) GetSecret(ctx context.Context, id uuid.UUID) (string, error) {
	stored, err := r.queries.GetRemoteProviderSecret(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get remote provider secret: %w", err)
	}
	return r.open(stored)
}

// Seals every row still holding plaintext - rows from before a key was set, which otherwise stay plaintext until their next edit or rotation
func (r *postgresRepository) SealPlaintext(ctx context.Context) (int, error) {
	if r.sealer == nil {
		return 0, nil
	}
	rows, err := r.queries.ListRemoteProviderSecrets(ctx)
	if err != nil {
		return 0, fmt.Errorf("list remote provider secrets: %w", err)
	}
	sealed := 0
	for _, row := range rows {
		secretOpen := !secrets.IsEncrypted(row.SigningSecret)
		configOpen := row.Config.Valid && !secrets.IsEncrypted(row.Config.String)
		if !secretOpen && !configOpen {
			continue
		}
		secret, cfg := row.SigningSecret, row.Config
		if secretOpen {
			if secret, err = r.seal(secret); err != nil {
				return sealed, fmt.Errorf("seal signing secret: %w", err)
			}
		}
		if configOpen {
			if cfg, err = r.sealConfig([]byte(cfg.String)); err != nil {
				return sealed, err
			}
		}
		if err := r.queries.SetRemoteProviderSealed(ctx, db.SetRemoteProviderSealedParams{ID: row.ID, SigningSecret: secret, Config: cfg}); err != nil {
			return sealed, fmt.Errorf("store sealed provider %s: %w", row.ID, err)
		}
		sealed++
	}
	return sealed, nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteRemoteProvider(ctx, id); err != nil {
		return fmt.Errorf("delete remote provider: %w", err)
	}
	return nil
}

func (r *postgresRepository) toProvider(row db.RemoteProvider) (Provider, error) {
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
		cfg, err := r.open(row.Config.String)
		if err != nil {
			return Provider{}, fmt.Errorf("open config of %s: %w", row.Slug, err)
		}
		p.Config = []byte(cfg)
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		p.ChannelID = &id
	}
	return p, nil
}

func (r *postgresRepository) toProviders(rows []db.RemoteProvider) ([]Provider, error) {
	out := make([]Provider, len(rows))
	for i, row := range rows {
		p, err := r.toProvider(row)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}
