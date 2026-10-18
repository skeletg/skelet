package channel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

const storefrontHostIndex = "idx_channels_storefront_host"

// tx is nil on a repository already bound to a transaction
type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) Repository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) Atomic(ctx context.Context, fn func(Repository) error) error {
	if r.tx == nil {
		return fn(r)
	}
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		return fn(&postgresRepository{queries: r.queries.WithTx(t)})
	})
}

func (r *postgresRepository) inTx(ctx context.Context, fn func(*db.Queries) error) error {
	return r.Atomic(ctx, func(repo Repository) error { return fn(repo.(*postgresRepository).queries) })
}

func (r *postgresRepository) Create(ctx context.Context, input CreateChannelInput) (Channel, error) {
	row, err := r.queries.CreateChannel(ctx, db.CreateChannelParams{
		Slug:           input.Slug,
		Name:           input.Name,
		DefaultLocale:  input.DefaultLocale,
		CurrencyCode:   input.CurrencyCode,
		PublicUrl:      input.PublicURL,
		StorefrontHost: storefrontHost(input.PublicURL),
		Metadata:       mapToJSON(input.Metadata),
	})
	if err != nil {
		return Channel{}, writeError("create channel", err)
	}
	return toChannel(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Channel, error) {
	return readOne(r.queries.GetChannelByID(ctx, id))
}

func (r *postgresRepository) GetForUpdate(ctx context.Context, id uuid.UUID) (Channel, error) {
	return readOne(r.queries.GetChannelByIDForUpdate(ctx, id))
}

func (r *postgresRepository) GetBySlug(ctx context.Context, slug string) (Channel, error) {
	return readOne(r.queries.GetChannelBySlug(ctx, slug))
}

func (r *postgresRepository) GetByStorefrontHost(ctx context.Context, host string) (Channel, error) {
	return readOne(r.queries.GetChannelByStorefrontHost(ctx, host))
}

func (r *postgresRepository) GetDefault(ctx context.Context) (Channel, error) {
	return readOne(r.queries.GetDefaultChannel(ctx))
}

func readOne(row db.Channel, err error) (Channel, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrChannelNotFound
	}
	if err != nil {
		return Channel{}, fmt.Errorf("get channel: %w", err)
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

func (r *postgresRepository) Update(ctx context.Context, ch Channel, writeMetadata bool) (Channel, error) {
	params := db.UpdateChannelParams{
		ID:             ch.ID,
		Name:           ch.Name,
		DefaultLocale:  ch.DefaultLocale,
		IsActive:       ch.IsActive,
		CurrencyCode:   ch.CurrencyCode,
		PublicUrl:      ch.PublicURL,
		StorefrontHost: storefrontHost(ch.PublicURL),
	}
	if writeMetadata {
		params.Metadata = pqtype.NullRawMessage{RawMessage: mapToJSON(ch.Metadata), Valid: true}
	}
	row, err := r.queries.UpdateChannel(ctx, params)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrChannelNotFound
	}
	if err != nil {
		return Channel{}, writeError("update channel", err)
	}
	return toChannel(row), nil
}

// The two unique indexes a channel write can trip, told apart by name
func writeError(op string, err error) error {
	if pgdb.IsUniqueViolation(err) {
		if pgdb.ConstraintName(err) == storefrontHostIndex {
			return ErrStorefrontHostTaken
		}
		return ErrSlugTaken
	}
	return fmt.Errorf("%s: %w", op, err)
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteChannel(ctx, id); err != nil {
		if pgdb.IsForeignKeyViolation(err) {
			return ErrChannelInUse // orders, listings, carts or gift cards still point at it - a RESTRICT FK, not a server fault
		}
		return fmt.Errorf("delete channel: %w", err)
	}
	return nil
}

func (r *postgresRepository) SetDefault(ctx context.Context, id uuid.UUID) (Channel, []uuid.UUID, error) {
	var (
		out     Channel
		cleared []uuid.UUID
	)
	err := r.inTx(ctx, func(q *db.Queries) error {
		var err error
		if cleared, err = q.UnsetDefaultChannelExcept(ctx, id); err != nil {
			return fmt.Errorf("unset other defaults: %w", err)
		}
		out, err = readOne(q.SetChannelAsDefault(ctx, id))
		return err
	})
	if err != nil {
		return Channel{}, nil, err
	}
	return out, cleared, nil
}

func (r *postgresRepository) CountInheritingListings(ctx context.Context, channelID uuid.UUID) (int64, error) {
	n, err := r.queries.CountInheritingListingsForChannel(ctx, channelID)
	if err != nil {
		return 0, fmt.Errorf("count inheriting listings: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListLocales(ctx context.Context, channelID uuid.UUID) ([]locale.Locale, error) {
	byID, err := r.ListLocalesByStoreIDs(ctx, []uuid.UUID{channelID})
	if err != nil {
		return nil, err
	}
	return byID[channelID], nil
}

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

// Replaces the whole set - each code is FK-checked on insert, so callers pre-validate for a friendlier error
func (r *postgresRepository) SetLocales(ctx context.Context, channelID uuid.UUID, codes []string) error {
	return r.inTx(ctx, func(q *db.Queries) error {
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

// Locales is left empty - callers hydrate it through ListLocales or the batched ListLocalesByStoreIDs
func toChannel(row db.Channel) Channel {
	return Channel{
		ID:                  row.ID,
		Slug:                row.Slug,
		Name:                row.Name,
		DefaultLocale:       row.DefaultLocale,
		IsActive:            row.IsActive,
		IsDefault:           row.IsDefault,
		CurrencyCode:        row.CurrencyCode,
		PublicURL:           row.PublicUrl,
		Metadata:            jsonToMap(row.ID, row.Metadata),
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

func jsonToMap(id uuid.UUID, raw json.RawMessage) Metadata {
	out := Metadata{}
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		slog.Warn("channel: unreadable metadata, serving it empty", "channel_id", id, "err", err)
		return Metadata{}
	}
	return out
}
