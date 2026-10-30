package order

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// Split delivery at placement - one ShipmentInput per package the checkout planned (a delivery group's lines leaving one origin). Each is re-priced on its own at its own destination and the order's shipping is their sum, the order keeping one row per package. Order doesn't know what a delivery kind or a point is: the point snapshot arrives as JSON and is stored as given

const (
	FulfillmentNone     = "none"
	FulfillmentPhysical = "physical"
	FulfillmentMixed    = "mixed"
)

type ShipmentLine struct {
	CartLineID uuid.UUID `json:"cartLineId"`
	SKU        string    `json:"sku"`
	Quantity   float64   `json:"quantity"`
}

type ShipmentInput struct {
	GroupKey           string
	OriginLocationID   *uuid.UUID
	DeliveryKind       string
	DeliveryOptionID   *uuid.UUID
	DeliveryOptionName string
	CarrierSlug        string
	MethodCode         string
	MethodName         string
	EstimatedDays      *int
	// Where the package goes - the shopper's address or the point's. Rated at this destination
	ShipTo *OrderAddress
	Point  json.RawMessage
	Lines  []ShipmentLine
}

// Shipment is a placed package as the order records it
type Shipment struct {
	ID                 uuid.UUID
	OrderID            uuid.UUID
	Position           int32
	GroupKey           string
	OriginLocationID   *uuid.UUID
	DeliveryKind       string
	DeliveryOptionID   *uuid.UUID
	DeliveryOptionName string
	CarrierSlug        string
	MethodCode         string
	MethodName         string
	AmountCents        int64
	EstimatedDays      *int
	ShipTo             *OrderAddress
	Point              json.RawMessage
	Lines              []ShipmentLine
}

// ListShipments is the order's packages in placement order - empty for an order placed before split delivery, or a digital one
func (s *Service) ListShipments(ctx context.Context, orderID uuid.UUID) ([]Shipment, error) {
	return s.repo.ListShipments(ctx, orderID)
}

// priceShipments re-prices every package at its own destination with the chosen carrier method - fail-closed like a single shipment, so a vanished method refuses placement and the shopper picks again
func (s *Service) priceShipments(ctx context.Context, cc *checkoutContext, shipments []ShipmentInput, discount int64) ([]int64, error) {
	byID := make(map[uuid.UUID]int, len(cc.cart.Lines))
	for i, l := range cc.cart.Lines {
		byID[l.ID] = i
	}
	// A "free above" threshold is judged on the whole group - splitting it across warehouses mustn't cost the shopper free shipping
	groupSub := map[string]int64{}
	for _, sh := range shipments {
		for _, sl := range sh.Lines {
			j, ok := byID[sl.CartLineID]
			if !ok {
				return nil, fmt.Errorf("shipment %s: line %s is not in the cart", sh.GroupKey, sl.CartLineID)
			}
			groupSub[sh.GroupKey] += money.LineTotal(cc.cart.Lines[j].UnitPrice, cc.cart.Lines[j].Quantity)
		}
	}
	out := make([]int64, len(shipments))
	for i, sh := range shipments {
		req := shippingpkg.RateRequest{ChannelID: cc.store.ID, CurrencyCode: cc.store.CurrencyCode}
		for _, sl := range sh.Lines {
			l := cc.cart.Lines[byID[sl.CartLineID]]
			req.Items = append(req.Items, shippingpkg.LineItems(shippingpkg.RateItem{SKU: l.ProductSku, Quantity: l.Quantity, UnitPriceCents: l.UnitPrice}, l.Quantity, cartLineParts(l))...)
		}
		req.SubtotalCents = groupSubtotal(groupSub[sh.GroupKey], cc.subtotal, discount)
		if sh.ShipTo != nil {
			req.Destination = shippingpkg.Address{
				Street1: sh.ShipTo.Street1, City: sh.ShipTo.City, PostalCode: sh.ShipTo.PostalCode,
				CountryCode: sh.ShipTo.CountryCode, Region: derefString(sh.ShipTo.State),
			}
		}
		cents, _, err := s.shipping.PriceSelected(ctx, sh.CarrierSlug, sh.MethodCode, req)
		if err != nil {
			return nil, fmt.Errorf("price shipment %s: %w", sh.GroupKey, err)
		}
		out[i] = cents
	}
	return out, nil
}

// groupSubtotal is a group's share of the order-level discount taken off its value - the order's discount is shared across groups by value. The checkout's quotes compute the same, so the price shown is the price charged
func groupSubtotal(sub, orderSubtotal, discount int64) int64 {
	if orderSubtotal > 0 {
		sub -= discount * sub / orderSubtotal
	}
	return max(sub, 0)
}

// selectShipments prices a split delivery, or a digital order's none - the caller only reaches it with shipments or with nothing to ship
func (s *Service) selectShipments(ctx context.Context, in PlaceOrderInput, cc *checkoutContext, discount int64, ranEngine bool, eval promotion.EvalResult) (shippingSelection, []int64, error) {
	if len(in.Shipments) == 0 || s.shipping == nil {
		return shippingSelection{}, nil, nil
	}
	var freeMethods []string
	var override *int64
	free := ranEngine && eval.Ledger.FreeShipping()
	if free {
		freeMethods = eval.Ledger.FreeShippingMethods()
	}
	if ranEngine {
		override = eval.Ledger.ShippingOverride()
	}
	return s.resolveShipmentCents(ctx, in, cc, discount, free, freeMethods, override)
}

// resolveShipmentCents is resolveShippingCents for a split delivery - promotions apply to the whole order's shipping as they do to one package, then each package carries its share
func (s *Service) resolveShipmentCents(ctx context.Context, in PlaceOrderInput, cc *checkoutContext, discount int64, freeShipping bool, freeMethods []string, override *int64) (shippingSelection, []int64, error) {
	amounts, err := s.priceShipments(ctx, cc, in.Shipments, discount)
	if err != nil {
		return shippingSelection{}, nil, err
	}
	applyShippingPromos(amounts, in.Shipments, freeShipping, freeMethods, override)
	var total int64
	for _, a := range amounts {
		total += a
	}
	first := in.Shipments[0]
	return shippingSelection{Cents: total, CarrierSlug: first.CarrierSlug, MethodCode: first.MethodCode}, amounts, nil
}

// singlePackage - the order has at most one package, so its one carrier and method can re-price it on an edit
func (s *Service) singlePackage(ctx context.Context, orderID uuid.UUID) (bool, error) {
	n, err := s.repo.CountShipments(ctx, orderID)
	return n <= 1, err
}

// applyShippingPromos waives the packages a free-shipping promotion covers - every one, or those of its named methods - and an override replaces the whole order's shipping, carried by the first package
func applyShippingPromos(amounts []int64, shipments []ShipmentInput, freeShipping bool, freeMethods []string, override *int64) {
	if freeShipping {
		for i, sh := range shipments {
			if len(freeMethods) == 0 || slices.Contains(freeMethods, sh.MethodCode) {
				amounts[i] = 0
			}
		}
	}
	if override != nil && len(amounts) > 0 {
		for i := range amounts {
			amounts[i] = 0
		}
		amounts[0] = *override
	}
}

func (s *Service) persistShipments(ctx context.Context, r Repository, orderID uuid.UUID, shipments []ShipmentInput, amounts []int64) error {
	for i, sh := range shipments {
		var amount int64
		if i < len(amounts) {
			amount = amounts[i]
		}
		if err := r.InsertShipment(ctx, orderID, int32(i), sh, amount); err != nil {
			return err
		}
	}
	return nil
}

func (r *postgresRepository) InsertShipment(ctx context.Context, orderID uuid.UUID, position int32, sh ShipmentInput, amount int64) error {
	lines, err := json.Marshal(nonNilShipmentLines(sh.Lines))
	if err != nil {
		return fmt.Errorf("encode shipment lines: %w", err)
	}
	var shipTo []byte
	if sh.ShipTo != nil {
		if shipTo, err = json.Marshal(sh.ShipTo); err != nil {
			return fmt.Errorf("encode shipment address: %w", err)
		}
	}
	var days sql.NullInt32
	if sh.EstimatedDays != nil {
		days = sql.NullInt32{Int32: int32(*sh.EstimatedDays), Valid: true}
	}
	_, err = r.queries.InsertOrderShipment(ctx, db.InsertOrderShipmentParams{
		OrderID: orderID, Position: position, GroupKey: sh.GroupKey,
		OriginLocationID: sqlnull.UUID(sh.OriginLocationID), DeliveryKind: sh.DeliveryKind,
		DeliveryOptionID: sqlnull.UUID(sh.DeliveryOptionID), DeliveryOptionName: sh.DeliveryOptionName,
		CarrierSlug: sh.CarrierSlug, MethodCode: sh.MethodCode, MethodName: sh.MethodName, AmountCents: amount,
		EstimatedDays: days, ShipTo: rawOrNull(shipTo), Point: rawOrNull(sh.Point), Lines: lines,
	})
	if err != nil {
		return fmt.Errorf("insert order shipment: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListShipments(ctx context.Context, orderID uuid.UUID) ([]Shipment, error) {
	rows, err := r.queries.ListOrderShipments(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list order shipments: %w", err)
	}
	out := make([]Shipment, 0, len(rows))
	for _, row := range rows {
		sh := Shipment{
			ID: row.ID, OrderID: row.OrderID, Position: row.Position, GroupKey: row.GroupKey,
			OriginLocationID: sqlnull.UUIDPtr(row.OriginLocationID), DeliveryKind: row.DeliveryKind,
			DeliveryOptionID: sqlnull.UUIDPtr(row.DeliveryOptionID), DeliveryOptionName: row.DeliveryOptionName,
			CarrierSlug: row.CarrierSlug, MethodCode: row.MethodCode, MethodName: row.MethodName, AmountCents: row.AmountCents,
		}
		if row.EstimatedDays.Valid {
			d := int(row.EstimatedDays.Int32)
			sh.EstimatedDays = &d
		}
		if row.ShipTo.Valid {
			var a OrderAddress
			if json.Unmarshal(row.ShipTo.RawMessage, &a) == nil {
				sh.ShipTo = &a
			}
		}
		if row.Point.Valid {
			sh.Point = row.Point.RawMessage
		}
		_ = json.Unmarshal(row.Lines, &sh.Lines)
		out = append(out, sh)
	}
	return out, nil
}

func (r *postgresRepository) SetShipmentAmount(ctx context.Context, orderID uuid.UUID, amount int64) error {
	return r.queries.SetOrderShipmentAmount(ctx, db.SetOrderShipmentAmountParams{OrderID: orderID, AmountCents: amount})
}

func (r *postgresRepository) CountShipments(ctx context.Context, orderID uuid.UUID) (int64, error) {
	return r.queries.CountOrderShipments(ctx, orderID)
}

func (r *postgresRepository) SetShipmentLines(ctx context.Context, shipmentID uuid.UUID, lines []ShipmentLine) error {
	b, err := json.Marshal(nonNilShipmentLines(lines))
	if err != nil {
		return fmt.Errorf("encode shipment lines: %w", err)
	}
	return r.queries.SetOrderShipmentLines(ctx, db.SetOrderShipmentLinesParams{ID: shipmentID, Lines: b})
}

// editShipmentLines keeps each package's lines in step with a line edit - a removed line leaves its package, a new quantity follows, and an added line joins the first package
func editShipmentLines(ctx context.Context, r Repository, orderID uuid.UUID, removed map[uuid.UUID]bool, qty map[uuid.UUID]float64, added []OrderLine) error {
	ships, err := r.ListShipments(ctx, orderID)
	if err != nil || len(ships) == 0 {
		return err
	}
	for i, sh := range ships {
		lines := slices.DeleteFunc(sh.Lines, func(l ShipmentLine) bool { return removed[l.CartLineID] })
		for j := range lines {
			if q, ok := qty[lines[j].CartLineID]; ok {
				lines[j].Quantity = q
			}
		}
		if i == 0 {
			for _, l := range added {
				lines = append(lines, ShipmentLine{CartLineID: l.ID, SKU: l.ProductSku, Quantity: l.Quantity})
			}
		}
		if err := r.SetShipmentLines(ctx, sh.ID, lines); err != nil {
			return err
		}
	}
	return nil
}

func (r *postgresRepository) SetShipmentShipTo(ctx context.Context, orderID uuid.UUID, shipTo OrderAddress) error {
	b, err := json.Marshal(shipTo)
	if err != nil {
		return fmt.Errorf("encode shipment address: %w", err)
	}
	return r.queries.SetOrderShipmentShipTo(ctx, db.SetOrderShipmentShipToParams{OrderID: orderID, ShipTo: rawOrNull(b)})
}

func (r *postgresRepository) SetFulfillmentNeed(ctx context.Context, orderID uuid.UUID, need string) error {
	if need == "" {
		need = FulfillmentPhysical
	}
	return r.queries.SetOrderFulfillmentNeed(ctx, db.SetOrderFulfillmentNeedParams{ID: orderID, FulfillmentNeed: need})
}

func nonNilShipmentLines(l []ShipmentLine) []ShipmentLine {
	if l == nil {
		return []ShipmentLine{}
	}
	return l
}

func rawOrNull(b []byte) pqtype.NullRawMessage {
	if len(b) == 0 {
		return pqtype.NullRawMessage{}
	}
	return sqlnull.Raw(b)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
