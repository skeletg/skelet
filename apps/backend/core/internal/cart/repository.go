package cart

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

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
		Kind:          sqlnull.String(f.Kind),
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
			Kind:              row.Kind,
			Email:             row.Email,
			OfferExpiresAt:    sqlnull.TimePtr(row.OfferExpiresAt),
			OfferSentAt:       sqlnull.TimePtr(row.OfferSentAt),
			OfferOpenedAt:     sqlnull.TimePtr(row.OfferOpenedAt),
			OfferRevokedAt:    sqlnull.TimePtr(row.OfferRevokedAt),
			OrderID:           sqlnull.UUIDPtr(row.OrderID),
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
		Kind:          sqlnull.String(f.Kind),
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

func (r *postgresRepository) LockCart(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	row, err := r.queries.LockCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	return toCart(row), nil
}

func (r *postgresRepository) SetCheckout(ctx context.Context, cartID uuid.UUID, c Checkout) error {
	shipping, err := encodeAddress(c.ShippingAddress)
	if err != nil {
		return err
	}
	billing, err := encodeAddress(c.BillingAddress)
	if err != nil {
		return err
	}
	codes := c.GiftCardCodes
	if codes == nil {
		codes = []string{}
	}
	var pickup pqtype.NullRawMessage
	if c.PickupContact != nil {
		b, err := json.Marshal(c.PickupContact)
		if err != nil {
			return fmt.Errorf("encode pickup contact: %w", err)
		}
		pickup = sqlnull.Raw(b)
	}
	ids := c.BusinessIDs
	if ids == nil {
		ids = map[string]string{}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("encode business ids: %w", err)
	}
	purchase := c.PurchaseType
	if purchase == "" {
		purchase = PurchasePersonal
	}
	deliveries := c.Deliveries
	if deliveries == nil {
		deliveries = map[string]DeliverySelection{}
	}
	deliveriesJSON, err := json.Marshal(deliveries)
	if err != nil {
		return fmt.Errorf("encode deliveries: %w", err)
	}
	return r.queries.SetCartCheckout(ctx, db.SetCartCheckoutParams{
		Deliveries:            deliveriesJSON,
		SplitDelivery:         c.SplitDelivery,
		ID:                    cartID,
		ShippingAddress:       shipping,
		ShippingAddressID:     sqlnull.UUID(c.ShippingAddressID),
		BillingSameAsShipping: c.BillingSameAsShipping,
		BillingAddress:        billing,
		BillingAddressID:      sqlnull.UUID(c.BillingAddressID),
		ShippingCarrierSlug:   c.ShippingCarrierSlug,
		ShippingMethodCode:    c.ShippingMethodCode,
		PaymentProviderSlug:   c.PaymentProviderSlug,
		PoNumber:              c.PONumber,
		UseStoreCredit:        c.UseStoreCredit,
		GiftCardCodes:         codes,
		DestinationCountry:    c.Destination.CountryCode,
		DestinationRegion:     c.Destination.Region,
		DestinationPostalCode: c.Destination.PostalCode,
		ContactPhone:          c.ContactPhone,
		PurchaseType:          purchase,
		CompanyName:           c.Company,
		BusinessIds:           idsJSON,
		PickupContact:         pickup,
	})
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

func (r *postgresRepository) DeleteSavedItem(ctx context.Context, id, cartID uuid.UUID) (bool, error) {
	n, err := r.queries.DeleteSavedItem(ctx, db.DeleteSavedItemParams{ID: id, CartID: cartID})
	if err != nil {
		return false, fmt.Errorf("delete saved item: %w", err)
	}
	return n > 0, nil
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

// Writes the bundle line then replaces its components, OVERWRITING quantity/price/name on a SKU conflict (the simple upsert sums instead) - several statements, so callers run it in a transaction
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

// Writes the line then replaces its option rows, OVERWRITING quantity/price/name on a SKU conflict like the bundle path (a reconfigure, not an accumulation) - several statements, so callers run it in a transaction
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
			SourceProductID: sqlnull.UUID(o.SourceProductID),
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

func (r *postgresRepository) SetBundleLineQuantity(ctx context.Context, lineID uuid.UUID, quantity float64) error {
	n, err := r.queries.SetBundleLineQuantity(ctx, db.SetBundleLineQuantityParams{LineID: lineID, Quantity: quantity})
	if err != nil {
		return fmt.Errorf("update bundle line: %w", err)
	}
	if n == 0 {
		return ErrCartLineNotFound
	}
	return nil
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

func (r *postgresRepository) SetCartOrder(ctx context.Context, cartID, orderID uuid.UUID) error {
	return r.queries.SetCartOrder(ctx, db.SetCartOrderParams{ID: cartID, OrderID: uuid.NullUUID{UUID: orderID, Valid: true}})
}

// --- offers ---

func (r *postgresRepository) GetOfferByToken(ctx context.Context, token string) (Cart, error) {
	row, err := r.queries.GetCartByOfferToken(ctx, token)
	if err != nil {
		return Cart{}, fmt.Errorf("load offer by token: %w", err)
	}
	return toCart(row), nil
}

func (r *postgresRepository) CreateOfferCart(ctx context.Context, in CreateOfferInput) (Cart, error) {
	row, err := r.queries.CreateOfferCart(ctx, db.CreateOfferCartParams{
		CustomerID: sqlnull.UUID(in.CustomerID),
		ChannelID:  in.ChannelID,
		Email:      in.Email,
		ExpiresAt:  sql.NullTime{Time: in.ExpiresAt, Valid: true},
		CreatedBy:  sqlnull.UUID(in.CreatedBy),
	})
	if err != nil {
		return Cart{}, fmt.Errorf("create offer cart: %w", err)
	}
	return toCart(row), nil
}

func (r *postgresRepository) SetOffer(ctx context.Context, cartID uuid.UUID, in OfferSettings) error {
	locks := in.Locks
	if locks == nil {
		locks = []string{}
	}
	methods := in.PaymentMethods
	if methods == nil {
		methods = []string{}
	}
	return r.queries.SetCartOffer(ctx, db.SetCartOfferParams{
		ID:             cartID,
		ExpiresAt:      sql.NullTime{Time: in.ExpiresAt, Valid: true},
		Message:        in.Message,
		ShowInAccount:  in.ShowInAccount,
		Locks:          locks,
		PaymentMethods: methods,
	})
}

func (r *postgresRepository) MarkOfferSent(ctx context.Context, cartID uuid.UUID) error {
	return r.queries.MarkCartOfferSent(ctx, cartID)
}

func (r *postgresRepository) MarkOfferOpened(ctx context.Context, cartID uuid.UUID) error {
	return r.queries.MarkCartOfferOpened(ctx, cartID)
}

func (r *postgresRepository) RevokeOffer(ctx context.Context, cartID uuid.UUID) error {
	return r.queries.RevokeCartOffer(ctx, cartID)
}

func (r *postgresRepository) BumpOfferLink(ctx context.Context, cartID uuid.UUID) (int32, error) {
	return r.queries.BumpCartOfferLink(ctx, cartID)
}

func (r *postgresRepository) ListCustomerOffers(ctx context.Context, customerID, channelID uuid.UUID) ([]Cart, error) {
	rows, err := r.queries.ListCustomerCartOffers(ctx, db.ListCustomerCartOffersParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
		ChannelID:  channelID,
	})
	if err != nil {
		return nil, fmt.Errorf("list customer offers: %w", err)
	}
	return toCarts(rows), nil
}

func (r *postgresRepository) ListExpiredOffers(ctx context.Context, limit int32) ([]Cart, error) {
	rows, err := r.queries.ExpiredCartOffers(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired offers: %w", err)
	}
	return toCarts(rows), nil
}

func (r *postgresRepository) MarkOffersExpiredNotified(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return r.queries.MarkCartOfferExpiredNotified(ctx, ids)
}

func (r *postgresRepository) SetLinePrice(ctx context.Context, lineID uuid.UUID, unitPrice int64, override bool) (CartLine, error) {
	row, err := r.queries.SetCartLinePrice(ctx, db.SetCartLinePriceParams{ID: lineID, UnitPrice: unitPrice, PriceOverride: override})
	if err != nil {
		return CartLine{}, fmt.Errorf("set line price: %w", err)
	}
	return toCartLine(row), nil
}

func (r *postgresRepository) InsertCustomLine(ctx context.Context, cartID uuid.UUID, in CustomLineInput) (CartLine, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return CartLine{}, err
	}
	row, err := r.queries.InsertCustomCartLine(ctx, db.InsertCustomCartLineParams{
		CartID:      cartID,
		ProductName: in.Name,
		ProductSku:  CustomLineSKUPrefix + strings.ToUpper(id.String()[24:]), // the line's unique key in the cart - no catalog product carries the prefix
		UnitPrice:   in.UnitPrice,
		Quantity:    in.Quantity,
		TaxClass:    in.TaxClass,
	})
	if err != nil {
		return CartLine{}, fmt.Errorf("add custom line: %w", err)
	}
	return toCartLine(row), nil
}

func toCarts(rows []db.Cart) []Cart {
	out := make([]Cart, len(rows))
	for i, row := range rows {
		out[i] = toCart(row)
	}
	return out
}

// --- mappers ---

func toCart(row db.Cart) Cart {
	var customerID *uuid.UUID
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		customerID = &id
	}
	c := Cart{
		ID:         row.ID,
		CustomerID: customerID,
		ChannelID:  row.ChannelID,
		Status:     row.Status,
		Kind:       row.Kind,
		Email:      row.Email,
		Note:       row.Note,
		Checkout: Checkout{
			ShippingAddress:       decodeAddress(row.ShippingAddress),
			ShippingAddressID:     sqlnull.UUIDPtr(row.ShippingAddressID),
			BillingSameAsShipping: row.BillingSameAsShipping,
			BillingAddress:        decodeAddress(row.BillingAddress),
			BillingAddressID:      sqlnull.UUIDPtr(row.BillingAddressID),
			ShippingCarrierSlug:   row.ShippingCarrierSlug,
			ShippingMethodCode:    row.ShippingMethodCode,
			PaymentProviderSlug:   row.PaymentProviderSlug,
			PONumber:              row.PoNumber,
			UseStoreCredit:        row.UseStoreCredit,
			GiftCardCodes:         row.GiftCardCodes,
			Destination: Destination{
				CountryCode: row.DestinationCountry,
				Region:      row.DestinationRegion,
				PostalCode:  row.DestinationPostalCode,
			},
			ContactPhone:  row.ContactPhone,
			PurchaseType:  row.PurchaseType,
			Company:       row.CompanyName,
			BusinessIDs:   decodeBusinessIDs(row.BusinessIds),
			PickupContact: decodePickupContact(row.PickupContact),
			Deliveries:    decodeDeliveries(row.Deliveries),
			SplitDelivery: row.SplitDelivery,
		},
		CheckoutVersion: row.CheckoutVersion,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if row.Kind == KindOffer {
		c.Offer = &Offer{
			ExpiresAt:      row.OfferExpiresAt.Time,
			Message:        row.OfferMessage,
			ShowInAccount:  row.OfferShowInAccount,
			Locks:          row.OfferLocks,
			PaymentMethods: row.OfferPaymentMethods,
			LinkVersion:    row.OfferLinkVersion,
			LinkToken:      row.OfferLinkToken,
			SentAt:         sqlnull.TimePtr(row.OfferSentAt),
			SendCount:      row.OfferSendCount,
			OpenedAt:       sqlnull.TimePtr(row.OfferOpenedAt),
			RevokedAt:      sqlnull.TimePtr(row.OfferRevokedAt),
			CreatedBy:      sqlnull.UUIDPtr(row.OfferCreatedBy),
			OrderID:        sqlnull.UUIDPtr(row.OrderID),
		}
	}
	return c
}

func encodeAddress(a *Address) (pqtype.NullRawMessage, error) {
	if a == nil {
		return pqtype.NullRawMessage{}, nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return pqtype.NullRawMessage{}, fmt.Errorf("encode address: %w", err)
	}
	return sqlnull.Raw(b), nil
}

func decodeBusinessIDs(raw json.RawMessage) map[string]string {
	var ids map[string]string
	if json.Unmarshal(raw, &ids) != nil || len(ids) == 0 {
		return nil
	}
	return ids
}

func decodeDeliveries(raw json.RawMessage) map[string]DeliverySelection {
	var d map[string]DeliverySelection
	if json.Unmarshal(raw, &d) != nil || len(d) == 0 {
		return nil
	}
	return d
}

func decodePickupContact(n pqtype.NullRawMessage) *PickupContact {
	if !n.Valid {
		return nil
	}
	var p PickupContact
	if json.Unmarshal(n.RawMessage, &p) != nil {
		return nil
	}
	return &p
}

// Only SetCheckout writes these columns, so an undecodable one reads as unset and the shopper re-enters it rather than the cart failing to load
func decodeAddress(n pqtype.NullRawMessage) *Address {
	if !n.Valid {
		return nil
	}
	var a Address
	if json.Unmarshal(n.RawMessage, &a) != nil {
		return nil
	}
	return &a
}

func toCartLine(row db.CartLine) CartLine {
	return CartLine{
		ID:            row.ID,
		CartID:        row.CartID,
		ProductName:   row.ProductName,
		ProductSku:    row.ProductSku,
		UnitPrice:     row.UnitPrice,
		Quantity:      row.Quantity,
		PriceOverride: row.PriceOverride,
		IsCustom:      row.IsCustom,
		TaxClass:      row.TaxClass,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
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
	o.SourceProductID = sqlnull.UUIDPtr(row.SourceProductID)
	return o
}

// IsSQLNoRows spots "no rows" through the %w chains every repo method wraps
func IsSQLNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
