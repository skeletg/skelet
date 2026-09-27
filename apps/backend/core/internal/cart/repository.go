package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(tx *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(tx)}
}

func (r *postgresRepository) GetCartByID(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	row, err := r.queries.GetCartByID(ctx, cartID)
	if err != nil {
		return Cart{}, fmt.Errorf("get cart: %w", err)
	}
	return toCart(row), nil
}

func (r *postgresRepository) GetCartByCustomerAndChannel(ctx context.Context, customerID, channelID uuid.UUID) (Cart, error) {
	row, err := r.queries.GetCartByCustomerAndChannel(ctx, db.GetCartByCustomerAndChannelParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
		ChannelID:  channelID,
	})
	if err != nil {
		return Cart{}, fmt.Errorf("get cart: %w", err)
	}
	return toCart(row), nil
}

func (r *postgresRepository) CreateCart(ctx context.Context, customerID *uuid.UUID, channelID uuid.UUID) (Cart, error) {
	var param uuid.NullUUID
	if customerID != nil {
		param = uuid.NullUUID{UUID: *customerID, Valid: true}
	}
	row, err := r.queries.CreateCart(ctx, db.CreateCartParams{
		CustomerID: param,
		ChannelID:  channelID,
	})
	if err != nil {
		return Cart{}, fmt.Errorf("create cart: %w", err)
	}
	return toCart(row), nil
}

func (r *postgresRepository) ListCarts(ctx context.Context, f CartFilter) ([]CartListItem, error) {
	rows, err := r.queries.ListCarts(ctx, db.ListCartsParams{
		ChannelID:     f.ChannelID,
		CustomerID:    sqlnull.UUID(f.CustomerID),
		Status:        sqlnull.String(f.Status),
		UpdatedBefore: sqlnull.Time(f.UpdatedBefore),
		Search:        sqlnull.String(f.Search),
		Off:           f.Offset,
		Lim:           f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list carts: %w", err)
	}
	out := make([]CartListItem, len(rows))
	for i, row := range rows {
		out[i] = CartListItem{
			ID:                row.ID,
			CustomerID:        sqlnull.UUIDPtr(row.CustomerID),
			ChannelID:         row.ChannelID,
			Status:            row.Status,
			CustomerEmail:     sqlnull.StringPtr(row.CustomerEmail),
			CustomerFirstName: sqlnull.StringPtr(row.CustomerFirstName),
			CustomerLastName:  sqlnull.StringPtr(row.CustomerLastName),
			ItemCount:         row.ItemCount,
			Subtotal:          row.Subtotal,
			CreatedAt:         row.CreatedAt,
			UpdatedAt:         row.UpdatedAt,
		}
	}
	return out, nil
}

func (r *postgresRepository) CountCarts(ctx context.Context, f CartFilter) (int32, error) {
	n, err := r.queries.CountCarts(ctx, db.CountCartsParams{
		ChannelID:     f.ChannelID,
		CustomerID:    sqlnull.UUID(f.CustomerID),
		Status:        sqlnull.String(f.Status),
		UpdatedBefore: sqlnull.Time(f.UpdatedBefore),
		Search:        sqlnull.String(f.Search),
	})
	if err != nil {
		return 0, fmt.Errorf("count carts: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) ListAbandoned(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]AbandonedCart, error) {
	rows, err := r.queries.AbandonedCarts(ctx, db.AbandonedCartsParams{ChannelID: channelID, Threshold: threshold, MaxAge: maxAge, Lim: limit})
	if err != nil {
		return nil, fmt.Errorf("list abandoned carts: %w", err)
	}
	out := make([]AbandonedCart, 0, len(rows))
	for _, row := range rows {
		ac := AbandonedCart{ID: row.ID, ChannelID: row.ChannelID, Email: row.Email, LastActivity: row.LastActivity}
		if row.CustomerID.Valid {
			ac.CustomerID = row.CustomerID.UUID
		}
		// SQL already filters to contactable carts, but guard against a row with neither
		if ac.CustomerID == uuid.Nil && ac.Email == "" {
			continue
		}
		out = append(out, ac)
	}
	return out, nil
}

func (r *postgresRepository) MarkAbandonedNotified(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return r.queries.MarkCartsAbandonedNotified(ctx, ids)
}

func (r *postgresRepository) SetEmail(ctx context.Context, cartID uuid.UUID, email string) error {
	return r.queries.SetCartEmail(ctx, db.SetCartEmailParams{ID: cartID, Email: email})
}

func (r *postgresRepository) SetNote(ctx context.Context, cartID uuid.UUID, note string) error {
	return r.queries.SetCartNote(ctx, db.SetCartNoteParams{ID: cartID, Note: note})
}

func (r *postgresRepository) PruneStale(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.queries.PruneStaleCarts(ctx, cutoff)
}

func toSavedItem(row db.CartSavedItem) SavedItem {
	return SavedItem{
		ID:          row.ID,
		CartID:      row.CartID,
		ProductSku:  row.ProductSku,
		ProductName: row.ProductName,
		Quantity:    row.Quantity,
		CreatedAt:   row.CreatedAt,
	}
}

func (r *postgresRepository) SaveItem(ctx context.Context, cartID uuid.UUID, sku, name string, qty float64) (SavedItem, error) {
	row, err := r.queries.SaveCartItem(ctx, db.SaveCartItemParams{
		CartID: cartID, ProductSku: sku, ProductName: name, Quantity: qty,
	})
	if err != nil {
		return SavedItem{}, fmt.Errorf("save cart item: %w", err)
	}
	return toSavedItem(row), nil
}

func (r *postgresRepository) GetSavedItem(ctx context.Context, id uuid.UUID) (SavedItem, error) {
	row, err := r.queries.GetSavedItem(ctx, id)
	if err != nil {
		return SavedItem{}, fmt.Errorf("get saved item: %w", err)
	}
	return toSavedItem(row), nil
}

func (r *postgresRepository) DeleteSavedItem(ctx context.Context, id, cartID uuid.UUID) error {
	return r.queries.DeleteSavedItem(ctx, db.DeleteSavedItemParams{ID: id, CartID: cartID})
}

func (r *postgresRepository) ListSavedItems(ctx context.Context, cartID uuid.UUID) ([]SavedItem, error) {
	rows, err := r.queries.ListSavedItemsByCart(ctx, cartID)
	if err != nil {
		return nil, fmt.Errorf("list saved items: %w", err)
	}
	out := make([]SavedItem, len(rows))
	for i, row := range rows {
		out[i] = toSavedItem(row)
	}
	return out, nil
}

func (r *postgresRepository) ReassignCart(ctx context.Context, cartID, customerID uuid.UUID) (Cart, error) {
	row, err := r.queries.ReassignCartToCustomer(ctx, db.ReassignCartToCustomerParams{
		ID:         cartID,
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Cart{}, fmt.Errorf("cart not eligible for adoption")
		}
		return Cart{}, fmt.Errorf("reassign cart: %w", err)
	}
	return toCart(row), nil
}

func (r *postgresRepository) GetCartLineByID(ctx context.Context, lineID uuid.UUID) (CartLine, error) {
	row, err := r.queries.GetCartLineByID(ctx, lineID)
	if err != nil {
		return CartLine{}, fmt.Errorf("get cart line: %w", err)
	}
	return toCartLine(row), nil
}

func (r *postgresRepository) ListCartLinesByCart(ctx context.Context, cartID uuid.UUID) ([]CartLine, error) {
	rows, err := r.queries.ListCartLinesByCart(ctx, cartID)
	if err != nil {
		return nil, fmt.Errorf("list cart lines: %w", err)
	}
	lines := make([]CartLine, len(rows))
	lineIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		lines[i] = toCartLine(row)
		lineIDs[i] = row.ID
	}
	// Hydrate components in one batch so bundle lines arrive complete (simple lines stay empty)
	if len(lineIDs) > 0 {
		comps, err := r.queries.ListCartLineComponentsByLineIDs(ctx, lineIDs)
		if err != nil {
			return nil, fmt.Errorf("list cart line components: %w", err)
		}
		byLine := make(map[uuid.UUID][]CartLineComponent, len(lineIDs))
		for _, c := range comps {
			byLine[c.CartLineID] = append(byLine[c.CartLineID], toCartLineComponent(c))
		}
		for i := range lines {
			lines[i].Components = byLine[lines[i].ID]
		}
		// Hydrate option picks in one batch too
		opts, err := r.queries.ListCartLineOptionsByLineIDs(ctx, lineIDs)
		if err != nil {
			return nil, fmt.Errorf("list cart line options: %w", err)
		}
		optByLine := make(map[uuid.UUID][]CartLineOption, len(lineIDs))
		for _, o := range opts {
			optByLine[o.CartLineID] = append(optByLine[o.CartLineID], toCartLineOption(o))
		}
		for i := range lines {
			lines[i].Options = optByLine[lines[i].ID]
		}
	}
	return lines, nil
}

func (r *postgresRepository) UpsertCartLineBySku(ctx context.Context, cartID uuid.UUID, input AddCartLineInput) (CartLine, error) {
	row, err := r.queries.UpsertCartLineBySku(ctx, db.UpsertCartLineBySkuParams{
		CartID:      cartID,
		ProductName: input.ProductName,
		ProductSku:  input.ProductSku,
		UnitPrice:   input.UnitPrice,
		Quantity:    input.Quantity,
	})
	if err != nil {
		return CartLine{}, fmt.Errorf("upsert cart line: %w", err)
	}
	return toCartLine(row), nil
}

// UpsertBundleCartLine writes the bundle line then replaces its components - On
// SKU conflict it OVERWRITES quantity/price/name (the simple upsert sums instead)
func (r *postgresRepository) UpsertBundleCartLine(ctx context.Context, cartID uuid.UUID, input AddBundleLineInput) (CartLine, error) {
	row, err := r.queries.UpsertBundleCartLine(ctx, db.UpsertBundleCartLineParams{
		CartID:      cartID,
		ProductName: input.ProductName,
		ProductSku:  input.ProductSku,
		UnitPrice:   input.UnitPrice,
		Quantity:    input.Quantity,
	})
	if err != nil {
		return CartLine{}, fmt.Errorf("upsert bundle cart line: %w", err)
	}
	if err := r.queries.DeleteCartLineComponentsByLine(ctx, row.ID); err != nil {
		return CartLine{}, fmt.Errorf("clear bundle line components: %w", err)
	}
	line := toCartLine(row)
	line.Components = make([]CartLineComponent, 0, len(input.Components))
	for _, c := range input.Components {
		comp, err := r.queries.InsertCartLineComponent(ctx, db.InsertCartLineComponentParams{
			CartLineID:        row.ID,
			BundleOptionID:    c.BundleOptionID,
			BundleSelectionID: c.BundleSelectionID,
			ProductID:         c.ProductID,
			ProductSku:        c.ProductSku,
			ProductName:       c.ProductName,
			UnitPrice:         c.UnitPrice,
			Quantity:          c.Quantity,
			SortOrder:         c.SortOrder,
		})
		if err != nil {
			return CartLine{}, fmt.Errorf("insert bundle line component: %w", err)
		}
		line.Components = append(line.Components, toCartLineComponent(comp))
	}
	return line, nil
}

// UpsertConfiguredCartLine writes the simple line then replaces its option rows Like the bundle path it OVERWRITES
// quantity/price/name on SKU conflict (a reconfigure, not an accumulation)
func (r *postgresRepository) UpsertConfiguredCartLine(ctx context.Context, cartID uuid.UUID, input AddConfiguredLineInput) (CartLine, error) {
	row, err := r.queries.UpsertConfiguredCartLine(ctx, db.UpsertConfiguredCartLineParams{
		CartID:      cartID,
		ProductName: input.ProductName,
		ProductSku:  input.ProductSku,
		UnitPrice:   input.UnitPrice,
		Quantity:    input.Quantity,
	})
	if err != nil {
		return CartLine{}, fmt.Errorf("upsert configured cart line: %w", err)
	}
	if err := r.queries.DeleteCartLineOptionsByLine(ctx, row.ID); err != nil {
		return CartLine{}, fmt.Errorf("clear cart line options: %w", err)
	}
	line := toCartLine(row)
	line.Options = make([]CartLineOption, 0, len(input.Options))
	for _, o := range input.Options {
		valueID := uuid.NullUUID{}
		if o.OptionValueID != nil {
			valueID = uuid.NullUUID{UUID: *o.OptionValueID, Valid: true}
		}
		opt, err := r.queries.InsertCartLineOption(ctx, db.InsertCartLineOptionParams{
			CartLineID:      row.ID,
			ProductOptionID: o.ProductOptionID,
			OptionValueID:   valueID,
			InputType:       o.InputType,
			OptionTitle:     o.OptionTitle,
			ValueLabel:      o.ValueLabel,
			TextValue:       o.TextValue,
			PriceDelta:      o.PriceDelta,
			Quantity:        o.Quantity,
			SortOrder:       o.SortOrder,
			FileUrl:         o.FileURL,
		})
		if err != nil {
			return CartLine{}, fmt.Errorf("insert cart line option: %w", err)
		}
		line.Options = append(line.Options, toCartLineOption(opt))
	}
	return line, nil
}

func (r *postgresRepository) UpdateCartLineQuantity(ctx context.Context, lineID uuid.UUID, quantity float64) (CartLine, error) {
	row, err := r.queries.UpdateCartLineQuantity(ctx, db.UpdateCartLineQuantityParams{
		ID:       lineID,
		Quantity: quantity,
	})
	if err != nil {
		return CartLine{}, fmt.Errorf("update cart line: %w", err)
	}
	return toCartLine(row), nil
}

func (r *postgresRepository) UpdateCartLineQuantityPrice(ctx context.Context, lineID uuid.UUID, quantity float64, unitPrice int64) (CartLine, error) {
	row, err := r.queries.UpdateCartLineQuantityPrice(ctx, db.UpdateCartLineQuantityPriceParams{
		ID:        lineID,
		Quantity:  quantity,
		UnitPrice: unitPrice,
	})
	if err != nil {
		return CartLine{}, fmt.Errorf("update cart line: %w", err)
	}
	return toCartLine(row), nil
}

func (r *postgresRepository) RemoveCartLine(ctx context.Context, lineID uuid.UUID) error {
	if err := r.queries.DeleteCartLine(ctx, lineID); err != nil {
		return fmt.Errorf("remove cart line: %w", err)
	}
	return nil
}

func (r *postgresRepository) ClearCartLines(ctx context.Context, cartID uuid.UUID) error {
	if err := r.queries.DeleteCartLinesByCart(ctx, cartID); err != nil {
		return fmt.Errorf("clear cart lines: %w", err)
	}
	return nil
}

func (r *postgresRepository) ConvertCart(ctx context.Context, cartID uuid.UUID) error {
	// SetCartStatus only updates status='active' - zero rows means a concurrent placement/merge won
	if _, err := r.queries.SetCartStatus(ctx, db.SetCartStatusParams{ID: cartID, Status: "converted"}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCartAlreadyConverted
		}
		return fmt.Errorf("convert cart: %w", err)
	}
	return nil
}

func (r *postgresRepository) SetCartMerged(ctx context.Context, cartID uuid.UUID) error {
	if _, err := r.queries.SetCartStatus(ctx, db.SetCartStatusParams{ID: cartID, Status: "merged"}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCartAlreadyConverted
		}
		return fmt.Errorf("set cart merged: %w", err)
	}
	return nil
}

// --- mappers ---

func toCart(row db.Cart) Cart {
	var customerID *uuid.UUID
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		customerID = &id
	}
	return Cart{
		ID:         row.ID,
		CustomerID: customerID,
		ChannelID:  row.ChannelID,
		Status:     row.Status,
		Note:       row.Note,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func toCartLine(row db.CartLine) CartLine {
	return CartLine{
		ID:          row.ID,
		CartID:      row.CartID,
		ProductName: row.ProductName,
		ProductSku:  row.ProductSku,
		UnitPrice:   row.UnitPrice,
		Quantity:    row.Quantity,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toCartLineComponent(row db.CartLineComponent) CartLineComponent {
	return CartLineComponent{
		ID:                row.ID,
		CartLineID:        row.CartLineID,
		BundleOptionID:    row.BundleOptionID,
		BundleSelectionID: row.BundleSelectionID,
		ProductID:         row.ProductID,
		ProductSku:        row.ProductSku,
		ProductName:       row.ProductName,
		UnitPrice:         row.UnitPrice,
		Quantity:          row.Quantity,
		SortOrder:         row.SortOrder,
		CreatedAt:         row.CreatedAt,
	}
}

func toCartLineOption(row db.CartLineOption) CartLineOption {
	o := CartLineOption{
		ID:              row.ID,
		ProductOptionID: row.ProductOptionID,
		InputType:       row.InputType,
		OptionTitle:     row.OptionTitle,
		ValueLabel:      row.ValueLabel,
		TextValue:       row.TextValue,
		PriceDelta:      row.PriceDelta,
		Quantity:        row.Quantity,
		SortOrder:       row.SortOrder,
		FileURL:         row.FileUrl,
	}
	if row.OptionValueID.Valid {
		v := row.OptionValueID.UUID
		o.OptionValueID = &v
	}
	return o
}

// IsSQLNoRows lets the service layer spot "no rows" without importing database/sql. errors.Is so wrapped chains
// (fmt.Errorf("...: %w", sql.ErrNoRows)) still resolve - otherwise every %w repo method would hide the no-rows signal
func IsSQLNoRows(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, sql.ErrNoRows) || err.Error() == "sql: no rows in result set"
}
