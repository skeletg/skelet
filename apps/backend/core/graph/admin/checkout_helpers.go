package admin

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// requireCheckout gates a checkout settings read or write on the channel
func (r *Resolver) requireCheckout(ctx context.Context, channelID uuid.UUID, write bool) error {
	slug := "checkout.view"
	if write {
		slug = "checkout.manage"
	}
	return auth.RequireChannelPermission(ctx, r.AdminSvc, slug, channelID, nil)
}

// requireOptionScope gates a write on an option before loading it, then on the option's own channel
func (r *Resolver) requireOptionScope(ctx context.Context, id uuid.UUID) (delivery.Option, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "checkout.manage"); err != nil {
		return delivery.Option{}, err
	}
	o, err := r.DeliverySvc.GetOption(ctx, id)
	if err != nil {
		return delivery.Option{}, err
	}
	if err := r.requireCheckout(ctx, o.ChannelID, true); err != nil {
		return delivery.Option{}, err
	}
	return o, nil
}

// checkoutConfiguration is everything the Checkout settings page edits and checks, for one channel
func (r *Resolver) checkoutConfiguration(ctx context.Context, channelID uuid.UUID) (*model.CheckoutConfiguration, error) {
	opts, err := r.DeliverySvc.ListOptions(ctx, channelID)
	if err != nil {
		return nil, err
	}
	m := &model.CheckoutConfiguration{ChannelID: channelID.String(), UsingDefaults: len(opts) == 0}
	if m.UsingDefaults {
		opts = delivery.DefaultOptions(channelID)
	}
	for _, k := range delivery.Kinds() {
		m.Kinds = append(m.Kinds, toDeliveryKindModel(k))
	}
	readyEnabled := 0
	for _, o := range opts {
		om := r.deliveryOptionModel(ctx, o, m.UsingDefaults)
		if o.Enabled && om.Ready {
			readyEnabled++
		}
		m.Options = append(m.Options, om)
	}

	m.DigitalBlocks = toOptionBlocks(delivery.DigitalKind, nil)
	for _, s := range r.DeliverySvc.PointSources(ctx, channelID) {
		m.PointSources = append(m.PointSources, toDeliverySourceModel(s))
	}
	points, err := r.DeliverySvc.ListPoints(ctx, channelID)
	if err != nil {
		return nil, err
	}
	m.DeliveryPoints = toDeliveryPointModels(points)
	methods := r.enabledPaymentMethods(ctx, channelID)
	if kinds, err := r.ShippingSvc.EnabledCarrierKinds(ctx, channelID); err == nil {
		for _, k := range delivery.Kinds() {
			var names []string
			refs := []*model.CarrierRef{}
			for _, n := range kinds[k.Slug] {
				slug, label, _ := strings.Cut(n, ":")
				names = append(names, label)
				refs = append(refs, &model.CarrierRef{Slug: slug, Name: label})
			}
			m.CarrierKinds = append(m.CarrierKinds, &model.CarrierKind{Kind: k.Slug, Carriers: nonNilKinds(names), CarrierRefs: refs})
		}
	}

	// The go-live checklist - what has to be true for a shopper to finish checking out at all
	m.Readiness = []*model.ReadinessCheck{
		{Code: "payment", Label: "At least one payment method is enabled", Ok: len(methods) > 0, Tab: "payment",
			Detail: detailOr(len(methods) > 0, strings.Join(methods, ", "), "Enable a payment method on the Payment tab")},
		{Code: "delivery", Label: "Physical orders have a ready delivery option", Ok: readyEnabled > 0, Tab: "delivery",
			Detail: detailOr(readyEnabled > 0, pluralOptions(readyEnabled, len(opts)), "Enable a delivery option whose setup checklist passes")},
	}
	for _, o := range m.Options {
		if !o.Enabled || o.Ready {
			continue
		}
		for _, c := range o.Readiness {
			if !c.Ok && !c.Advisory {
				m.Readiness = append(m.Readiness, &model.ReadinessCheck{Code: "option:" + o.ID, Label: o.Name + " is ready", Ok: false, Detail: c.Label + " - " + c.Detail, Tab: c.Tab})
				break
			}
		}
	}
	return m, nil
}

func (r *Resolver) deliveryOptionModel(ctx context.Context, o delivery.Option, implicit bool) *model.DeliveryOption {
	m := &model.DeliveryOption{
		ID: o.ID.String(), ChannelID: o.ChannelID.String(), Kind: o.Kind, Name: o.Name, Description: o.Description,
		Enabled: o.Enabled, SortOrder: o.SortOrder, BlockModes: modesToJSON(o.BlockModes),
		PointSource: o.PointSource, CarrierSlug: o.CarrierSlug, Implicit: implicit, KindLabel: o.Kind,
	}
	if k, ok := delivery.KindBySlug(o.Kind); ok {
		m.KindLabel = k.Label
		m.Blocks = toOptionBlocks(k, o.BlockModes)
	} else {
		m.Blocks = []*model.DeliveryOptionBlock{}
	}
	checks := r.DeliverySvc.Readiness(ctx, o.ChannelID, o)
	for _, c := range checks {
		m.Readiness = append(m.Readiness, &model.ReadinessCheck{Code: c.Code, Label: c.Label, Ok: c.OK, Detail: c.Detail, Tab: c.Tab, Advisory: c.Advisory})
	}
	m.Ready = delivery.Ready(checks)
	return m
}

func toDeliveryKindModel(k delivery.Kind) *model.DeliveryKind {
	m := &model.DeliveryKind{
		Slug: k.Slug, Label: k.Label, Description: k.Description, ShipTo: strings.ToUpper(string(k.ShipTo)),
		Owner: ownerOrCore(k.Owner), Blocks: []*model.DeliveryKindBlock{},
	}
	if k.PointLabel != "" {
		m.PointLabel = &k.PointLabel
	}
	for _, b := range k.Blocks {
		kb := &model.DeliveryKindBlock{Type: b.Type, Label: blockLabel(b), Floor: toBlockFloor(b.Floor), Reason: b.Reason}
		if b.Floor == delivery.FloorConfigurable && b.Default.Valid() {
			d := toBlockMode(b.Default)
			kb.Default = &d
		}
		m.Blocks = append(m.Blocks, kb)
	}
	return m
}

func toOptionBlocks(k delivery.Kind, modes map[string]delivery.Mode) []*model.DeliveryOptionBlock {
	out := []*model.DeliveryOptionBlock{}
	for _, rb := range k.Resolve(modes) {
		out = append(out, &model.DeliveryOptionBlock{
			Type: rb.Spec.Type, Label: blockLabel(rb.Spec), Floor: toBlockFloor(rb.Spec.Floor), Mode: toBlockMode(rb.Mode), Reason: rb.Spec.Reason,
		})
	}
	return out
}

// blockLabel names a block by its type when the kind gave it no label
func blockLabel(b delivery.BlockSpec) string {
	if b.Label != "" {
		return b.Label
	}
	return strings.ReplaceAll(b.Type, "_", " ")
}

func toBlockFloor(f delivery.Floor) model.BlockFloor {
	switch f {
	case delivery.FloorLocked:
		return model.BlockFloorLocked
	case delivery.FloorNA:
		return model.BlockFloorNa
	}
	return model.BlockFloorConfigurable
}

func toBlockMode(m delivery.Mode) model.BlockMode {
	switch m {
	case delivery.ModeRequired:
		return model.BlockModeRequired
	case delivery.ModeOptional:
		return model.BlockModeOptional
	}
	return model.BlockModeHidden
}

func toDeliverySourceModel(s delivery.SourceInfo) *model.DeliverySource {
	return &model.DeliverySource{Slug: s.Slug, Label: s.Label, Kinds: nonNilKinds(s.Kinds), Owner: ownerOrCore(s.Owner), Remote: s.Remote, Active: s.Active}
}

// enabledPaymentMethods names the channel's enabled payment methods, for the go-live checklist
func (r *Resolver) enabledPaymentMethods(ctx context.Context, channelID uuid.UUID) []string {
	out := []string{}
	provs, err := r.PaymentSvc.ListProvidersForChannel(ctx, channelID)
	if err != nil {
		return out
	}
	for _, p := range provs {
		if !p.Method.IsEnabled || !p.IsAvailable {
			continue
		}
		name := p.Method.DisplayName
		if name == "" {
			name = p.DisplayName
		}
		out = append(out, name)
	}
	return out
}

// toOptionInput reads the admin input - the modes blob is decoded into typed modes, so a malformed one is refused here rather than stored
func toOptionInput(in model.DeliveryOptionInput) (delivery.SaveOptionInput, error) {
	out := delivery.SaveOptionInput{Name: strings.TrimSpace(in.Name), Enabled: in.Enabled}
	if in.Kind != nil {
		out.Kind = *in.Kind
	}
	if in.Description != nil {
		out.Description = strings.TrimSpace(*in.Description)
	}
	if in.SortOrder != nil {
		out.SortOrder = *in.SortOrder
	}
	if in.PointSource != nil {
		out.PointSource = *in.PointSource
	}
	if in.CarrierSlug != nil {
		out.CarrierSlug = *in.CarrierSlug
	}
	var err error
	if out.BlockModes, err = modesFromJSON(in.BlockModes); err != nil {
		return out, err
	}
	return out, nil
}

var errCheckoutJSON = apperr.New("CHECKOUT_SETTINGS_INVALID", apperr.ClassValidation, "the checkout settings couldn't be read")

func modesFromJSON(v any) (map[string]delivery.Mode, error) {
	out := map[string]delivery.Mode{}
	if v == nil {
		return out, nil
	}
	b, err := json.Marshal(v)
	if err != nil || json.Unmarshal(b, &out) != nil {
		return nil, errCheckoutJSON
	}
	for k, m := range out {
		out[k] = delivery.Mode(strings.ToLower(string(m)))
	}
	return out, nil
}

func modesToJSON(m map[string]delivery.Mode) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = string(v)
	}
	return out
}

func toOrderShipmentModel(s order.Shipment) *model.OrderShipment {
	m := &model.OrderShipment{
		ID: s.ID.String(), Position: s.Position, GroupKey: s.GroupKey, DeliveryKind: s.DeliveryKind,
		DeliveryOptionName: s.DeliveryOptionName, CarrierSlug: s.CarrierSlug, MethodCode: s.MethodCode, MethodName: s.MethodName,
		AmountCents: int32(s.AmountCents), Lines: rawToAny(mustJSON(s.Lines), []any{}), Point: rawToAny(s.Point, nil),
	}
	if s.OriginLocationID != nil {
		id := s.OriginLocationID.String()
		m.OriginLocationID = &id
	}
	if s.DeliveryOptionID != nil {
		id := s.DeliveryOptionID.String()
		m.DeliveryOptionID = &id
	}
	if s.EstimatedDays != nil {
		d := int32(*s.EstimatedDays)
		m.EstimatedDays = &d
	}
	if s.ShipTo != nil {
		m.ShipTo = toOrderAddressModel(*s.ShipTo)
	}
	return m
}

func rawToAny(raw []byte, empty any) any {
	if len(raw) == 0 {
		return empty
	}
	var v any
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return empty
	}
	return v
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func ownerOrCore(o string) string {
	if o == "" {
		return "core"
	}
	return o
}

func nonNilKinds(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func detailOr(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func pluralOptions(ready, total int) string {
	if total == 1 {
		return "1 option ready"
	}
	return strconv.Itoa(ready) + " of " + strconv.Itoa(total) + " options ready"
}

func toDeliveryPointModels(points []delivery.ManualPoint) []*model.DeliveryPointAdmin {
	out := make([]*model.DeliveryPointAdmin, len(points))
	for i, p := range points {
		a := p.Address
		out[i] = &model.DeliveryPointAdmin{
			ID: p.ID.String(), Name: p.Name, Street1: a.Street1, Street2: a.Street2, City: a.City, Region: a.Region,
			PostalCode: a.PostalCode, CountryCode: a.CountryCode, Instructions: p.Instructions, Hours: p.Hours, IsActive: p.Active,
		}
	}
	return out
}

// toManualPoints reads the admin's PO box list - an id is kept so an edit updates its row, a missing one is a new point
func toManualPoints(in []*model.DeliveryPointInput) ([]delivery.ManualPoint, error) {
	out := make([]delivery.ManualPoint, 0, len(in))
	for _, p := range in {
		mp := delivery.ManualPoint{
			Kind: delivery.KindParcelLocker, Name: p.Name, Active: p.IsActive,
			Instructions: strings.TrimSpace(ptrDeref(p.Instructions)), Hours: strings.TrimSpace(ptrDeref(p.Hours)),
			Address: delivery.Address{
				Name: p.Name, Street1: strings.TrimSpace(p.Street1), Street2: strings.TrimSpace(ptrDeref(p.Street2)), City: strings.TrimSpace(p.City),
				Region: strings.TrimSpace(ptrDeref(p.Region)), PostalCode: strings.TrimSpace(p.PostalCode), CountryCode: p.CountryCode,
			},
		}
		if p.ID != nil && *p.ID != "" {
			id, err := uuid.Parse(*p.ID)
			if err != nil {
				return nil, errInvalidID("delivery_point", err)
			}
			mp.ID = id
		}
		out = append(out, mp)
	}
	return out, nil
}

func ptrDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
