package admin

// Helpers for cms.resolvers.go. Kept in a separate file so gqlgen doesn't
// move them into a /* */ comment block on regeneration of resolvers.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/cms"
	"github.com/skeletg/skelet/internal/ptr"
)

// parseCMSPageIDs converts a list of GraphQL ID strings to UUIDs, failing on
// the first invalid one (used by the bulk page mutations).
func parseCMSPageIDs(ids []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		uid, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", id, err)
		}
		out = append(out, uid)
	}
	return out, nil
}

func cmsInputTypeToString(t model.CMSInputType) string {
	switch t {
	case model.CMSInputTypeText:
		return "text"
	case model.CMSInputTypeTextarea:
		return "textarea"
	case model.CMSInputTypeRichtext:
		return "richtext"
	case model.CMSInputTypeNumber:
		return "number"
	case model.CMSInputTypeBoolean:
		return "boolean"
	case model.CMSInputTypeDate:
		return "date"
	case model.CMSInputTypeDatetime:
		return "datetime"
	case model.CMSInputTypeURL:
		return "url"
	case model.CMSInputTypeSelect:
		return "select"
	case model.CMSInputTypeMultiselect:
		return "multiselect"
	case model.CMSInputTypeAsset:
		return "asset"
	case model.CMSInputTypeAssets:
		return "assets"
	case model.CMSInputTypeRelationship:
		return "relationship"
	case model.CMSInputTypeJSON:
		return "json"
	}
	return "text"
}

func toCMSInputType(t cms.InputType) model.CMSInputType {
	switch t {
	case cms.InputText:
		return model.CMSInputTypeText
	case cms.InputTextarea:
		return model.CMSInputTypeTextarea
	case cms.InputRichText:
		return model.CMSInputTypeRichtext
	case cms.InputNumber:
		return model.CMSInputTypeNumber
	case cms.InputBoolean:
		return model.CMSInputTypeBoolean
	case cms.InputDate:
		return model.CMSInputTypeDate
	case cms.InputDateTime:
		return model.CMSInputTypeDatetime
	case cms.InputURL:
		return model.CMSInputTypeURL
	case cms.InputSelect:
		return model.CMSInputTypeSelect
	case cms.InputMultiSelect:
		return model.CMSInputTypeMultiselect
	case cms.InputAsset:
		return model.CMSInputTypeAsset
	case cms.InputAssets:
		return model.CMSInputTypeAssets
	case cms.InputRelationship:
		return model.CMSInputTypeRelationship
	case cms.InputJSON:
		return model.CMSInputTypeJSON
	}
	return model.CMSInputTypeText
}

func cmsTargetToString(t model.CMSRelationshipTarget) string {
	switch t {
	case model.CMSRelationshipTargetCmsPage:
		return "cms_page"
	case model.CMSRelationshipTargetBlogPost:
		return "blog_post"
	case model.CMSRelationshipTargetProduct:
		return "product"
	case model.CMSRelationshipTargetCollection:
		return "collection"
	case model.CMSRelationshipTargetCategory:
		return "category"
	}
	return "cms_page"
}

func cmsMetadataFromInput(in []*model.MetadataInput) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		out[item.Key] = item.Value
	}
	return out
}

// ==================== Conversions ====================

func toCMSAttributeModel(a cms.Attribute) *model.CMSAttribute {
	m := &model.CMSAttribute{
		ID:                a.ID.String(),
		Slug:              a.Slug,
		InputType:         toCMSInputType(a.InputType),
		IsTranslatable:    a.IsTranslatable,
		IsRequiredDefault: a.IsRequiredDefault,
		Unit:              a.Unit,
		Validation:        unmarshalJSON(a.Validation),
		DefaultValue:      unmarshalJSON(a.DefaultValue),
		Metadata:          cmsMetadataItems(a.Metadata),
		Values:            toCMSAttributeValuesModel(a.Values),
		AllContent:        toCMSAttributeContentModels(a.AllContent),
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}
	if a.RelationshipTarget != nil {
		t := toCMSRelationshipTarget(*a.RelationshipTarget)
		m.RelationshipTarget = &t
	}
	if a.Content != nil {
		m.Content = toCMSAttributeContentModel(*a.Content)
	}
	return m
}

func toCMSAttributeContentModel(c cms.AttributeContent) *model.CMSAttributeContent {
	return &model.CMSAttributeContent{
		Locale: c.Locale, Name: c.Name,
		HelperText: c.HelperText, Placeholder: c.Placeholder,
		UpdatedAt: c.UpdatedAt,
	}
}

func toCMSAttributeContentModels(in []cms.AttributeContent) []*model.CMSAttributeContent {
	out := make([]*model.CMSAttributeContent, len(in))
	for i, c := range in {
		out[i] = toCMSAttributeContentModel(c)
	}
	return out
}

func toCMSAttributeValueModel(v cms.AttributeValue) *model.CMSAttributeValue {
	allContent := make([]*model.CMSAttributeValueContent, len(v.AllContent))
	for i, c := range v.AllContent {
		allContent[i] = &model.CMSAttributeValueContent{Locale: c.Locale, Name: c.Name, UpdatedAt: c.UpdatedAt}
	}
	return &model.CMSAttributeValue{
		ID:          v.ID.String(),
		AttributeID: v.AttributeID.String(),
		Slug:        v.Slug,
		Position:    v.Position,
		AllContent:  allContent,
		CreatedAt:   v.CreatedAt,
	}
}

func toCMSAttributeValuesModel(in []cms.AttributeValue) []*model.CMSAttributeValue {
	out := make([]*model.CMSAttributeValue, len(in))
	for i, v := range in {
		out[i] = toCMSAttributeValueModel(v)
	}
	return out
}

func toCMSAttributeGroupModel(g cms.AttributeGroup) *model.CMSAttributeGroup {
	return &model.CMSAttributeGroup{
		ID: g.ID.String(), Slug: g.Slug, Position: g.Position,
		CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
	}
}

func toCMSPageTypeModel(pt cms.PageType) *model.CMSPageType {
	m := &model.CMSPageType{
		ID:                pt.ID.String(),
		Slug:              pt.Slug,
		Icon:              pt.Icon,
		IsSystem:          pt.IsSystem,
		Position:          pt.Position,
		ChannelRoots:      toCMSPageTypeChannelRootModels(pt.ChannelRoots),
		DefaultSeoNoindex: pt.DefaultSEONoindex,
		IsSingleton:       pt.IsSingleton,
		AllContent:        toCMSPageTypeContentModels(pt.AllContent),
		Attributes:        toCMSPageTypeAttributesModel(pt.Attributes),
		CreatedAt:         pt.CreatedAt,
		UpdatedAt:         pt.UpdatedAt,
	}
	if pt.RootSlug != "" {
		rs := pt.RootSlug
		m.RootSlug = &rs
	}
	if pt.DefaultMetaTitle != "" {
		dmt := pt.DefaultMetaTitle
		m.DefaultMetaTitle = &dmt
	}
	if len(pt.DefaultBlocks) > 0 {
		var v any
		if err := json.Unmarshal(pt.DefaultBlocks, &v); err == nil {
			m.DefaultBlocks = v
		}
	}
	if pt.Content != nil {
		m.Content = toCMSPageTypeContentModel(*pt.Content)
	}
	return m
}

func toCMSPageTypeChannelRootModels(roots []cms.PageTypeChannelRoot) []*model.CMSPageTypeChannelRoot {
	out := make([]*model.CMSPageTypeChannelRoot, len(roots))
	for i, r := range roots {
		out[i] = &model.CMSPageTypeChannelRoot{ChannelID: r.ChannelID.String(), RootSlug: r.RootSlug}
	}
	return out
}

func toCMSPageTypeContentModel(c cms.PageTypeContent) *model.CMSPageTypeContent {
	return &model.CMSPageTypeContent{
		Locale: c.Locale, Name: c.Name, Description: c.Description,
		UpdatedAt: c.UpdatedAt,
	}
}

func toCMSPageTypeContentModels(in []cms.PageTypeContent) []*model.CMSPageTypeContent {
	out := make([]*model.CMSPageTypeContent, len(in))
	for i, c := range in {
		out[i] = toCMSPageTypeContentModel(c)
	}
	return out
}

func toCMSPageTypeAttributesModel(in []cms.PageTypeAttribute) []*model.CMSPageTypeAttribute {
	out := make([]*model.CMSPageTypeAttribute, len(in))
	for i, pta := range in {
		m := &model.CMSPageTypeAttribute{
			Attribute:      toCMSAttributeModel(pta.Attribute),
			Position:       pta.Position,
			IsRequired:     pta.IsRequired,
			ConditionValue: pta.ConditionValue,
		}
		if pta.GroupID != nil {
			id := pta.GroupID.String()
			m.GroupID = &id
		}
		if pta.ConditionAttributeID != nil {
			cid := pta.ConditionAttributeID.String()
			m.ConditionAttributeID = &cid
		}
		out[i] = m
	}
	return out
}

func toCMSPageModel(p cms.Page) *model.CMSPage {
	m := &model.CMSPage{
		ID:              p.ID.String(),
		PageTypeID:      p.PageTypeID.String(),
		Slug:            p.Slug,
		BasePath:        p.BasePath,
		URLPath:         p.URLPath,
		Position:        p.Position,
		IsPublished:     p.IsPublished,
		SeoNoindex:      p.SEONoindex,
		PublishedAt:     p.PublishedAt,
		ScheduledAt:     p.ScheduledAt,
		Data:            unmarshalJSON(p.Data),
		Metadata:        cmsMetadataItems(p.Metadata),
		PrivateMetadata: cmsMetadataItems(p.PrivateMetadata),
		Tags:            p.Tags,
		AllContent:      toCMSPageContentModels(p.AllContent),
		Selections:      toCMSPageSelectionsModel(p.Selections),
		Relationships:   toCMSPageRelationshipsModel(p.Relationships),
		ChannelListings: toCMSPageChannelListings(p.ChannelListings),
		ExperimentKey:   p.ExperimentKey,
		Variants:        toCMSPageVariantModels(p.Variants),
		// Editorial workflow. Domain states are lower_snake; the GraphQL enum is
		// UPPER (DRAFT/IN_REVIEW/…), so upper-case the literal.
		WorkflowState: model.CMSWorkflowState(strings.ToUpper(string(p.WorkflowState))),
		AssignedTo:    uuidPtrStr(p.AssignedTo),
		SubmittedBy:   uuidPtrStr(p.SubmittedBy),
		SubmittedAt:   p.SubmittedAt,
		ApprovedBy:    uuidPtrStr(p.ApprovedBy),
		ApprovedAt:    p.ApprovedAt,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
	if p.ParentID != nil {
		id := p.ParentID.String()
		m.ParentID = &id
	}
	if p.Content != nil {
		m.Content = toCMSPageContentModel(*p.Content)
	}
	return m
}

func uuidPtrStr(p *uuid.UUID) *string {
	if p == nil {
		return nil
	}
	s := p.String()
	return &s
}

// tagModel builds a tag model with its current page count. Lives here (not in
// *.resolvers.go) so gqlgen doesn't evict it on regeneration.
func (r *Resolver) tagModel(ctx context.Context, t cms.Tag) *model.CMSTag {
	counts, _ := r.CMSSvc.TagUsageCounts(ctx)
	return toCMSTagModel(t, counts[t.Slug])
}

func toCMSTagModel(t cms.Tag, pageCount int) *model.CMSTag {
	return &model.CMSTag{
		Slug:      t.Slug,
		Name:      t.Name,
		PageCount: int32(pageCount),
		CreatedAt: t.CreatedAt,
	}
}

func toCMSPageCommentModel(c cms.PageComment) *model.CMSPageComment {
	return &model.CMSPageComment{
		ID:         c.ID.String(),
		PageID:     c.PageID.String(),
		Locale:     c.Locale,
		BlockID:    c.BlockID,
		AuthorID:   c.AuthorID.String(),
		Body:       c.Body,
		Resolved:   c.Resolved,
		ResolvedBy: uuidPtrStr(c.ResolvedBy),
		ResolvedAt: c.ResolvedAt,
		CreatedAt:  c.CreatedAt,
	}
}

func toCMSPageCommentModels(cs []cms.PageComment) []*model.CMSPageComment {
	out := make([]*model.CMSPageComment, len(cs))
	for i, c := range cs {
		out[i] = toCMSPageCommentModel(c)
	}
	return out
}

func toCMSPageVariantModels(vs []cms.PageVariant) []*model.CMSPageVariant {
	out := make([]*model.CMSPageVariant, len(vs))
	for i, v := range vs {
		var body any
		if len(v.Body) > 0 {
			_ = json.Unmarshal(v.Body, &body)
		}
		out[i] = &model.CMSPageVariant{VariantKey: v.VariantKey, Body: body}
	}
	return out
}

func toCMSPageChannelListings(in []cms.PageChannelListing) []*model.CMSPageChannelListing {
	out := make([]*model.CMSPageChannelListing, len(in))
	for i, l := range in {
		out[i] = &model.CMSPageChannelListing{
			ChannelID:   l.ChannelID.String(),
			IsPublished: l.IsPublished,
			SeoNoindex:  l.SEONoindex,
			IsHomepage:  l.IsHomepage,
			CreatedAt:   l.CreatedAt,
			UpdatedAt:   l.UpdatedAt,
		}
	}
	return out
}

// ==================== Inputs ====================

func fromCreateCMSAttributeInput(in model.CreateCMSAttributeInput) cms.CreateAttributeInput {
	out := cms.CreateAttributeInput{
		Slug:              in.Slug,
		InputType:         cms.InputType(cmsInputTypeToString(in.InputType)),
		IsTranslatable:    boolDeref(in.IsTranslatable, false),
		IsRequiredDefault: boolDeref(in.IsRequiredDefault, false),
		Unit:              ptr.Deref(in.Unit),
		Validation:        rawJSON(in.Validation),
		DefaultValue:      rawJSON(in.DefaultValue),
		Metadata:          cmsMetadataFromInput(in.Metadata),
	}
	if in.RelationshipTarget != nil {
		t := cms.RelationshipTarget(cmsTargetToString(*in.RelationshipTarget))
		out.RelationshipTarget = &t
	}
	if len(in.Values) > 0 {
		out.Values = make([]cms.AttributeValueInput, len(in.Values))
		for i, v := range in.Values {
			out.Values[i] = cms.AttributeValueInput{
				Slug: v.Slug, Position: intDeref(v.Position, 0),
			}
		}
	}
	return out
}

func fromCreateCMSPageInput(in model.CreateCMSPageInput) (cms.CreatePageInput, error) {
	pageTypeID, err := parseUUIDOpt(&in.PageTypeID)
	if err != nil {
		return cms.CreatePageInput{}, err
	}
	if pageTypeID == nil {
		return cms.CreatePageInput{}, nil
	}
	out := cms.CreatePageInput{
		PageTypeID:      *pageTypeID,
		Slug:            in.Slug,
		Position:        intDeref(in.Position, 0),
		IsPublished:     boolDeref(in.IsPublished, false),
		SEONoindex:      boolDeref(in.SeoNoindex, false),
		PublishedAt:     in.PublishedAt,
		Data:            rawJSON(in.Data),
		Metadata:        cmsMetadataFromInput(in.Metadata),
		PrivateMetadata: cmsMetadataFromInput(in.PrivateMetadata),
		Locale:          in.Locale,
		Title:           in.Title,
		Body:            rawJSON(in.Body),
		MetaTitle:       ptr.Deref(in.MetaTitle),
		MetaDescription: ptr.Deref(in.MetaDescription),
		MetaKeywords:    ptr.Deref(in.MetaKeywords),
		ContentData:     rawJSON(in.ContentData),
	}
	if v, err := parseUUIDOpt(in.ParentID); err != nil {
		return cms.CreatePageInput{}, err
	} else {
		out.ParentID = v
	}
	if len(in.Selections) > 0 {
		out.Selections = make([]cms.PageAttributeSelection, 0, len(in.Selections))
		for _, s := range in.Selections {
			aid, err := parseUUIDOpt(&s.AttributeID)
			if err != nil {
				return cms.CreatePageInput{}, err
			}
			vid, err := parseUUIDOpt(&s.ValueID)
			if err != nil {
				return cms.CreatePageInput{}, err
			}
			out.Selections = append(out.Selections, cms.PageAttributeSelection{
				AttributeID: *aid, ValueID: *vid,
			})
		}
	}
	if len(in.Relationships) > 0 {
		out.Relationships = make([]cms.PageRelationship, 0, len(in.Relationships))
		for _, rl := range in.Relationships {
			aid, err := parseUUIDOpt(&rl.AttributeID)
			if err != nil {
				return cms.CreatePageInput{}, err
			}
			tid, err := parseUUIDOpt(&rl.TargetID)
			if err != nil {
				return cms.CreatePageInput{}, err
			}
			out.Relationships = append(out.Relationships, cms.PageRelationship{
				AttributeID: *aid,
				TargetType:  cms.RelationshipTarget(cmsTargetToString(rl.TargetType)),
				TargetID:    *tid,
				Position:    intDeref(rl.Position, 0),
			})
		}
	}
	return out, nil
}

func fromUpdateCMSPageInput(in model.UpdateCMSPageInput) (cms.UpdatePageInput, error) {
	id, err := parseUUIDOpt(&in.ID)
	if err != nil || id == nil {
		return cms.UpdatePageInput{}, err
	}
	out := cms.UpdatePageInput{
		ID:              *id,
		Slug:            in.Slug,
		Position:        in.Position,
		IsPublished:     in.IsPublished,
		SEONoindex:      in.SeoNoindex,
		ClearParent:     boolDeref(in.ClearParent, false),
		ClearPublished:  boolDeref(in.ClearPublished, false),
		Data:            rawJSON(in.Data),
		Metadata:        cmsMetadataFromInput(in.Metadata),
		PrivateMetadata: cmsMetadataFromInput(in.PrivateMetadata),
	}
	// Optional list: nil when the client omits `tags` (leave unchanged); a
	// provided (possibly empty) list replaces the set.
	if in.Tags != nil {
		tags := in.Tags
		out.Tags = &tags
	}
	if in.PublishedAt != nil {
		t := *in.PublishedAt
		out.PublishedAt = &t
	}
	if v, err := parseUUIDOpt(in.ParentID); err != nil {
		return cms.UpdatePageInput{}, err
	} else {
		out.ParentID = v
	}
	return out, nil
}

func toCMSSnippetModel(s cms.Snippet) *model.CMSSnippet {
	return &model.CMSSnippet{
		ID:        s.ID.String(),
		Slug:      s.Slug,
		Name:      s.Name,
		Blocks:    unmarshalJSON(s.Blocks),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func toCMSBlockTypeModel(b cms.BlockType) *model.CMSBlockType {
	return &model.CMSBlockType{
		ID:           b.ID.String(),
		Key:          b.Key,
		Name:         b.Name,
		Icon:         b.Icon,
		Fields:       unmarshalJSON(b.Fields),
		HTMLTemplate: b.HTMLTemplate,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
	}
}

func toCMSPageRevisionModel(r cms.PageRevision) *model.CMSPageRevision {
	m := &model.CMSPageRevision{
		ID:              r.ID.String(),
		PageID:          r.PageID.String(),
		Locale:          r.Locale,
		Title:           r.Title,
		Body:            unmarshalJSON(r.Body),
		MetaTitle:       r.MetaTitle,
		MetaDescription: r.MetaDescription,
		MetaKeywords:    r.MetaKeywords,
		CreatedAt:       r.CreatedAt,
	}
	if r.AuthorAdminID != nil {
		id := r.AuthorAdminID.String()
		m.AuthorAdminID = &id
	}
	return m
}
