package modelconv

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/consent"
	"github.com/skeletg/skelet/internal/customer"
	"github.com/skeletg/skelet/internal/order"
)

func keysOf(items []*model.MetadataItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Key)
	}
	return out
}

// Go map iteration is random, so unsorted output would reshuffle the admin metadata table on every refetch and make an unrelated field look edited
func TestMetadataItemsAreSortedByKey(t *testing.T) {
	in := map[string]string{"zebra": "z", "alpha": "a", "middle": "m", "Alpha": "A"}
	want := []string{"Alpha", "alpha", "middle", "zebra"} // byte order, so uppercase sorts first

	for name, got := range map[string][]*model.MetadataItem{
		"MetadataItems":         MetadataItems(catalog.Metadata(in)),
		"StringMapItems":        StringMapItems(in),
		"CustomerMetadataItems": CustomerMetadataItems(customer.Metadata(in)),
	} {
		gotKeys := keysOf(got)
		if len(gotKeys) != len(want) {
			t.Errorf("%s returned %v, want %v", name, gotKeys, want)
			continue
		}
		for i := range want {
			if gotKeys[i] != want[i] {
				t.Errorf("%s = %v, want %v", name, gotKeys, want)
				break
			}
		}
	}
}

// The GraphQL field is a non-null list, so nil would serialize as null and break the client - an empty map has to come back as an empty slice
func TestMetadataItemsNeverReturnNil(t *testing.T) {
	for name, got := range map[string][]*model.MetadataItem{
		"MetadataItems nil":           MetadataItems(nil),
		"MetadataItems empty":         MetadataItems(catalog.Metadata{}),
		"StringMapItems nil":          StringMapItems(nil),
		"StringMapItems empty":        StringMapItems(map[string]string{}),
		"CustomerMetadataItems nil":   CustomerMetadataItems(nil),
		"CustomerMetadataItems empty": CustomerMetadataItems(customer.Metadata{}),
	} {
		if got == nil {
			t.Errorf("%s = nil, want an empty slice", name)
		}
		if len(got) != 0 {
			t.Errorf("%s = %v, want it empty", name, keysOf(got))
		}
	}
}

// The role map holds ids, the media list holds already-converted models - a role pointing at an id that isn't in the list must resolve to nil rather than the wrong image
func TestResolveMediaRole(t *testing.T) {
	thumbID, otherID := uuid.New(), uuid.New()
	mediaList := []*model.ProductMedia{
		{ID: otherID.String(), URL: "/other.jpg"},
		{ID: thumbID.String(), URL: "/thumb.jpg"},
	}
	roles := map[catalog.MediaRole]uuid.UUID{catalog.MediaRoleThumbnail: thumbID}

	got := ResolveMediaRole(roles, catalog.MediaRoleThumbnail, mediaList)
	if got == nil || got.URL != "/thumb.jpg" {
		t.Errorf("ResolveMediaRole = %v, want the thumbnail", got)
	}
	if got := ResolveMediaRole(nil, catalog.MediaRoleThumbnail, mediaList); got != nil {
		t.Errorf("with no roles = %v, want nil", got)
	}
	if got := ResolveMediaRole(map[catalog.MediaRole]uuid.UUID{}, catalog.MediaRoleThumbnail, mediaList); got != nil {
		t.Errorf("with the role unset = %v, want nil", got)
	}
	if got := ResolveMediaRole(map[catalog.MediaRole]uuid.UUID{catalog.MediaRoleThumbnail: uuid.New()}, catalog.MediaRoleThumbnail, mediaList); got != nil {
		t.Errorf("with a role pointing at a media row that isn't loaded = %v, want nil", got)
	}
	if got := ResolveMediaRole(roles, catalog.MediaRoleThumbnail, nil); got != nil {
		t.Errorf("with no media loaded = %v, want nil", got)
	}
}

// A per-product override of false has to beat a product type that tracks inventory - reading the pointer as "set" rather than dereferencing it is the mistake this pins
func TestEffectiveTracksInventory(t *testing.T) {
	yes, no := true, false
	for name, tc := range map[string]struct {
		product catalog.Product
		ptype   *model.ProductType
		want    bool
	}{
		"override off beats a tracking type":    {catalog.Product{TracksInventoryOverride: &no}, &model.ProductType{TracksInventory: true}, false},
		"override on beats a non-tracking type": {catalog.Product{TracksInventoryOverride: &yes}, &model.ProductType{TracksInventory: false}, true},
		"no override falls back to the type":    {catalog.Product{}, &model.ProductType{TracksInventory: true}, true},
		"no override, non-tracking type":        {catalog.Product{}, &model.ProductType{TracksInventory: false}, false},
		"no type loaded":                        {catalog.Product{}, nil, false},
		"override on with no type":              {catalog.Product{TracksInventoryOverride: &yes}, nil, true},
	} {
		if got := EffectiveTracksInventory(tc.product, tc.ptype); got != tc.want {
			t.Errorf("%s: EffectiveTracksInventory = %v, want %v", name, got, tc.want)
		}
	}
}

// Only configurables have a variant mode, and one saved before the column existed reads as ATTRIBUTE
func TestProductVariantModeModel(t *testing.T) {
	list := catalog.VariantModeList
	attr := catalog.VariantModeAttribute

	if got := ProductVariantModeModel(catalog.Product{Kind: catalog.KindSimple, VariantMode: &list}); got != nil {
		t.Errorf("a simple product = %v, want nil even with a mode stored", got)
	}
	got := ProductVariantModeModel(catalog.Product{Kind: catalog.KindConfigurable})
	if got == nil || *got != model.VariantModeAttribute {
		t.Errorf("a configurable with no mode = %v, want ATTRIBUTE", got)
	}
	got = ProductVariantModeModel(catalog.Product{Kind: catalog.KindConfigurable, VariantMode: &list})
	if got == nil || *got != model.VariantModeList {
		t.Errorf("a LIST configurable = %v, want LIST", got)
	}
	got = ProductVariantModeModel(catalog.Product{Kind: catalog.KindConfigurable, VariantMode: &attr})
	if got == nil || *got != model.VariantModeAttribute {
		t.Errorf("an ATTRIBUTE configurable = %v, want ATTRIBUTE", got)
	}
}

// The GraphQL field is nullable and every non-reference attribute has the empty target, so the two directions have to agree or a saved relationship target silently changes on the next read
func TestReferenceTargetRoundTrips(t *testing.T) {
	for _, target := range []catalog.ReferenceTarget{
		catalog.ReferenceTargetProduct, catalog.ReferenceTargetCategory, catalog.ReferenceTargetCollection,
	} {
		gql := ReferenceTargetToGQL(target)
		if gql == nil {
			t.Errorf("ReferenceTargetToGQL(%q) = nil, want a value", target)
			continue
		}
		if back := ReferenceTargetFromGQL(gql); back != target {
			t.Errorf("%q round-tripped to %q", target, back)
		}
	}
	if got := ReferenceTargetToGQL(""); got != nil {
		t.Errorf("the empty target = %v, want nil for the nullable field", got)
	}
	if got := ReferenceTargetToGQL(catalog.ReferenceTarget("customer")); got != nil {
		t.Errorf("an unknown target = %v, want nil", got)
	}
	if got := ReferenceTargetFromGQL(nil); got != "" {
		t.Errorf("nil from GraphQL = %q, want the empty target", got)
	}
	unknown := model.AttributeReferenceTarget("BLOG_POST")
	if got := ReferenceTargetFromGQL(&unknown); got != "" {
		t.Errorf("an unknown GraphQL target = %q, want the empty target", got)
	}
}

// Every catalog input type needs its own GraphQL value - collapsing two onto one would make the admin attribute editor render the wrong control
func TestAttributeInputTypeToGQLIsInjective(t *testing.T) {
	inputs := []catalog.InputType{
		catalog.InputTypePlainText, catalog.InputTypeNumeric, catalog.InputTypeBoolean,
		catalog.InputTypeDate, catalog.InputTypeDateTime, catalog.InputTypeDropdown,
		catalog.InputTypeMultiselect, catalog.InputTypeSwatch, catalog.InputTypeReference,
	}
	seen := make(map[model.AttributeInputType]catalog.InputType, len(inputs))
	for _, in := range inputs {
		got := AttributeInputTypeToGQL(in)
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, got)
		}
		seen[got] = in
	}
	if got := AttributeInputTypeToGQL(catalog.InputType("hologram")); got != model.AttributeInputTypePlainText {
		t.Errorf("an unknown input type = %q, want the PLAIN_TEXT fallback", got)
	}
}

// Media is pending until the upload is confirmed, so anything that isn't explicitly ready must not read as ready and get rendered on the storefront
func TestProductMediaStatusConversion(t *testing.T) {
	if got := ProductMediaStatusToGQL(catalog.MediaStatusReady); got != model.ProductMediaStatusReady {
		t.Errorf("ready = %q, want READY", got)
	}
	for _, s := range []catalog.MediaStatus{catalog.MediaStatusPending, "", "half-uploaded"} {
		if got := ProductMediaStatusToGQL(s); got != model.ProductMediaStatusPending {
			t.Errorf("%q = %q, want PENDING", s, got)
		}
	}
	if got := ProductMediaStatusFromString("ready"); got != model.ProductMediaStatusReady {
		t.Errorf(`FromString("ready") = %q, want READY`, got)
	}
	for _, s := range []string{"pending", "", "READY", "Ready"} {
		if got := ProductMediaStatusFromString(s); got != model.ProductMediaStatusPending {
			t.Errorf("FromString(%q) = %q, want PENDING", s, got)
		}
	}
}

func TestConfigScopeModel(t *testing.T) {
	channelID := uuid.New()
	locale := "de"

	m := ConfigScopeModel(config.DefaultScope)
	if m.Kind != model.ConfigScopeKindDefault || m.ChannelID != nil || m.Locale != nil {
		t.Errorf("default scope = %+v, want a bare DEFAULT", m)
	}

	m = ConfigScopeModel(config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID, Locale: &locale})
	if m.Kind != model.ConfigScopeKindChannel {
		t.Errorf("kind = %q, want CHANNEL", m.Kind)
	}
	if m.ChannelID == nil || *m.ChannelID != channelID.String() {
		t.Errorf("channelID = %v, want %q", m.ChannelID, channelID)
	}
	if m.Locale == nil || *m.Locale != "de" {
		t.Errorf("locale = %v, want %q", m.Locale, "de")
	}

	// an unrecognized kind must not read as a channel scope, which is the wider one
	if got := ConfigScopeKindModel("website"); got != model.ConfigScopeKindDefault {
		t.Errorf("an unknown kind = %q, want DEFAULT", got)
	}
}

// Marketing and transactional consent are legally different, so an unknown purpose defaulting to marketing is the conservative direction - it means more suppression, not less
func TestConsentConversion(t *testing.T) {
	for in, want := range map[consent.Channel]model.ConsentChannel{
		consent.ChannelEmail: model.ConsentChannelEmail,
		consent.ChannelSMS:   model.ConsentChannelSms,
		"":                   model.ConsentChannelEmail,
		"carrier-pigeon":     model.ConsentChannelEmail,
	} {
		if got := ConsentChannelModel(in); got != want {
			t.Errorf("ConsentChannelModel(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[consent.Purpose]model.ConsentPurpose{
		consent.PurposeTransactional: model.ConsentPurposeTransactional,
		consent.PurposeMarketing:     model.ConsentPurposeMarketing,
		"":                           model.ConsentPurposeMarketing,
		"analytics":                  model.ConsentPurposeMarketing,
	} {
		if got := ConsentPurposeModel(in); got != want {
			t.Errorf("ConsentPurposeModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// A default consent has never been decided by the customer, so it must not carry a timestamp that makes the admin think they opted in at some point
func TestCustomerConsentModelOmitsTheTimestampForDefaults(t *testing.T) {
	def := consent.Definition{Slug: "marketing_email", Purpose: consent.PurposeMarketing, Channel: consent.ChannelEmail}
	updated := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)

	m := CustomerConsentModel(consent.Consent{Definition: def, IsDefault: true, UpdatedAt: updated})
	if m.UpdatedAt != nil {
		t.Errorf("a default consent carries updatedAt %v, want nil", m.UpdatedAt)
	}
	m = CustomerConsentModel(consent.Consent{Definition: def, OptedIn: true, UpdatedAt: updated})
	if m.UpdatedAt == nil || !m.UpdatedAt.Equal(updated) {
		t.Errorf("a decided consent updatedAt = %v, want %v", m.UpdatedAt, updated)
	}
	m = CustomerConsentModel(consent.Consent{Definition: def, OptedIn: true})
	if m.UpdatedAt != nil {
		t.Errorf("a zero timestamp came through as %v, want nil", m.UpdatedAt)
	}
	if m.Kind != "marketing_email" {
		t.Errorf("kind = %q, want the definition slug", m.Kind)
	}
}

// Rich-text bodies are stored as raw JSON, and a row written by an older builder version must not take the whole query down
func TestContentModelsSurviveMalformedRichText(t *testing.T) {
	valid := json.RawMessage(`{"type":"doc","content":[]}`)
	broken := json.RawMessage(`{"type":`)

	cc := CategoryContentModel(catalog.CategoryContent{Locale: "en", Name: "Tires", ShortDescription: valid})
	if cc.ShortDescription == nil {
		t.Error("a valid short description came back nil")
	}
	cc = CategoryContentModel(catalog.CategoryContent{Locale: "en", Name: "Tires", ShortDescription: broken})
	if cc.ShortDescription != nil {
		t.Errorf("a malformed short description = %v, want nil rather than a partial value", cc.ShortDescription)
	}
	cc = CategoryContentModel(catalog.CategoryContent{Locale: "en", Name: "Tires"})
	if cc.ShortDescription != nil {
		t.Errorf("an absent short description = %v, want nil", cc.ShortDescription)
	}

	pcs := ProductContentModels([]catalog.ProductContent{
		{Locale: "en", Name: "Tire", Description: valid, ShortDescription: broken},
	})
	if len(pcs) != 1 {
		t.Fatalf("ProductContentModels returned %d rows, want 1", len(pcs))
	}
	if pcs[0].Description == nil {
		t.Error("a valid description came back nil")
	}
	if pcs[0].DescriptionRichText == nil {
		t.Error("descriptionRichText was not populated alongside description")
	}
	if pcs[0].ShortDescription != nil {
		t.Errorf("a malformed short description = %v, want nil", pcs[0].ShortDescription)
	}

	ccs := CollectionContentModels([]catalog.CollectionContent{{Locale: "en", Name: "Winter", Description: broken}})
	if len(ccs) != 1 || ccs[0].Description != nil {
		t.Errorf("CollectionContentModels = %v, want a row with a nil description", ccs)
	}
	if got := ProductContentModels(nil); got == nil || len(got) != 0 {
		t.Errorf("ProductContentModels(nil) = %v, want an empty non-nil slice", got)
	}
}

func TestCollectionCoverModelPassesNilThrough(t *testing.T) {
	if got := CollectionCoverModel(nil); got != nil {
		t.Errorf("CollectionCoverModel(nil) = %v, want nil", got)
	}
	got := CollectionCoverModel(&catalog.CollectionCover{URL: "/c.jpg", Status: "ready"})
	if got == nil || got.URL != "/c.jpg" || got.Status != model.ProductMediaStatusReady {
		t.Errorf("CollectionCoverModel = %+v, want the cover with a READY status", got)
	}
}

// A system event has no actor, and an empty string would render as an author of "" in the timeline
func TestOrderEventModelOmitsAnAbsentActor(t *testing.T) {
	if m := OrderEventModel(order.OrderEvent{Type: "order.placed"}); m.ActorEmail != nil {
		t.Errorf("a system event actorEmail = %v, want nil", m.ActorEmail)
	}
	m := OrderEventModel(order.OrderEvent{Type: "order.refunded", ActorEmail: "ops@example.invalid"})
	if m.ActorEmail == nil || *m.ActorEmail != "ops@example.invalid" {
		t.Errorf("actorEmail = %v, want the admin address", m.ActorEmail)
	}
}

// Options and components are non-null GraphQL lists, and the money fields narrow from int64
func TestOrderLineModel(t *testing.T) {
	line := order.OrderLine{
		ID: uuid.New(), ProductName: "Tire", ProductSku: "T-1",
		UnitPrice: 12995, Quantity: 2, TotalPrice: 25990,
		Options:    []order.OrderLineOption{{ID: uuid.New(), OptionTitle: "Mounting", PriceDelta: 1500, Quantity: 2}},
		Components: []order.OrderLineComponent{{ID: uuid.New(), ProductSku: "C-1", UnitPrice: 500, Quantity: 4}},
	}

	m := OrderLineModel(line)
	if m.UnitPrice != 12995 || m.TotalPrice != 25990 || m.Quantity != 2 {
		t.Errorf("money = {unit %d total %d qty %v}, want {12995 25990 2}", m.UnitPrice, m.TotalPrice, m.Quantity)
	}
	if len(m.Options) != 1 || m.Options[0].PriceDelta != 1500 {
		t.Errorf("options = %+v, want the mounting option at 1500", m.Options)
	}
	if len(m.Components) != 1 || m.Components[0].UnitPrice != 500 {
		t.Errorf("components = %+v, want the component at 500", m.Components)
	}

	bare := OrderLineModel(order.OrderLine{ID: uuid.New()})
	if bare.Options == nil || bare.Components == nil {
		t.Error("a line with no options or components produced nil lists, want empty ones for the non-null fields")
	}
}

// Every field has to survive the hop in both directions, or an edit silently drops whatever was missed
func TestAddressRoundTripsThroughTheModel(t *testing.T) {
	company, street2, state, phone := "Analytical", "Apt 2", "LDN", "+445550000"
	in := model.AddressInput{
		FirstName: "Ada", LastName: "Lovelace", Company: &company,
		Street1: "1 Bridge St", Street2: &street2, City: "London",
		State: &state, PostalCode: "E1 6AN", CountryCode: "GB", Phone: &phone,
	}
	domain := AddressInput(in)
	out := AddressModel(customer.Address{
		ID: uuid.New(), FirstName: domain.FirstName, LastName: domain.LastName, Company: domain.Company,
		Street1: domain.Street1, Street2: domain.Street2, City: domain.City, State: domain.State,
		PostalCode: domain.PostalCode, CountryCode: domain.CountryCode, Phone: domain.Phone,
	})

	if out.FirstName != in.FirstName || out.LastName != in.LastName || out.City != in.City ||
		out.Street1 != in.Street1 || out.PostalCode != in.PostalCode || out.CountryCode != in.CountryCode {
		t.Errorf("required fields round-tripped to %+v, want %+v", out, in)
	}
	for name, pair := range map[string][2]*string{
		"company": {in.Company, out.Company},
		"street2": {in.Street2, out.Street2},
		"state":   {in.State, out.State},
		"phone":   {in.Phone, out.Phone},
	} {
		want, got := pair[0], pair[1]
		if got == nil || *got != *want {
			t.Errorf("%s round-tripped to %v, want %q", name, got, *want)
		}
	}
}

func TestBoolOrFalse(t *testing.T) {
	yes, no := true, false
	if BoolOrFalse(nil) {
		t.Error("BoolOrFalse(nil) = true, want false")
	}
	if !BoolOrFalse(&yes) {
		t.Error("BoolOrFalse(&true) = false, want true")
	}
	if BoolOrFalse(&no) {
		t.Error("BoolOrFalse(&false) = true, want false")
	}
}
