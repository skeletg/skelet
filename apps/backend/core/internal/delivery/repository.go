package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

type Repository interface {
	ListOptions(ctx context.Context, channelID uuid.UUID) ([]Option, error)
	GetOption(ctx context.Context, id uuid.UUID) (Option, error)
	CreateOption(ctx context.Context, channelID uuid.UUID, in SaveOptionInput) (Option, error)
	UpdateOption(ctx context.Context, id uuid.UUID, in SaveOptionInput) (Option, error)
	DeleteOption(ctx context.Context, id uuid.UUID) error
	LineProfiles(ctx context.Context, skus []string) (map[string]LineProfile, error)
	SetProductTypeKinds(ctx context.Context, productTypeID uuid.UUID, kinds []string) error
	ListPoints(ctx context.Context, channelID uuid.UUID) ([]ManualPoint, error)
	SearchPoints(ctx context.Context, channelID uuid.UUID, kind string, near Near, search string, limit int) ([]ManualPoint, error)
	GetPoint(ctx context.Context, channelID, id uuid.UUID) (ManualPoint, error)
	CountActivePoints(ctx context.Context, channelID uuid.UUID, kind string) (int64, error)
	ReplacePoints(ctx context.Context, channelID uuid.UUID, points []ManualPoint) ([]ManualPoint, error)
}

type postgresRepository struct {
	q  *db.Queries
	tx *txrunner.Manager
}

func NewRepository(q *db.Queries, managers ...*txrunner.Manager) Repository {
	r := &postgresRepository{q: q}
	if len(managers) > 0 {
		r.tx = managers[0]
	}
	return r
}

func (r *postgresRepository) ListOptions(ctx context.Context, channelID uuid.UUID) ([]Option, error) {
	rows, err := r.q.ListDeliveryOptionsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]Option, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOption(row))
	}
	return out, nil
}

func (r *postgresRepository) GetOption(ctx context.Context, id uuid.UUID) (Option, error) {
	row, err := r.q.GetDeliveryOption(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Option{}, ErrOptionNotFound
	}
	if err != nil {
		return Option{}, err
	}
	return toOption(row), nil
}

func (r *postgresRepository) CreateOption(ctx context.Context, channelID uuid.UUID, in SaveOptionInput) (Option, error) {
	modes, err := json.Marshal(nonNilModes(in.BlockModes))
	if err != nil {
		return Option{}, err
	}
	row, err := r.q.CreateDeliveryOption(ctx, db.CreateDeliveryOptionParams{
		ChannelID: channelID, Kind: in.Kind, Name: in.Name, Description: in.Description, Enabled: in.Enabled,
		SortOrder: in.SortOrder, BlockModes: modes, PointSource: in.PointSource, CarrierSlug: in.CarrierSlug,
	})
	if err != nil {
		return Option{}, err
	}
	return toOption(row), nil
}

func (r *postgresRepository) UpdateOption(ctx context.Context, id uuid.UUID, in SaveOptionInput) (Option, error) {
	modes, err := json.Marshal(nonNilModes(in.BlockModes))
	if err != nil {
		return Option{}, err
	}
	row, err := r.q.UpdateDeliveryOption(ctx, db.UpdateDeliveryOptionParams{
		ID: id, Name: in.Name, Description: in.Description, Enabled: in.Enabled, SortOrder: in.SortOrder,
		BlockModes: modes, PointSource: in.PointSource, CarrierSlug: in.CarrierSlug,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Option{}, ErrOptionNotFound
	}
	if err != nil {
		return Option{}, err
	}
	return toOption(row), nil
}

func (r *postgresRepository) DeleteOption(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteDeliveryOption(ctx, id)
}

func (r *postgresRepository) LineProfiles(ctx context.Context, skus []string) (map[string]LineProfile, error) {
	out := make(map[string]LineProfile, len(skus))
	if len(skus) == 0 {
		return out, nil
	}
	rows, err := r.q.GetDeliveryProfilesBySKUs(ctx, skus)
	if err != nil {
		return nil, fmt.Errorf("delivery profiles: %w", err)
	}
	for _, row := range rows {
		out[row.Sku] = LineProfile{
			SKU: row.Sku, ProductID: row.ProductID, ProductTypeID: row.ProductTypeID, RequiresShipping: row.IsShippingRequired,
			DeliveryKinds: row.DeliveryKinds, TypeName: row.ProductTypeName,
		}
	}
	return out, nil
}

func (r *postgresRepository) SetProductTypeKinds(ctx context.Context, productTypeID uuid.UUID, kinds []string) error {
	return r.q.SetProductTypeDeliveryKinds(ctx, db.SetProductTypeDeliveryKindsParams{ID: productTypeID, DeliveryKinds: kinds})
}

func (r *postgresRepository) ListPoints(ctx context.Context, channelID uuid.UUID) ([]ManualPoint, error) {
	rows, err := r.q.ListDeliveryPointsByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return toManualPoints(rows), nil
}

func (r *postgresRepository) SearchPoints(ctx context.Context, channelID uuid.UUID, kind string, near Near, search string, limit int) ([]ManualPoint, error) {
	rows, err := r.q.SearchDeliveryPoints(ctx, db.SearchDeliveryPointsParams{
		ChannelID: channelID, Kind: kind, CountryCode: near.CountryCode, Search: search, PostalCode: near.PostalCode, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return toManualPoints(rows), nil
}

func (r *postgresRepository) GetPoint(ctx context.Context, channelID, id uuid.UUID) (ManualPoint, error) {
	row, err := r.q.GetDeliveryPoint(ctx, db.GetDeliveryPointParams{ChannelID: channelID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return ManualPoint{}, ErrPointNotFound
	}
	if err != nil {
		return ManualPoint{}, err
	}
	return toManualPoint(row), nil
}

func (r *postgresRepository) CountActivePoints(ctx context.Context, channelID uuid.UUID, kind string) (int64, error) {
	return r.q.CountActiveDeliveryPoints(ctx, db.CountActiveDeliveryPointsParams{ChannelID: channelID, Kind: kind})
}

// ReplacePoints saves the channel's whole list in one transaction - rows it names are upserted, the rest deleted
func (r *postgresRepository) ReplacePoints(ctx context.Context, channelID uuid.UUID, points []ManualPoint) ([]ManualPoint, error) {
	save := func(q *db.Queries) error {
		keep := make([]uuid.UUID, 0, len(points))
		for _, p := range points {
			keep = append(keep, p.ID)
		}
		if err := q.DeleteDeliveryPointsNotIn(ctx, db.DeleteDeliveryPointsNotInParams{ChannelID: channelID, Keep: keep}); err != nil {
			return err
		}
		for _, p := range points {
			if _, err := q.UpsertDeliveryPoint(ctx, db.UpsertDeliveryPointParams{
				ID: p.ID, ChannelID: channelID, Kind: p.Kind, Name: p.Name, Street1: p.Address.Street1, Street2: p.Address.Street2,
				City: p.Address.City, Region: p.Address.Region, PostalCode: p.Address.PostalCode, CountryCode: p.Address.CountryCode,
				Instructions: p.Instructions, Hours: p.Hours, IsActive: p.Active, SortOrder: p.SortOrder,
			}); err != nil {
				// An id that belongs to another channel matches no row to update and returns none
				if errors.Is(err, sql.ErrNoRows) {
					return ErrPointNotFound
				}
				return err
			}
		}
		return nil
	}
	var err error
	if r.tx != nil {
		err = r.tx.InTransaction(ctx, func(t *sql.Tx) error { return save(r.q.WithTx(t)) })
	} else {
		err = save(r.q)
	}
	if err != nil {
		return nil, err
	}
	return r.ListPoints(ctx, channelID)
}

func toOption(row db.DeliveryOption) Option {
	o := Option{
		ID: row.ID, ChannelID: row.ChannelID, Kind: row.Kind, Name: row.Name, Description: row.Description,
		Enabled: row.Enabled, SortOrder: row.SortOrder, PointSource: row.PointSource, CarrierSlug: row.CarrierSlug,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, BlockModes: map[string]Mode{},
	}
	_ = json.Unmarshal(row.BlockModes, &o.BlockModes)
	return o
}

func toManualPoint(row db.DeliveryPoint) ManualPoint {
	return ManualPoint{
		ID: row.ID, Kind: row.Kind, Name: row.Name, Instructions: row.Instructions, Hours: row.Hours, Active: row.IsActive, SortOrder: row.SortOrder,
		Address: Address{
			Name: row.Name, Street1: row.Street1, Street2: row.Street2, City: row.City, Region: row.Region,
			PostalCode: row.PostalCode, CountryCode: row.CountryCode,
		},
	}
}

func toManualPoints(rows []db.DeliveryPoint) []ManualPoint {
	out := make([]ManualPoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, toManualPoint(row))
	}
	return out
}

func nonNilModes(m map[string]Mode) map[string]Mode {
	if m == nil {
		return map[string]Mode{}
	}
	return m
}
