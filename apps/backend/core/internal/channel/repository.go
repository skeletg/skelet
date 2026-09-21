package channel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

// NewRepository constructs a postgres-backed Repository - The txrunner.Manager is only used for SetLocales (delete + N
// inserts in one transaction); other operations talk to db.Queries directly
func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) Repository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateChannelInput) (Channel, error) {
	row, err := r.queries.CreateChannel(ctx, db.CreateChannelParams{
		Slug:          input.Slug,
		Name:          input.Name,
		DefaultLocale: input.DefaultLocale,
		CurrencyCode:  input.CurrencyCode,
		PublicUrl:     input.PublicURL,
		Metadata:      mapToJSON(input.Metadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Channel{}, ErrSlugTaken
		}
		return Channel{}, fmt.Errorf("create channel: %w", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Channel, error) {
	row, err := r.queries.GetChannelByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Channel{}, ErrChannelNotFound
		}
		return Channel{}, fmt.Errorf("get channel: %w", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) GetBySlug(ctx context.Context, slug string) (Channel, error) {
	row, err := r.queries.GetChannelBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Channel{}, ErrChannelNotFound
		}
		return Channel{}, fmt.Errorf("get channel by slug: %w", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) GetByStorefrontHost(ctx context.Context, host string) (Channel, error) {
	row, err := r.queries.GetChannelByStorefrontHost(ctx, host)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Channel{}, ErrChannelNotFound
		}
		return Channel{}, fmt.Errorf("get channel by host: %w", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) GetDefault(ctx context.Context) (Channel, error) {
	row, err := r.queries.GetDefaultChannel(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Channel{}, ErrChannelNotFound
		}
		return Channel{}, fmt.Errorf("get default channel: %w", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) List(ctx context.Context) ([]Channel, error) {
	rows, err := r.queries.ListChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]Channel, len(rows))
	for i, row := range rows {
		out[i] = toChannel(row)
	}
	return out, nil
}

func (r *postgresRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Channel, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Channel{}, nil
	}
	rows, err := r.queries.ListChannelsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list channels by ids: %w", err)
	}
	out := make(map[uuid.UUID]Channel, len(rows))
	for _, row := range rows {
		out[row.ID] = toChannel(row)
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateChannelInput) (Channel, error) {
	current, err := r.queries.GetChannelByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Channel{}, ErrChannelNotFound
		}
		return Channel{}, fmt.Errorf("load channel: %w", err)
	}
	merged := mergeUpdate(current, input)
	row, err := r.queries.UpdateChannel(ctx, merged)
	if err != nil {
		if isUniqueViolation(err) {
			return Channel{}, ErrSlugTaken
		}
		return Channel{}, fmt.Errorf("update channel: %w", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteChannel(ctx, id); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return ErrChannelInUse // orders, listings or carts still point at it - a RESTRICT FK, not a server fault
		}
		return fmt.Errorf("delete channel: %w", err)
	}
	return nil
}

// SetDefault atomically flips is_default=true on the given channel and clears it from any other row - Wrapped in a
// transaction so the partial unique index never sees two TRUE rows mid-flight
func (r *postgresRepository) SetDefault(ctx context.Context, id uuid.UUID) (Channel, error) {
	var out Channel
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.UnsetDefaultChannelExcept(ctx, id); err != nil {
			return fmt.Errorf("unset other defaults: %w", err)
		}
		row, err := q.SetChannelAsDefault(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChannelNotFound
			}
			return fmt.Errorf("set default: %w", err)
		}
		out = toChannel(row)
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	return out, nil
}

// ListLocales returns the locales attached to one store, ordered by the
// junction's sort_order
func (r *postgresRepository) ListLocales(ctx context.Context, channelID uuid.UUID) ([]locale.Locale, error) {
	rows, err := r.queries.ListChannelLocales(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel locales: %w", err)
	}
	out := make([]locale.Locale, len(rows))
	for i, row := range rows {
		out[i] = locale.Locale{
			Code:       row.Code,
			Name:       row.Name,
			NativeName: row.NativeName,
			IsActive:   row.IsActive,
			IsRTL:      row.IsRtl,
			SortOrder:  row.LocaleSortOrder,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		}
	}
	return out, nil
}

// ListLocalesByStoreIDs is the batched form for admin list views
func (r *postgresRepository) ListLocalesByStoreIDs(ctx context.Context, channelIDs []uuid.UUID) (map[uuid.UUID][]locale.Locale, error) {
	if len(channelIDs) == 0 {
		return map[uuid.UUID][]locale.Locale{}, nil
	}
	rows, err := r.queries.ListChannelLocalesByStoreIDs(ctx, channelIDs)
	if err != nil {
		return nil, fmt.Errorf("list channel locales batched: %w", err)
	}
	out := make(map[uuid.UUID][]locale.Locale, len(channelIDs))
	for _, row := range rows {
		out[row.ChannelID] = append(out[row.ChannelID], locale.Locale{
			Code:       row.Code,
			Name:       row.Name,
			NativeName: row.NativeName,
			IsActive:   row.IsActive,
			IsRTL:      row.IsRtl,
			SortOrder:  row.LocaleSortOrder,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		})
	}
	return out, nil
}

// SetLocales atomically replaces the supported-locales set for one store - Empty `codes` clears the list - Each insert lowercases the code and FK-validates against locales.code; an unknown code surfaces as a raw FK violation here - callers should pre-validate via locale.Service.EnsureExists for a friendlier error
func (r *postgresRepository) SetLocales(ctx context.Context, channelID uuid.UUID, codes []string) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.DeleteChannelLocales(ctx, channelID); err != nil {
			return fmt.Errorf("clear channel locales: %w", err)
		}
		for i, code := range codes {
			if err := q.InsertChannelLocale(ctx, db.InsertChannelLocaleParams{
				ChannelID: channelID,
				Lower:     code,
				SortOrder: int32(i),
			}); err != nil {
				return fmt.Errorf("insert channel locale %q: %w", code, err)
			}
		}
		return nil
	})
}

// mergeUpdate folds nil-pointer "no change" semantics from input over the
// currently-stored row, producing a fully-populated UpdateChannelParams
func mergeUpdate(current db.Channel, in UpdateChannelInput) db.UpdateChannelParams {
	out := db.UpdateChannelParams{
		ID:            in.ID,
		Name:          current.Name,
		DefaultLocale: current.DefaultLocale,
		IsActive:      current.IsActive,
		CurrencyCode:  current.CurrencyCode,
		PublicUrl:     current.PublicUrl,
		Metadata:      current.Metadata,
	}
	if in.Name != nil {
		out.Name = *in.Name
	}
	if in.DefaultLocale != nil {
		out.DefaultLocale = strings.ToLower(*in.DefaultLocale)
	}
	if in.IsActive != nil {
		out.IsActive = *in.IsActive
	}
	if in.CurrencyCode != nil {
		out.CurrencyCode = *in.CurrencyCode
	}
	if in.PublicURL != nil {
		out.PublicUrl = *in.PublicURL
	}
	if in.Metadata != nil {
		out.Metadata = mapToJSON(*in.Metadata)
	}
	return out
}

// toChannel maps a row to the domain type - Locales is left empty - callers that need it call repo.ListLocales(id) (or
// the batched ListLocalesByStoreIDs for list views)
func toChannel(row db.Channel) Channel {
	return Channel{
		ID:                  row.ID,
		Slug:                row.Slug,
		Name:                row.Name,
		DefaultLocale:       row.DefaultLocale,
		Locales:             nil,
		IsActive:            row.IsActive,
		IsDefault:           row.IsDefault,
		CurrencyCode:        row.CurrencyCode,
		PublicURL:           row.PublicUrl,
		Metadata:            jsonToMap(row.Metadata),
		TaxInclusivePricing: row.TaxInclusivePricing,
		CreatedAt:           row.CreatedAt,
	}
}

func mapToJSON(m Metadata) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return out
}

func jsonToMap(raw json.RawMessage) Metadata {
	if len(raw) == 0 {
		return Metadata{}
	}
	out := Metadata{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Metadata{}
	}
	return out
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
