package shipping

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

var (
	ErrZoneNotFound   = errors.New("shipping: zone not found")
	ErrCodeTaken      = errors.New("shipping: code already exists in this channel")
	ErrMethodNotFound = errors.New("shipping: method not found")
)

// Repository is the thin DB layer - Zone.Countries hydration is service-level
type Repository interface {
	WithTx(t *sql.Tx) Repository

	CreateZone(ctx context.Context, in Zone) (Zone, error)
	UpdateZone(ctx context.Context, in Zone) (Zone, error)
	DeleteZone(ctx context.Context, id uuid.UUID) error
	GetZone(ctx context.Context, id uuid.UUID) (Zone, error)
	ListZonesByChannel(ctx context.Context, channelID uuid.UUID) ([]Zone, error)
	FindZoneForCountry(ctx context.Context, channelID uuid.UUID, country string) (Zone, error)

	AddCountryToZone(ctx context.Context, zoneID uuid.UUID, country string) error
	RemoveCountryFromZone(ctx context.Context, zoneID uuid.UUID, country string) error
	ListCountriesForZone(ctx context.Context, zoneID uuid.UUID) ([]string, error)
	ClearZoneCountries(ctx context.Context, zoneID uuid.UUID) error

	CreateMethod(ctx context.Context, in Method) (Method, error)
	UpdateMethod(ctx context.Context, in Method) (Method, error)
	DeleteMethod(ctx context.Context, id uuid.UUID) error
	GetMethod(ctx context.Context, id uuid.UUID) (Method, error)
	GetMethodByCode(ctx context.Context, zoneID uuid.UUID, code string) (Method, error)
	ListMethodsByZone(ctx context.Context, zoneID uuid.UUID) ([]Method, error)
	ListActiveMethodsByZone(ctx context.Context, zoneID uuid.UUID) ([]Method, error)

	AddCountryToMethod(ctx context.Context, methodID uuid.UUID, country string) error
	ListCountriesForMethod(ctx context.Context, methodID uuid.UUID) ([]string, error)
	ClearMethodCountries(ctx context.Context, methodID uuid.UUID) error

	// Per-channel carrier config (the shipping mirror of payment_methods)
	UpsertCarrier(ctx context.Context, channelID uuid.UUID, slug string, isEnabled bool, displayName string) (CarrierConfig, error)
	GetCarrierConfig(ctx context.Context, channelID uuid.UUID, slug string) (CarrierConfig, error)
	ListCarrierConfigsByChannel(ctx context.Context, channelID uuid.UUID) ([]CarrierConfig, error)
	SetCarrierEnabled(ctx context.Context, channelID uuid.UUID, slug string, enabled bool) (CarrierConfig, error)
	SetCarrierSortOrder(ctx context.Context, channelID uuid.UUID, slug string, sortOrder int32) (CarrierConfig, error)
	SetCarrierCountryMode(ctx context.Context, channelID uuid.UUID, slug string, mode string) (CarrierConfig, error)
	AddCountryToCarrier(ctx context.Context, carrierID uuid.UUID, country string) error
	ListCountriesForCarrier(ctx context.Context, carrierID uuid.UUID) ([]string, error)
	ClearCarrierCountries(ctx context.Context, carrierID uuid.UUID) error

	// Boxes (cartonization)
	UpsertBox(ctx context.Context, in Box) (Box, error)
	GetBox(ctx context.Context, id uuid.UUID) (Box, error)
	DeleteBox(ctx context.Context, id uuid.UUID) error
	ListBoxesByChannel(ctx context.Context, channelID uuid.UUID) ([]Box, error)
	ListActiveBoxesByChannel(ctx context.Context, channelID uuid.UUID) ([]Box, error)
}

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t)}
}

func (r *postgresRepository) CreateZone(ctx context.Context, in Zone) (Zone, error) {
	row, err := r.queries.CreateShippingZone(ctx, db.CreateShippingZoneParams{
		ChannelID: in.ChannelID,
		Code:      in.Code,
		Name:      in.Name,
		SortOrder: in.SortOrder,
		IsActive:  in.IsActive,
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Zone{}, ErrCodeTaken
		}
		return Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return toZone(row), nil
}

func (r *postgresRepository) UpdateZone(ctx context.Context, in Zone) (Zone, error) {
	row, err := r.queries.UpdateShippingZone(ctx, db.UpdateShippingZoneParams{
		ID:        in.ID,
		Code:      in.Code,
		Name:      in.Name,
		SortOrder: in.SortOrder,
		IsActive:  in.IsActive,
	})
	if err != nil {
		return Zone{}, fmt.Errorf("update zone: %w", err)
	}
	return toZone(row), nil
}

func (r *postgresRepository) DeleteZone(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteShippingZone(ctx, id); err != nil {
		return fmt.Errorf("delete zone: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetZone(ctx context.Context, id uuid.UUID) (Zone, error) {
	row, err := r.queries.GetShippingZone(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, fmt.Errorf("get zone: %w", err)
	}
	return toZone(row), nil
}

func (r *postgresRepository) ListZonesByChannel(ctx context.Context, channelID uuid.UUID) ([]Zone, error) {
	rows, err := r.queries.ListShippingZonesByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	out := make([]Zone, len(rows))
	for i, row := range rows {
		out[i] = toZone(row)
	}
	return out, nil
}

func (r *postgresRepository) FindZoneForCountry(ctx context.Context, channelID uuid.UUID, country string) (Zone, error) {
	row, err := r.queries.FindZoneForCountry(ctx, db.FindZoneForCountryParams{
		ChannelID:   channelID,
		CountryCode: country,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, fmt.Errorf("find zone for country: %w", err)
	}
	return toZone(row), nil
}

func (r *postgresRepository) AddCountryToZone(ctx context.Context, zoneID uuid.UUID, country string) error {
	if err := r.queries.AddCountryToZone(ctx, db.AddCountryToZoneParams{
		ZoneID:      zoneID,
		CountryCode: country,
	}); err != nil {
		return fmt.Errorf("add country to zone: %w", err)
	}
	return nil
}

func (r *postgresRepository) RemoveCountryFromZone(ctx context.Context, zoneID uuid.UUID, country string) error {
	if err := r.queries.RemoveCountryFromZone(ctx, db.RemoveCountryFromZoneParams{
		ZoneID:      zoneID,
		CountryCode: country,
	}); err != nil {
		return fmt.Errorf("remove country from zone: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListCountriesForZone(ctx context.Context, zoneID uuid.UUID) ([]string, error) {
	rows, err := r.queries.ListCountriesForZone(ctx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("list countries for zone: %w", err)
	}
	return rows, nil
}

func (r *postgresRepository) ClearZoneCountries(ctx context.Context, zoneID uuid.UUID) error {
	if err := r.queries.ClearZoneCountries(ctx, zoneID); err != nil {
		return fmt.Errorf("clear zone countries: %w", err)
	}
	return nil
}

func (r *postgresRepository) CreateMethod(ctx context.Context, in Method) (Method, error) {
	row, err := r.queries.CreateShippingMethod(ctx, db.CreateShippingMethodParams{
		ZoneID:         in.ZoneID,
		Code:           in.Code,
		Name:           in.Name,
		CalcType:       string(in.CalcType),
		FlatCents:      in.FlatCents,
		FreeAboveCents: in.FreeAboveCents,
		SortOrder:      in.SortOrder,
		IsActive:       in.IsActive,
		CountryMode:    normalizeMethodMode(in.CountryMode),
	})
	if err != nil {
		if pgdb.IsUniqueViolation(err) {
			return Method{}, ErrCodeTaken
		}
		return Method{}, fmt.Errorf("create method: %w", err)
	}
	return toMethod(row), nil
}

func (r *postgresRepository) UpdateMethod(ctx context.Context, in Method) (Method, error) {
	row, err := r.queries.UpdateShippingMethod(ctx, db.UpdateShippingMethodParams{
		ID:             in.ID,
		Code:           in.Code,
		Name:           in.Name,
		CalcType:       string(in.CalcType),
		FlatCents:      in.FlatCents,
		FreeAboveCents: in.FreeAboveCents,
		SortOrder:      in.SortOrder,
		IsActive:       in.IsActive,
		CountryMode:    normalizeMethodMode(in.CountryMode),
	})
	if err != nil {
		return Method{}, fmt.Errorf("update method: %w", err)
	}
	return toMethod(row), nil
}

func (r *postgresRepository) DeleteMethod(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteShippingMethod(ctx, id); err != nil {
		return fmt.Errorf("delete method: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetMethod(ctx context.Context, id uuid.UUID) (Method, error) {
	row, err := r.queries.GetShippingMethod(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Method{}, ErrMethodNotFound
		}
		return Method{}, fmt.Errorf("get method: %w", err)
	}
	return toMethod(row), nil
}

func (r *postgresRepository) GetMethodByCode(ctx context.Context, zoneID uuid.UUID, code string) (Method, error) {
	row, err := r.queries.GetShippingMethodByCode(ctx, db.GetShippingMethodByCodeParams{
		ZoneID: zoneID,
		Code:   code,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Method{}, ErrMethodNotFound
		}
		return Method{}, fmt.Errorf("get method by code: %w", err)
	}
	return toMethod(row), nil
}

func (r *postgresRepository) ListMethodsByZone(ctx context.Context, zoneID uuid.UUID) ([]Method, error) {
	rows, err := r.queries.ListShippingMethodsByZone(ctx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("list methods by zone: %w", err)
	}
	out := make([]Method, len(rows))
	for i, row := range rows {
		out[i] = toMethod(row)
	}
	return out, nil
}

func (r *postgresRepository) ListActiveMethodsByZone(ctx context.Context, zoneID uuid.UUID) ([]Method, error) {
	rows, err := r.queries.ListActiveShippingMethodsByZone(ctx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("list active methods by zone: %w", err)
	}
	out := make([]Method, len(rows))
	for i, row := range rows {
		out[i] = toMethod(row)
	}
	return out, nil
}

func toZone(row db.ShippingZone) Zone {
	return Zone{
		ID:        row.ID,
		ChannelID: row.ChannelID,
		Code:      row.Code,
		Name:      row.Name,
		SortOrder: row.SortOrder,
		IsActive:  row.IsActive,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toMethod(row db.ShippingMethod) Method {
	return Method{
		ID:             row.ID,
		ZoneID:         row.ZoneID,
		Code:           row.Code,
		Name:           row.Name,
		CalcType:       CalcType(row.CalcType),
		FlatCents:      row.FlatCents,
		FreeAboveCents: row.FreeAboveCents,
		SortOrder:      row.SortOrder,
		IsActive:       row.IsActive,
		CountryMode:    row.CountryMode,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

// normalizeMethodMode coerces an empty/unknown mode to "all" for the DB CHECK constraint
func normalizeMethodMode(mode string) string {
	if mode == "specific" {
		return "specific"
	}
	return "all"
}

func (r *postgresRepository) AddCountryToMethod(ctx context.Context, methodID uuid.UUID, country string) error {
	if err := r.queries.AddCountryToMethod(ctx, db.AddCountryToMethodParams{
		MethodID:    methodID,
		CountryCode: country,
	}); err != nil {
		return fmt.Errorf("add country to method: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListCountriesForMethod(ctx context.Context, methodID uuid.UUID) ([]string, error) {
	rows, err := r.queries.ListCountriesForMethod(ctx, methodID)
	if err != nil {
		return nil, fmt.Errorf("list countries for method: %w", err)
	}
	return rows, nil
}

func (r *postgresRepository) ClearMethodCountries(ctx context.Context, methodID uuid.UUID) error {
	if err := r.queries.ClearMethodCountries(ctx, methodID); err != nil {
		return fmt.Errorf("clear method countries: %w", err)
	}
	return nil
}

// ---------- Per-channel carrier config ----------

func toCarrierConfig(row db.ShippingCarrier) CarrierConfig {
	return CarrierConfig{
		ID:          row.ID,
		ChannelID:   row.ChannelID,
		CarrierSlug: row.CarrierSlug,
		IsEnabled:   row.IsEnabled,
		DisplayName: row.DisplayName,
		SortOrder:   row.SortOrder,
		CountryMode: row.CountryMode,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func (r *postgresRepository) UpsertCarrier(ctx context.Context, channelID uuid.UUID, slug string, isEnabled bool, displayName string) (CarrierConfig, error) {
	row, err := r.queries.UpsertShippingCarrier(ctx, db.UpsertShippingCarrierParams{
		ChannelID:   channelID,
		CarrierSlug: slug,
		IsEnabled:   isEnabled,
		DisplayName: displayName,
	})
	if err != nil {
		return CarrierConfig{}, fmt.Errorf("upsert carrier: %w", err)
	}
	return toCarrierConfig(row), nil
}

func (r *postgresRepository) GetCarrierConfig(ctx context.Context, channelID uuid.UUID, slug string) (CarrierConfig, error) {
	row, err := r.queries.GetShippingCarrier(ctx, db.GetShippingCarrierParams{ChannelID: channelID, CarrierSlug: slug})
	if errors.Is(err, sql.ErrNoRows) {
		return CarrierConfig{}, ErrCarrierNotFound
	}
	if err != nil {
		return CarrierConfig{}, fmt.Errorf("get carrier config: %w", err)
	}
	return toCarrierConfig(row), nil
}

func (r *postgresRepository) ListCarrierConfigsByChannel(ctx context.Context, channelID uuid.UUID) ([]CarrierConfig, error) {
	rows, err := r.queries.ListShippingCarriersByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list carrier configs: %w", err)
	}
	out := make([]CarrierConfig, len(rows))
	for i, row := range rows {
		out[i] = toCarrierConfig(row)
	}
	return out, nil
}

func (r *postgresRepository) SetCarrierEnabled(ctx context.Context, channelID uuid.UUID, slug string, enabled bool) (CarrierConfig, error) {
	row, err := r.queries.SetShippingCarrierEnabled(ctx, db.SetShippingCarrierEnabledParams{ChannelID: channelID, CarrierSlug: slug, IsEnabled: enabled})
	if err != nil {
		return CarrierConfig{}, fmt.Errorf("set carrier enabled: %w", err)
	}
	return toCarrierConfig(row), nil
}

func (r *postgresRepository) SetCarrierSortOrder(ctx context.Context, channelID uuid.UUID, slug string, sortOrder int32) (CarrierConfig, error) {
	row, err := r.queries.SetShippingCarrierSortOrder(ctx, db.SetShippingCarrierSortOrderParams{ChannelID: channelID, CarrierSlug: slug, SortOrder: sortOrder})
	if err != nil {
		return CarrierConfig{}, fmt.Errorf("set carrier sort order: %w", err)
	}
	return toCarrierConfig(row), nil
}

func (r *postgresRepository) SetCarrierCountryMode(ctx context.Context, channelID uuid.UUID, slug string, mode string) (CarrierConfig, error) {
	row, err := r.queries.SetShippingCarrierCountryMode(ctx, db.SetShippingCarrierCountryModeParams{ChannelID: channelID, CarrierSlug: slug, CountryMode: normalizeMethodMode(mode)})
	if err != nil {
		return CarrierConfig{}, fmt.Errorf("set carrier country mode: %w", err)
	}
	return toCarrierConfig(row), nil
}

func (r *postgresRepository) AddCountryToCarrier(ctx context.Context, carrierID uuid.UUID, country string) error {
	if err := r.queries.AddCountryToShippingCarrier(ctx, db.AddCountryToShippingCarrierParams{CarrierID: carrierID, CountryCode: country}); err != nil {
		return fmt.Errorf("add country to carrier: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListCountriesForCarrier(ctx context.Context, carrierID uuid.UUID) ([]string, error) {
	rows, err := r.queries.ListCountriesForShippingCarrier(ctx, carrierID)
	if err != nil {
		return nil, fmt.Errorf("list countries for carrier: %w", err)
	}
	return rows, nil
}

func (r *postgresRepository) ClearCarrierCountries(ctx context.Context, carrierID uuid.UUID) error {
	if err := r.queries.ClearShippingCarrierCountries(ctx, carrierID); err != nil {
		return fmt.Errorf("clear carrier countries: %w", err)
	}
	return nil
}

// ---------- Boxes (cartonization) ----------

func toBox(row db.ShippingBox) Box {
	return Box{
		ID:             row.ID,
		ChannelID:      row.ChannelID,
		Code:           row.Code,
		Name:           row.Name,
		InnerLengthMm:  row.InnerLengthMm,
		InnerWidthMm:   row.InnerWidthMm,
		InnerHeightMm:  row.InnerHeightMm,
		MaxWeightGrams: row.MaxWeightGrams,
		BoxWeightGrams: row.BoxWeightGrams,
		IsActive:       row.IsActive,
		SortOrder:      row.SortOrder,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func (r *postgresRepository) GetBox(ctx context.Context, id uuid.UUID) (Box, error) {
	row, err := r.queries.GetShippingBox(ctx, id)
	if err != nil {
		return Box{}, fmt.Errorf("get shipping box: %w", err)
	}
	return toBox(row), nil
}

func (r *postgresRepository) UpsertBox(ctx context.Context, in Box) (Box, error) {
	row, err := r.queries.UpsertShippingBox(ctx, db.UpsertShippingBoxParams{
		ChannelID:      in.ChannelID,
		Code:           in.Code,
		Name:           in.Name,
		InnerLengthMm:  in.InnerLengthMm,
		InnerWidthMm:   in.InnerWidthMm,
		InnerHeightMm:  in.InnerHeightMm,
		MaxWeightGrams: in.MaxWeightGrams,
		BoxWeightGrams: in.BoxWeightGrams,
		IsActive:       in.IsActive,
		SortOrder:      in.SortOrder,
	})
	if err != nil {
		return Box{}, fmt.Errorf("upsert box: %w", err)
	}
	return toBox(row), nil
}

func (r *postgresRepository) DeleteBox(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteShippingBox(ctx, id); err != nil {
		return fmt.Errorf("delete box: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListBoxesByChannel(ctx context.Context, channelID uuid.UUID) ([]Box, error) {
	rows, err := r.queries.ListShippingBoxesByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list boxes: %w", err)
	}
	out := make([]Box, len(rows))
	for i, row := range rows {
		out[i] = toBox(row)
	}
	return out, nil
}

func (r *postgresRepository) ListActiveBoxesByChannel(ctx context.Context, channelID uuid.UUID) ([]Box, error) {
	rows, err := r.queries.ListActiveShippingBoxesByChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list active boxes: %w", err)
	}
	out := make([]Box, len(rows))
	for i, row := range rows {
		out[i] = toBox(row)
	}
	return out, nil
}
