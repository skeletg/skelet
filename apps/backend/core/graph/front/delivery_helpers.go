package front

// The checkout as delivery options shape it. A cart is planned into delivery groups (lines that take one delivery choice) and each group into shipments (its lines leaving one origin); every group is offered the channel's options of the kinds its lines allow, priced per shipment and summed. The same view feeds the storefront's checkout, the requirements placeOrder enforces and the placement request, so they can't disagree

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	frontmodel "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// OriginPlanner says where each physical line would ship from
type OriginPlanner interface {
	PlanOrigins(ctx context.Context, channelID uuid.UUID, lines []delivery.Line, profiles map[string]delivery.LineProfile) (map[uuid.UUID]delivery.Origin, error)
}

var (
	errDeliveryGroupUnknown = apperr.New("CHECKOUT_DELIVERY_GROUP_UNKNOWN", apperr.ClassValidation, "those items are no longer grouped that way - choose the delivery again")
	errDeliveryChoice       = apperr.New("SHIPPING_METHOD_UNAVAILABLE", apperr.ClassValidation, "that delivery isn't available for these items here")
	errDeliveryNoPoint      = apperr.New("CHECKOUT_DELIVERY_POINT_UNSUPPORTED", apperr.ClassValidation, "this delivery doesn't take a pickup point")
	errDeliveryChooseFirst  = apperr.New("CHECKOUT_DELIVERY_OPTION_REQUIRED", apperr.ClassValidation, "choose how to deliver these items first")
)

type checkoutView struct {
	c        cart.Cart
	options  []delivery.Option
	profiles map[string]delivery.LineProfile
	plan     delivery.Plan
	groups   []*groupView
	// For sharing the cart-level discount across shipments the way placement does
	subtotal, discount int64
	// Planning the cart split would give more than one group
	splittable bool
}

type groupView struct {
	delivery.Group
	shipments []delivery.Shipment
	choices   []deliveryChoice
	sel       cart.DeliverySelection
	hasSel    bool
	option    *delivery.Option // the selection's option, while it's still offered for this group
	kind      *delivery.Kind
	chosen    *deliveryChoice // the selection's choice, while it's still quoted
}

type deliveryChoice struct {
	option        delivery.Option
	kind          delivery.Kind
	carrierSlug   string
	carrierName   string
	methodCode    string
	methodName    string
	priceCents    int64
	estimatedDays int
	// ISO date, empty when the carrier gives no estimate
	estimatedArrival string
	perShipment      []int64
}

func (v *checkoutView) option(id uuid.UUID) *delivery.Option {
	for i := range v.options {
		if v.options[i].ID == id {
			return &v.options[i]
		}
	}
	return nil
}

func (v *checkoutView) group(key string) *groupView {
	for _, g := range v.groups {
		if g.Key == key {
			return g
		}
	}
	return nil
}

// chosenGroups are the groups whose delivery is chosen and still offered
func (v *checkoutView) chosenGroups() []*groupView {
	var out []*groupView
	for _, g := range v.groups {
		if g.chosen != nil {
			out = append(out, g)
		}
	}
	return out
}

// shipsToShopper - some chosen delivery goes to the shopper's own address
func (v *checkoutView) shipsToShopper() bool {
	for _, g := range v.groups {
		if g.kind != nil && g.kind.ShipTo == delivery.ShipToCustomer {
			return true
		}
	}
	return false
}

// needs reports whether any chosen delivery asks for the block at all
func (v *checkoutView) needs(block string) bool {
	for _, g := range v.groups {
		if g.kind != nil && g.option != nil && g.kind.ModeOf(g.option.BlockModes, block) != delivery.ModeHidden {
			return true
		}
	}
	return false
}

// offeredOptions are the channel's delivery options - the implicit defaults while the admin has built none, whose pickup asks billing as sales/checkout/pickup_requires_billing_address says
func (r *Resolver) offeredOptions(ctx context.Context, channelID uuid.UUID) []delivery.Option {
	opts := delivery.DefaultOptions(channelID)
	if r.DeliverySvc != nil {
		var err error
		if opts, err = r.DeliverySvc.OfferedOptions(ctx, channelID); err != nil {
			return nil
		}
	}
	for i, o := range opts {
		if o.Kind == delivery.KindStorePickup && delivery.IsDefaultOption(channelID, o.ID) && r.checkoutSettings(ctx, channelID).PickupNeedsBilling {
			opts[i].BlockModes = map[string]delivery.Mode{delivery.BlockBillingAddress: delivery.ModeRequired}
		}
	}
	return opts
}

// checkoutView plans the cart - read on every checkout response, so it quotes each shipment once and nothing else
func (r *Resolver) checkoutView(ctx context.Context, c cart.Cart) *checkoutView {
	v := &checkoutView{c: c, options: r.offeredOptions(ctx, c.ChannelID), profiles: map[string]delivery.LineProfile{}}
	lines := cart.DeliveryLines(c.Lines)
	skus := make([]string, 0, len(c.Lines))
	for _, l := range c.Lines {
		skus = append(skus, l.ProductSku)
	}
	if r.DeliverySvc != nil {
		if p, err := r.DeliverySvc.LineProfiles(ctx, skus); err == nil {
			v.profiles = p
		}
	}
	v.profiles = cart.DeliveryProfiles(c.Lines, v.profiles)
	co := c.Checkout
	v.plan = delivery.PlanGroups(lines, v.profiles, delivery.OfferedKinds(v.options), co.SplitDelivery)
	v.splittable = len(delivery.PlanGroups(lines, v.profiles, delivery.OfferedKinds(v.options), true).Groups) > 1
	if len(v.plan.Groups) == 0 {
		return v
	}
	var physical []delivery.Line
	for _, g := range v.plan.Groups {
		physical = append(physical, g.Lines...)
	}
	var origins map[uuid.UUID]delivery.Origin
	if r.OriginPlanner != nil {
		origins, _ = r.OriginPlanner.PlanOrigins(ctx, c.ChannelID, physical, v.profiles)
	}
	sv, ok := channel.FromContext(ctx)
	if ok && sv != nil && !co.QuoteDestination().IsZero() {
		_, m := r.cartRateRequest(ctx, sv, c, co.QuoteDestination().CountryCode, co.QuoteDestination().Region, co.QuoteDestination().PostalCode)
		v.subtotal, v.discount = int64(m.Subtotal), int64(m.TotalDiscount)
	}
	for _, g := range v.plan.Groups {
		gv := &groupView{Group: g, shipments: delivery.SplitShipments([]delivery.Group{g}, origins)}
		gv.sel, gv.hasSel = co.Delivery(g.Key)
		if gv.hasSel {
			if o := v.option(gv.sel.OptionID); o != nil && slices.Contains(g.Kinds, o.Kind) {
				if k, ok := delivery.KindBySlug(o.Kind); ok {
					gv.option, gv.kind = o, &k
				}
			}
		}
		if ok && sv != nil {
			gv.choices = r.groupChoices(ctx, sv, v, gv)
		}
		if gv.option != nil {
			for i := range gv.choices {
				ch := &gv.choices[i]
				if ch.option.ID == gv.option.ID && ch.carrierSlug == gv.sel.CarrierSlug && ch.methodCode == gv.sel.MethodCode {
					gv.chosen = ch
					break
				}
			}
		}
		v.groups = append(v.groups, gv)
	}
	return v
}

// groupDestination is where a group is rated - the chosen point once there is one, else the shopper's destination
func groupDestination(co cart.Checkout, gv *groupView) shipping.Address {
	if gv.kind != nil && gv.kind.ShipTo == delivery.ShipToPoint && gv.sel.Point != nil {
		a := gv.sel.Point.Address
		return shipping.Address{Street1: a.Street1, Street2: a.Street2, City: a.City, Region: a.Region, PostalCode: a.PostalCode, CountryCode: a.CountryCode}
	}
	if a := co.ShippingAddress; a != nil {
		return shipping.Address{Street1: a.Street1, City: a.City, Region: ptrOr(a.State), PostalCode: a.PostalCode, CountryCode: a.CountryCode}
	}
	d := co.Destination
	return shipping.Address{CountryCode: d.CountryCode, Region: d.Region, PostalCode: d.PostalCode}
}

// groupChoices quotes every shipment of the group and offers each option a method every shipment can take, of the option's kind and carrier, priced as the sum
func (r *Resolver) groupChoices(ctx context.Context, sv *channel.Channel, v *checkoutView, gv *groupView) []deliveryChoice {
	dest := groupDestination(v.c.Checkout, gv)
	if dest.CountryCode == "" || r.ShippingSvc == nil {
		return nil
	}
	perShipment := make([][]shipping.RatedQuote, 0, len(gv.shipments))
	for _, sh := range gv.shipments {
		req := shipping.RateRequest{ChannelID: sv.ID, Destination: dest, CurrencyCode: sv.CurrencyCode}
		var sub int64
		for _, l := range sh.Lines {
			req.Items = append(req.Items, shipping.RateItem{SKU: l.SKU, Quantity: l.Quantity, UnitPriceCents: l.UnitPrice})
			sub += int64(float64(l.UnitPrice) * l.Quantity)
		}
		if v.subtotal > 0 {
			sub -= v.discount * sub / v.subtotal
		}
		req.SubtotalCents = max(sub, 0)
		quotes, err := r.ShippingSvc.QuoteAll(ctx, req)
		if err != nil {
			return nil
		}
		perShipment = append(perShipment, quotes)
	}
	return matchChoices(v.options, gv.Kinds, perShipment, r.storeToday(ctx, sv.ID))
}

// matchChoices offers each option, in order, every method all shipments were quoted that is of the option's kind - and of its carrier, when it names one. A choice costs the sum of its shipments and arrives with the slowest, counted from today
func matchChoices(options []delivery.Option, kinds []string, perShipment [][]shipping.RatedQuote, today time.Time) []deliveryChoice {
	type key struct{ carrier, code string }
	type rated struct {
		q      shipping.RatedQuote
		prices []int64
		days   int
	}
	var avail map[key]*rated
	var order []key
	for si, quotes := range perShipment {
		if si == 0 {
			avail = map[key]*rated{}
			for _, q := range quotes {
				k := key{q.CarrierSlug, q.Method.Code}
				if _, dup := avail[k]; dup {
					continue
				}
				avail[k] = &rated{q: q, prices: []int64{q.Method.PriceCents}, days: q.Method.EstimatedDays}
				order = append(order, k)
			}
			continue
		}
		seen := map[key]shipping.RatedQuote{}
		for _, q := range quotes {
			seen[key{q.CarrierSlug, q.Method.Code}] = q
		}
		for k, a := range avail {
			q, ok := seen[k]
			if !ok {
				delete(avail, k)
				continue
			}
			a.prices = append(a.prices, q.Method.PriceCents)
			a.days = max(a.days, q.Method.EstimatedDays)
		}
	}
	var out []deliveryChoice
	for _, o := range options {
		if !slices.Contains(kinds, o.Kind) {
			continue
		}
		k, ok := delivery.KindBySlug(o.Kind)
		if !ok {
			continue
		}
		for _, mk := range order {
			a, ok := avail[mk]
			if !ok || a.q.Method.Kind != o.Kind || (o.CarrierSlug != "" && o.CarrierSlug != a.q.CarrierSlug) {
				continue
			}
			var total int64
			for _, p := range a.prices {
				total += p
			}
			out = append(out, deliveryChoice{
				option: o, kind: k, carrierSlug: a.q.CarrierSlug, carrierName: a.q.CarrierName,
				methodCode: a.q.Method.Code, methodName: a.q.Method.Name, priceCents: total, perShipment: a.prices,
				estimatedDays: a.days, estimatedArrival: shipping.ArrivalDate(today, a.days, a.q.Method.DeliversOnWeekends),
			})
		}
	}
	return out
}

// storeToday is now on the store's calendar - the day delivery estimates count from
func (r *Resolver) storeToday(ctx context.Context, channelID uuid.UUID) time.Time {
	now := time.Now()
	if r.ConfigSvc == nil {
		return now.UTC()
	}
	tz, err := r.ConfigSvc.GetString(ctx, "general/store_information/timezone", config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID})
	if err != nil || tz == "" {
		return now.UTC()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return now.UTC()
	}
	return now.In(loc)
}

// ---- requirements ----

func (r *Resolver) checkoutSnapshot(ctx context.Context, v *checkoutView) delivery.Snapshot {
	c := v.c
	co := c.Checkout
	set := r.checkoutSettings(ctx, c.ChannelID)
	billing, _ := co.EffectiveBilling()
	s := delivery.Snapshot{
		Plan:              v.plan,
		ChannelPhoneMode:  delivery.Mode(set.Phone),
		Email:             r.checkoutEmail(ctx, c) != "",
		Phone:             co.ContactPhone != "",
		ShippingAddress:   co.ShippingAddress != nil,
		Billing:           billing != nil,
		BillingMirrorable: v.shipsToShopper(),
		Payment:           co.PaymentProviderSlug != "" && r.paymentMethodOffered(ctx, c.ChannelID, co.PaymentProviderSlug, paymentCountry(co)),
	}
	if co.PickupContact != nil && co.PickupContact.SomeoneElse {
		s.BillingFloor = delivery.ModeRequired
	}
	for _, g := range v.groups {
		gs := delivery.GroupSnapshot{Group: g.Group, Choices: len(g.choices)}
		if g.option != nil {
			gs.Option, gs.Kind = g.option, g.kind
			gs.MethodChosen = g.chosen != nil
			gs.Point = g.sel.Point != nil
			gs.Collector = co.PickupContact != nil
		}
		s.Groups = append(s.Groups, gs)
	}
	return s
}

// paymentCountry is where payment eligibility is judged - the destination, else the billing address of a digital order
func paymentCountry(co cart.Checkout) string {
	if c := co.ShippingCountry(); c != "" {
		return c
	}
	if b, _ := co.EffectiveBilling(); b != nil {
		return b.CountryCode
	}
	return ""
}

// ---- placement ----

// placementShipments turns the chosen groups into the order's packages - each carries its own destination (the shopper's address or the point's, addressed to whoever collects), and the primary destination is what tax and the order's shipping address follow
func (r *Resolver) placementShipments(ctx context.Context, v *checkoutView) ([]order.ShipmentInput, *order.OrderAddress) {
	co := v.c.Checkout
	var (
		out     []order.ShipmentInput
		primary *order.OrderAddress
	)
	var customer *order.OrderAddress
	switch {
	case co.ShippingAddress != nil:
		a := cartAddressToOrderAddress(*co.ShippingAddress)
		customer = &a
	case !co.Destination.IsZero():
		// Only the destination so far - enough to quote, and placement refuses it through the requirements
		d := co.Destination
		customer = &order.OrderAddress{CountryCode: d.CountryCode, PostalCode: d.PostalCode, State: nilIfEmpty(d.Region)}
	}
	for _, g := range v.chosenGroups() {
		ch := g.chosen
		shipTo := customer
		var point []byte
		if g.kind.ShipTo == delivery.ShipToPoint && g.sel.Point != nil {
			shipTo = r.pointOrderAddress(co, *g.sel.Point)
			point, _ = json.Marshal(g.sel.Point)
		}
		if primary == nil && shipTo != nil && shipTo != customer {
			primary = shipTo
		}
		optID := ch.option.ID
		for i, sh := range g.shipments {
			in := order.ShipmentInput{
				GroupKey: g.Key, OriginLocationID: sh.Origin.LocationID, DeliveryKind: ch.option.Kind,
				DeliveryOptionName: ch.option.Name, CarrierSlug: ch.carrierSlug, MethodCode: ch.methodCode, MethodName: ch.methodName,
				ShipTo: shipTo, Point: point,
			}
			if r.optionPersisted(ctx, v.c.ChannelID, optID) {
				in.DeliveryOptionID = &optID
			}
			if i < len(ch.perShipment) && ch.estimatedDays > 0 {
				d := ch.estimatedDays
				in.EstimatedDays = &d
			}
			for _, l := range sh.Lines {
				in.Lines = append(in.Lines, order.ShipmentLine{CartLineID: l.ID, SKU: l.SKU, Quantity: l.Quantity})
			}
			out = append(out, in)
		}
	}
	// The shopper's own address leads whenever any package goes there - tax and the order's shipping address follow it
	if customer != nil && (v.shipsToShopper() || primary == nil) {
		primary = customer
	}
	return out, primary
}

// optionPersisted - the implicit default options have no row for an order to point at
func (r *Resolver) optionPersisted(ctx context.Context, channelID, id uuid.UUID) bool {
	if r.DeliverySvc == nil {
		return false
	}
	o, err := r.DeliverySvc.GetOption(ctx, id)
	return err == nil && o.ChannelID == channelID
}

// pointOrderAddress ships to the point, addressed to whoever collects - the pickup contact, else the billing name
func (r *Resolver) pointOrderAddress(co cart.Checkout, p delivery.Point) *order.OrderAddress {
	a := order.OrderAddress{
		Company: nilIfEmpty(p.Name), Street1: p.Address.Street1, Street2: nilIfEmpty(p.Address.Street2), City: p.Address.City,
		State: nilIfEmpty(p.Address.Region), PostalCode: p.Address.PostalCode, CountryCode: p.Address.CountryCode,
	}
	switch b, _ := co.EffectiveBilling(); {
	case co.PickupContact != nil:
		a.FirstName, a.LastName = co.PickupContact.FirstName, co.PickupContact.LastName
	case b != nil:
		a.FirstName, a.LastName = b.FirstName, b.LastName
	}
	if co.ContactPhone != "" {
		a.Phone = &co.ContactPhone
	}
	return &a
}

// ---- models ----

func (r *Resolver) deliveryGroupModels(v *checkoutView) []*frontmodel.DeliveryGroup {
	out := make([]*frontmodel.DeliveryGroup, 0, len(v.groups))
	for _, g := range v.groups {
		m := &frontmodel.DeliveryGroup{Key: g.Key, Kinds: g.Kinds, LineIds: lineIDs(g.Lines), Choices: []*frontmodel.DeliveryChoice{}, Shipments: []*frontmodel.DeliveryShipment{}}
		for _, ch := range g.choices {
			m.Choices = append(m.Choices, toDeliveryChoiceModel(ch))
		}
		for i, sh := range g.shipments {
			sm := &frontmodel.DeliveryShipment{LineIds: lineIDs(sh.Lines)}
			if sh.Origin.LocationID != nil {
				sm.OriginName, sm.OriginCity = nilIfEmpty(sh.Origin.Name), nilIfEmpty(sh.Origin.City)
			}
			if g.chosen != nil && i < len(g.chosen.perShipment) {
				p := int32(g.chosen.perShipment[i])
				sm.PriceCents = &p
				if g.chosen.estimatedDays > 0 {
					d := int32(g.chosen.estimatedDays)
					sm.EstimatedDays = &d
				}
				sm.EstimatedArrival = nilIfEmpty(g.chosen.estimatedArrival)
			}
			m.Shipments = append(m.Shipments, sm)
		}
		if g.chosen != nil {
			m.Selection = toDeliverySelectionModel(g)
		}
		out = append(out, m)
	}
	return out
}

func toDeliveryChoiceModel(ch deliveryChoice) *frontmodel.DeliveryChoice {
	m := &frontmodel.DeliveryChoice{
		OptionID: ch.option.ID.String(), OptionName: ch.option.Name, Description: ch.option.Description,
		Kind: ch.kind.Slug, KindLabel: ch.kind.Label, ShipTo: shipToModel(ch.kind.ShipTo), PointLabel: nilIfEmpty(ch.kind.PointLabel),
		CarrierSlug: ch.carrierSlug, CarrierName: ch.carrierName, MethodCode: ch.methodCode, MethodName: ch.methodName,
		PriceCents: int32(ch.priceCents),
	}
	if ch.estimatedDays > 0 {
		d := int32(ch.estimatedDays)
		m.EstimatedDays = &d
	}
	m.EstimatedArrival = nilIfEmpty(ch.estimatedArrival)
	return m
}

func toDeliverySelectionModel(g *groupView) *frontmodel.DeliverySelection {
	ch := g.chosen
	m := &frontmodel.DeliverySelection{
		OptionID: ch.option.ID.String(), OptionName: ch.option.Name, Kind: ch.kind.Slug, ShipTo: shipToModel(ch.kind.ShipTo),
		PointLabel: nilIfEmpty(ch.kind.PointLabel), CarrierSlug: ch.carrierSlug, MethodCode: ch.methodCode, MethodName: ch.methodName,
		PriceCents: int32(ch.priceCents),
	}
	if ch.estimatedDays > 0 {
		d := int32(ch.estimatedDays)
		m.EstimatedDays = &d
	}
	m.EstimatedArrival = nilIfEmpty(ch.estimatedArrival)
	if p := g.sel.Point; p != nil {
		m.Point = toDeliveryPointModel(*p)
	}
	return m
}

func toDeliveryPointModel(p delivery.Point) *frontmodel.DeliveryPoint {
	a := p.Address
	return &frontmodel.DeliveryPoint{
		ID: p.ID, Name: p.Name, Street1: a.Street1, Street2: nilIfEmpty(a.Street2), City: a.City, Region: nilIfEmpty(a.Region),
		PostalCode: a.PostalCode, CountryCode: a.CountryCode, Lat: p.Lat, Lng: p.Lng, DistanceKm: p.DistanceKm,
		Hours: nilIfEmpty(p.Hours), Instructions: nilIfEmpty(p.Instructions), Phone: nilIfEmpty(p.Phone),
	}
}

func toRequirementModels(reqs []delivery.Requirement) []*frontmodel.CheckoutRequirement {
	out := make([]*frontmodel.CheckoutRequirement, 0, len(reqs))
	for _, q := range reqs {
		m := &frontmodel.CheckoutRequirement{
			Scope: q.Scope, Type: q.Type, Label: q.Label, Code: q.Code, Message: q.Message,
			Mode: frontmodel.RequirementModeOptional, Status: frontmodel.RequirementStatus(strings.ToUpper(string(q.Status))),
		}
		if q.Mode == delivery.ModeRequired {
			m.Mode = frontmodel.RequirementModeRequired
		}
		out = append(out, m)
	}
	return out
}

func shipToModel(s delivery.ShipTo) frontmodel.DeliveryShipTo {
	if s == delivery.ShipToPoint {
		return frontmodel.DeliveryShipToPoint
	}
	return frontmodel.DeliveryShipToCustomer
}

func fulfillmentModel(n delivery.Need) frontmodel.FulfillmentNeed {
	switch n {
	case delivery.NeedNone:
		return frontmodel.FulfillmentNeedNone
	case delivery.NeedMixed:
		return frontmodel.FulfillmentNeedMixed
	}
	return frontmodel.FulfillmentNeedPhysical
}

func lineIDs(lines []delivery.Line) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.ID.String()
	}
	return out
}

func ptrOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---- step helpers ----

// selectDelivery stores a group's choice - a different option starts the group over. A kind whose point source has exactly one point gets it picked, so a one-store pickup asks nothing more
func (r *Resolver) selectDelivery(ctx context.Context, c cart.Cart, co *cart.Checkout, g *groupView, ch deliveryChoice) {
	sel := g.sel
	if !g.hasSel || sel.OptionID != ch.option.ID {
		sel = cart.DeliverySelection{}
	}
	sel.OptionID, sel.CarrierSlug, sel.MethodCode = ch.option.ID, ch.carrierSlug, ch.methodCode
	if sel.Point == nil && ch.kind.Needs(delivery.BlockPoint) && r.DeliverySvc != nil {
		if pts, err := r.DeliverySvc.SearchPoints(ctx, ch.option, delivery.PointQuery{Near: deliveryNear(c.Checkout), Items: deliveryItems(g.Lines), Limit: 2}); err == nil && len(pts) == 1 {
			sel.Point = &pts[0]
		}
	}
	co.SetDelivery(g.Key, sel)
}

// chosenGroup is a group whose delivery option is chosen - the steps that follow the choice need one
func (r *Resolver) chosenGroup(ctx context.Context, c cart.Cart, key string) (*groupView, error) {
	g := r.checkoutView(ctx, c).group(key)
	if g == nil {
		return nil, errDeliveryGroupUnknown
	}
	if g.option == nil || r.DeliverySvc == nil {
		return nil, errDeliveryChooseFirst
	}
	return g, nil
}

// storeCart is a cart the caller may read on the current store
func (r *Resolver) storeCart(ctx context.Context, cartID string) (cart.Cart, error) {
	cid, err := uuid.Parse(cartID)
	if err != nil {
		return cart.Cart{}, invalidID("cart")
	}
	if err := r.assertCartOnCurrentStore(ctx, cid); err != nil {
		return cart.Cart{}, err
	}
	return r.CartSvc.GetCart(ctx, cid, channel.OptionalCustomerID(ctx))
}

func deliveryNear(co cart.Checkout) delivery.Near {
	if a := co.ShippingAddress; a != nil {
		return delivery.Near{CountryCode: a.CountryCode, Region: ptrOr(a.State), PostalCode: a.PostalCode, City: a.City, Street1: a.Street1}
	}
	d := co.Destination
	return delivery.Near{CountryCode: d.CountryCode, Region: d.Region, PostalCode: d.PostalCode}
}

func deliveryItems(lines []delivery.Line) []delivery.Item {
	out := make([]delivery.Item, len(lines))
	for i, l := range lines {
		out[i] = delivery.Item{SKU: l.SKU, Quantity: l.Quantity}
	}
	return out
}
