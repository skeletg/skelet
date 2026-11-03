// Package modelconv holds the domain -> GraphQL admin-model converters that are byte-identical between the admin and storefront resolvers, aliased from the var blocks in graph/{admin,front}/helpers.go
// Only converters whose ENTIRE call-closure is identical belong here - intentionally-divergent ones (admin-superset vs front-subset) stay per-surface
package modelconv

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func AddressInput(input model.AddressInput) customer.AddressInput {
	return customer.AddressInput{
		FirstName:   input.FirstName,
		LastName:    input.LastName,
		Company:     input.Company,
		Street1:     input.Street1,
		Street2:     input.Street2,
		City:        input.City,
		State:       input.State,
		PostalCode:  input.PostalCode,
		CountryCode: input.CountryCode,
		Phone:       input.Phone,
	}
}

func AddressModel(a customer.Address) *model.Address {
	return &model.Address{
		ID:          a.ID.String(),
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Company:     a.Company,
		Street1:     a.Street1,
		Street2:     a.Street2,
		City:        a.City,
		State:       a.State,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}

func CategoryContentModel(cc catalog.CategoryContent) *model.CategoryContent {
	m := &model.CategoryContent{
		Locale:    cc.Locale,
		Name:      cc.Name,
		UpdatedAt: cc.UpdatedAt,
	}
	if len(cc.ShortDescription) > 0 {
		var v any
		if err := json.Unmarshal(cc.ShortDescription, &v); err == nil {
			m.ShortDescription = v
		}
	}
	if len(cc.Description) > 0 {
		var v any
		if err := json.Unmarshal(cc.Description, &v); err == nil {
			m.Description = v
		}
	}
	if cc.MetaTitle != nil {
		m.MetaTitle = cc.MetaTitle
	}
	if cc.MetaDescription != nil {
		m.MetaDescription = cc.MetaDescription
	}
	if cc.MetaKeywords != nil {
		m.MetaKeywords = cc.MetaKeywords
	}
	return m
}

func CollectionContentModels(content []catalog.CollectionContent) []*model.CollectionContent {
	out := make([]*model.CollectionContent, len(content))
	for i, c := range content {
		m := &model.CollectionContent{
			Locale:          c.Locale,
			Name:            c.Name,
			MetaTitle:       c.MetaTitle,
			MetaDescription: c.MetaDescription,
			MetaKeywords:    c.MetaKeywords,
			UpdatedAt:       c.UpdatedAt,
		}
		if len(c.Description) > 0 {
			var v any
			_ = json.Unmarshal(c.Description, &v)
			m.Description = v
		}
		out[i] = m
	}
	return out
}

func CollectionCoverModel(cover *catalog.CollectionCover) *model.CollectionCover {
	if cover == nil {
		return nil
	}
	return &model.CollectionCover{
		URL:      cover.URL,
		AltText:  cover.AltText,
		Width:    cover.Width,
		Height:   cover.Height,
		MimeType: cover.MimeType,
		Status:   ProductMediaStatusFromString(cover.Status),
	}
}

func ConfigScopeKindModel(k string) model.ConfigScopeKind {
	if k == config.ScopeChannel {
		return model.ConfigScopeKindChannel
	}
	return model.ConfigScopeKindDefault
}

func ConfigScopeModel(s config.Scope) *model.ConfigScope {
	m := &model.ConfigScope{Kind: ConfigScopeKindModel(s.Kind)}
	if s.ChannelID != nil {
		id := s.ChannelID.String()
		m.ChannelID = &id
	}
	m.Locale = s.Locale
	return m
}

func ConsentChannelModel(c consent.Channel) model.ConsentChannel {
	switch c {
	case consent.ChannelSMS:
		return model.ConsentChannelSms
	case consent.ChannelPush:
		return model.ConsentChannelPush
	}
	return model.ConsentChannelEmail
}

func ConsentDefinitionModel(d consent.Definition) *model.ConsentDefinition {
	return &model.ConsentDefinition{
		Slug:         d.Slug,
		Name:         d.Name,
		Description:  d.Description,
		Purpose:      ConsentPurposeModel(d.Purpose),
		Channel:      ConsentChannelModel(d.Channel),
		Required:     d.Required,
		DefaultOptIn: d.DefaultOptIn,
	}
}

func ConsentPurposeModel(p consent.Purpose) model.ConsentPurpose {
	switch p {
	case consent.PurposeTransactional:
		return model.ConsentPurposeTransactional
	}
	return model.ConsentPurposeMarketing
}

func CurrencyModel(c currency.Currency) *model.Currency {
	return &model.Currency{
		Code:           c.Code,
		Name:           c.Name,
		Symbol:         c.Symbol,
		Decimals:       int32(c.Decimals),
		SymbolPosition: string(c.SymbolPosition),
		IsActive:       c.IsActive,
		SortOrder:      int32(c.SortOrder),
		Metadata:       StringMapItems(c.Metadata),
		ExchangeRate:   c.ExchangeRate,
		RateUpdatedAt:  c.RateUpdatedAt,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

func CustomerConsentModel(c consent.Consent) *model.CustomerConsent {
	m := &model.CustomerConsent{
		Kind:       c.Definition.Slug,
		Definition: ConsentDefinitionModel(c.Definition),
		OptedIn:    c.OptedIn,
		Source:     c.Source,
		IsDefault:  c.IsDefault,
	}
	if !c.IsDefault && !c.UpdatedAt.IsZero() {
		m.UpdatedAt = &c.UpdatedAt
	}
	return m
}

func CustomerGroupModel(g customer.CustomerGroup) *model.CustomerGroup {
	return &model.CustomerGroup{
		ID:          g.ID.String(),
		Name:        g.Name,
		Slug:        g.Slug,
		Description: g.Description,
		IsDefault:   g.IsDefault,
		MemberCount: int32(g.MemberCount),
		CreatedAt:   g.CreatedAt,
	}
}

func LocaleModel(l locale.Locale) *model.Locale {
	return &model.Locale{
		Code:                  l.Code,
		Name:                  l.Name,
		NativeName:            l.NativeName,
		IsActive:              l.IsActive,
		IsRtl:                 l.IsRTL,
		SortOrder:             int32(l.SortOrder),
		DateFormat:            l.DateFormat,
		TimeFormat:            l.TimeFormat,
		DecimalSeparator:      l.DecimalSeparator,
		ThousandsSeparator:    l.ThousandsSeparator,
		FirstDayOfWeek:        int32(l.FirstDayOfWeek),
		DefaultCountryCode:    l.DefaultCountryCode,
		FlagEmoji:             l.FlagEmoji,
		HreflangValue:         l.HreflangValue,
		Script:                l.Script,
		PluralRule:            l.PluralRule,
		EmailFromNameOverride: l.EmailFromNameOverride,
		Description:           l.Description,
		Metadata:              StringMapItems(l.Metadata),
		CreatedAt:             l.CreatedAt,
		UpdatedAt:             l.UpdatedAt,
	}
}

func MetadataItems(m catalog.Metadata) []*model.MetadataItem { // sorted by key - Go map iteration is non-deterministic, and the admin UI would reshuffle on every refetch
	if len(m) == 0 {
		return []*model.MetadataItem{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*model.MetadataItem, len(keys))
	for i, k := range keys {
		out[i] = &model.MetadataItem{Key: k, Value: m[k]}
	}
	return out
}

func OrderAddressModel(a order.OrderAddress) *model.OrderAddress {
	return &model.OrderAddress{
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Company:     a.Company,
		Street1:     a.Street1,
		Street2:     a.Street2,
		City:        a.City,
		State:       a.State,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}

func OrderEventModel(e order.OrderEvent) *model.OrderEvent {
	m := &model.OrderEvent{
		ID:        e.ID.String(),
		Type:      e.Type,
		Message:   e.Message,
		CreatedAt: e.CreatedAt,
	}
	if e.ActorEmail != "" {
		actor := e.ActorEmail
		m.ActorEmail = &actor
	}
	return m
}

func OrderLineModel(l order.OrderLine) *model.OrderLine {
	options := make([]*model.OrderLineOption, len(l.Options))
	for i, o := range l.Options {
		options[i] = &model.OrderLineOption{
			ID:          o.ID.String(),
			InputType:   o.InputType,
			OptionTitle: o.OptionTitle,
			ValueLabel:  o.ValueLabel,
			TextValue:   o.TextValue,
			PriceDelta:  int32(o.PriceDelta),
			Quantity:    o.Quantity,
			FileURL:     o.FileURL,
		}
	}
	components := make([]*model.OrderLineComponent, len(l.Components))
	for i, c := range l.Components {
		components[i] = &model.OrderLineComponent{
			ID:         c.ID.String(),
			Sku:        c.ProductSku,
			Name:       c.ProductName,
			UnitPrice:  int32(c.UnitPrice),
			Quantity:   c.Quantity,
			Returnable: c.Returnable,
		}
	}
	return &model.OrderLine{
		ID:         l.ID.String(),
		Name:       l.ProductName,
		Sku:        l.ProductSku,
		UnitPrice:  int32(l.UnitPrice),
		Quantity:   l.Quantity,
		TotalPrice: int32(l.TotalPrice),
		IsGift:     l.IsGift,
		IsCustom:   l.IsCustom,
		Options:    options,
		Components: components,
	}
}

func ProductContentModels(content []catalog.ProductContent) []*model.ProductContent {
	out := make([]*model.ProductContent, len(content))
	for i, c := range content {
		m := &model.ProductContent{
			Locale:          c.Locale,
			Name:            c.Name,
			MetaTitle:       c.MetaTitle,
			MetaDescription: c.MetaDescription,
			MetaKeywords:    c.MetaKeywords,
			UpdatedAt:       c.UpdatedAt,
		}
		if len(c.ShortDescription) > 0 {
			var v any
			_ = json.Unmarshal(c.ShortDescription, &v)
			m.ShortDescription = v
		}
		if len(c.Description) > 0 {
			var v any
			_ = json.Unmarshal(c.Description, &v)
			m.Description = v
			m.DescriptionRichText = v
		}
		out[i] = m
	}
	return out
}

func ProductMediaModel(m catalog.ProductMedia) *model.ProductMedia {
	return &model.ProductMedia{
		ID:        m.ID.String(),
		URL:       m.URL,
		AltText:   m.AltText,
		SortOrder: m.SortOrder,
		MimeType:  m.MimeType,
		Width:     m.Width,
		Height:    m.Height,
		SizeBytes: int32(m.SizeBytes),
		Status:    ProductMediaStatusToGQL(m.Status),
		CreatedAt: m.CreatedAt,
	}
}

func StringMapItems(m map[string]string) []*model.MetadataItem { // sorted by key for stable rendering, same reason as MetadataItems
	if len(m) == 0 {
		return []*model.MetadataItem{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*model.MetadataItem, 0, len(keys))
	for _, k := range keys {
		out = append(out, &model.MetadataItem{Key: k, Value: m[k]})
	}
	return out
}

func ProductMediaStatusToGQL(s catalog.MediaStatus) model.ProductMediaStatus {
	switch s {
	case catalog.MediaStatusReady:
		return model.ProductMediaStatusReady
	}
	return model.ProductMediaStatusPending
}

func ProductMediaStatusFromString(s string) model.ProductMediaStatus {
	if s == "ready" {
		return model.ProductMediaStatusReady
	}
	return model.ProductMediaStatusPending
}

func AttributeInputTypeToGQL(t catalog.InputType) model.AttributeInputType {
	switch t {
	case catalog.InputTypePlainText:
		return model.AttributeInputTypePlainText
	case catalog.InputTypeNumeric:
		return model.AttributeInputTypeNumeric
	case catalog.InputTypeBoolean:
		return model.AttributeInputTypeBoolean
	case catalog.InputTypeDate:
		return model.AttributeInputTypeDate
	case catalog.InputTypeDateTime:
		return model.AttributeInputTypeDateTime
	case catalog.InputTypeDropdown:
		return model.AttributeInputTypeDropdown
	case catalog.InputTypeMultiselect:
		return model.AttributeInputTypeMultiselect
	case catalog.InputTypeSwatch:
		return model.AttributeInputTypeSwatch
	case catalog.InputTypeReference:
		return model.AttributeInputTypeReference
	}
	return model.AttributeInputTypePlainText
}

func ReferenceTargetToGQL(t catalog.ReferenceTarget) *model.AttributeReferenceTarget { // nil for the empty target, which is every non-reference attribute - matches the nullable relationshipTarget field
	var out model.AttributeReferenceTarget
	switch t {
	case catalog.ReferenceTargetProduct:
		out = model.AttributeReferenceTargetProduct
	case catalog.ReferenceTargetCategory:
		out = model.AttributeReferenceTargetCategory
	case catalog.ReferenceTargetCollection:
		out = model.AttributeReferenceTargetCollection
	default:
		return nil
	}
	return &out
}

func ReferenceTargetFromGQL(t *model.AttributeReferenceTarget) catalog.ReferenceTarget { // nil collapses to the empty target
	if t == nil {
		return ""
	}
	switch *t {
	case model.AttributeReferenceTargetProduct:
		return catalog.ReferenceTargetProduct
	case model.AttributeReferenceTargetCategory:
		return catalog.ReferenceTargetCategory
	case model.AttributeReferenceTargetCollection:
		return catalog.ReferenceTargetCollection
	}
	return ""
}

func CustomerMetadataItems(m customer.Metadata) []*model.MetadataItem { // stable key order for deterministic rendering
	if len(m) == 0 {
		return []*model.MetadataItem{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*model.MetadataItem, len(keys))
	for i, k := range keys {
		out[i] = &model.MetadataItem{Key: k, Value: m[k]}
	}
	return out
}

// Null for non-configurables, and a configurable with no explicit mode defaults to ATTRIBUTE
func ProductVariantModeModel(p catalog.Product) *model.VariantMode {
	if p.Kind != catalog.KindConfigurable {
		return nil
	}
	mode := model.VariantModeAttribute
	if p.VariantMode != nil && *p.VariantMode == catalog.VariantModeList {
		mode = model.VariantModeList
	}
	return &mode
}

func GroupedMemberModels(members []catalog.GroupedMember) []*model.GroupedMember {
	out := make([]*model.GroupedMember, len(members))
	for i, m := range members {
		out[i] = GroupedMemberModel(m)
	}
	return out
}

func GroupedMemberModel(m catalog.GroupedMember) *model.GroupedMember {
	prod := &model.Product{
		ID:                  m.MemberID.String(),
		URL:                 m.MemberURL,
		Sku:                 m.MemberSKU,
		Kind:                model.ProductKindSimple,
		DefaultPrice:        int32(m.MemberDefaultPrice),
		DefaultCurrencyCode: m.MemberCurrency,
		MinOrderQty:         m.MemberMinOrderQty, // each member is its own cart line, so its own qty rules apply - inStock / salableQuantity are field resolvers and fill themselves in
		MaxOrderQty:         m.MemberMaxOrderQty,
		QuantityStep:        m.MemberQuantityStep,
		QuantityDecimals:    int32(m.MemberQuantityDecimals),
	}
	if m.MemberName != "" {
		name := m.MemberName
		prod.Name = &name
	}
	return &model.GroupedMember{
		ID:              m.ID.String(),
		Member:          prod,
		DefaultQuantity: m.DefaultQuantity,
		SortOrder:       m.SortOrder,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func EffectiveTracksInventory(p catalog.Product, pt *model.ProductType) bool { // per-product override wins, otherwise the product type's tracksInventory default
	if p.TracksInventoryOverride != nil {
		return *p.TracksInventoryOverride
	}
	if pt != nil {
		return pt.TracksInventory
	}
	return false
}

func ResolveMediaRole(roles map[catalog.MediaRole]uuid.UUID, role catalog.MediaRole, mediaList []*model.ProductMedia) *model.ProductMedia {
	if roles == nil {
		return nil
	}
	id, ok := roles[role]
	if !ok {
		return nil
	}
	idStr := id.String()
	for _, m := range mediaList {
		if m.ID == idStr {
			return m
		}
	}
	return nil
}

func BoolOrFalse(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
