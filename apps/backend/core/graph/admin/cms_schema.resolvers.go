package admin

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *cMSPageTypeResolver) PageCount(ctx context.Context, obj *model.CMSPageType) (int32, error) {
	id, err := uuid.Parse(obj.ID)
	if err != nil {
		return 0, err
	}
	n, err := r.CMSSvc.CountPagesByType(ctx, id)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *mutationResolver) CreateCMSAttribute(ctx context.Context, input model.CreateCMSAttributeInput) (*model.CMSAttribute, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	in := fromCreateCMSAttributeInput(input)
	a, err := r.CMSSvc.CreateAttribute(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCMSAttributeModel(a), nil
}

func (r *mutationResolver) UpdateCMSAttribute(ctx context.Context, input model.UpdateCMSAttributeInput) (*model.CMSAttribute, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	in := cms.UpdateAttributeInput{
		ID:                id,
		IsTranslatable:    input.IsTranslatable,
		IsRequiredDefault: input.IsRequiredDefault,
		Unit:              input.Unit,
		Validation:        rawJSON(input.Validation),
		DefaultValue:      rawJSON(input.DefaultValue),
	}
	if input.RelationshipTarget != nil {
		t := cms.RelationshipTarget(cmsTargetToString(*input.RelationshipTarget))
		in.RelationshipTarget = &t
	}
	if input.Metadata != nil {
		in.Metadata = cmsMetadataFromInput(input.Metadata)
	}
	a, err := r.CMSSvc.UpdateAttribute(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCMSAttributeModel(a), nil
}

func (r *mutationResolver) DeleteCMSAttribute(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CMSSvc.DeleteAttribute(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertCMSAttributeContent(ctx context.Context, input model.UpsertCMSAttributeContentInput) (*model.CMSAttribute, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	uid, err := parseID(input.AttributeID, "attribute")
	if err != nil {
		return nil, err
	}
	a, err := r.CMSSvc.UpsertAttributeContent(ctx, uid, cms.AttributeContentInput{
		Locale:      input.Locale,
		Name:        input.Name,
		HelperText:  ptr.Deref(input.HelperText),
		Placeholder: ptr.Deref(input.Placeholder),
	})
	if err != nil {
		return nil, err
	}
	return toCMSAttributeModel(a), nil
}

func (r *mutationResolver) AddCMSAttributeValue(ctx context.Context, attributeID string, slug string, position *int32) (*model.CMSAttributeValue, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	uid, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	v, err := r.CMSSvc.AddAttributeValue(ctx, uid, cms.AttributeValueInput{
		Slug: slug, Position: intDeref(position, 0),
	})
	if err != nil {
		return nil, err
	}
	return toCMSAttributeValueModel(v), nil
}

func (r *mutationResolver) UpdateCMSAttributeValue(ctx context.Context, id string, slug string, position int32) (*model.CMSAttributeValue, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	v, err := r.CMSSvc.UpdateAttributeValue(ctx, uid, cms.AttributeValueInput{
		Slug: slug, Position: position,
	})
	if err != nil {
		return nil, err
	}
	return toCMSAttributeValueModel(v), nil
}

func (r *mutationResolver) UpsertCMSAttributeValueContent(ctx context.Context, valueID string, locale string, name string) (*model.CMSAttribute, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	vid, err := parseID(valueID, "value")
	if err != nil {
		return nil, err
	}
	a, err := r.CMSSvc.UpsertAttributeValueContent(ctx, vid, locale, name)
	if err != nil {
		return nil, err
	}
	return toCMSAttributeModel(a), nil
}

func (r *mutationResolver) DeleteCMSAttributeValue(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CMSSvc.DeleteAttributeValue(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateCMSAttributeGroup(ctx context.Context, slug string, position *int32) (*model.CMSAttributeGroup, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	g, err := r.CMSSvc.CreateAttributeGroup(ctx, slug, intDeref(position, 0))
	if err != nil {
		return nil, err
	}
	return toCMSAttributeGroupModel(g), nil
}

func (r *mutationResolver) UpdateCMSAttributeGroup(ctx context.Context, id string, slug string, position int32) (*model.CMSAttributeGroup, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	g, err := r.CMSSvc.UpdateAttributeGroup(ctx, uid, slug, position)
	if err != nil {
		return nil, err
	}
	return toCMSAttributeGroupModel(g), nil
}

func (r *mutationResolver) DeleteCMSAttributeGroup(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CMSSvc.DeleteAttributeGroup(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateCMSPageType(ctx context.Context, input model.CreateCMSPageTypeInput) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	in := cms.CreatePageTypeInput{
		Slug:        input.Slug,
		Icon:        ptr.Deref(input.Icon),
		Position:    intDeref(input.Position, 0),
		Locale:      input.Locale,
		Name:        input.Name,
		Description: ptr.Deref(input.Description),
	}
	pt, err := r.CMSSvc.CreatePageType(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) UpdateCMSPageType(ctx context.Context, input model.UpdateCMSPageTypeInput) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	pt, err := r.CMSSvc.UpdatePageType(ctx, cms.UpdatePageTypeInput{
		ID: id, Icon: input.Icon, Position: input.Position,
	})
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) DeleteCMSPageType(ctx context.Context, id string) (bool, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return false, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.CMSSvc.DeletePageType(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertCMSPageTypeContent(ctx context.Context, input model.UpsertCMSPageTypeContentInput) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	uid, err := parseID(input.PageTypeID, "page type")
	if err != nil {
		return nil, err
	}
	pt, err := r.CMSSvc.UpsertPageTypeContent(ctx, uid, cms.PageTypeContentInput{
		Locale: input.Locale, Name: input.Name, Description: ptr.Deref(input.Description),
	})
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) UpdateCMSPageTypeStructure(ctx context.Context, input model.UpdateCMSPageTypeStructureInput) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	pt, err := r.CMSSvc.UpdatePageTypeStructure(ctx, cms.UpdatePageTypeStructureInput{
		ID:                id,
		RootSlug:          input.RootSlug,
		DefaultMetaTitle:  input.DefaultMetaTitle,
		DefaultSEONoindex: input.DefaultSeoNoindex,
		IsSingleton:       input.IsSingleton,
	})
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) SetCMSPageTypeChannelRoot(ctx context.Context, pageTypeID string, channelID string, rootSlug string) (*model.CMSPageType, error) {
	ptID, err := uuid.Parse(pageTypeID)
	if err != nil {
		return nil, errInvalidID("pageTypeId", err)
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "cms.types_manage", chID, nil); err != nil {
		return nil, err
	}
	pt, err := r.CMSSvc.SetPageTypeChannelRoot(ctx, ptID, chID, rootSlug)
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) SetCMSPageTypeDefaultBlocks(ctx context.Context, pageTypeID string, blocks interface{}) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	ptID, err := uuid.Parse(pageTypeID)
	if err != nil {
		return nil, errInvalidID("pageTypeId", err)
	}
	var raw json.RawMessage
	if blocks != nil {
		raw, err = json.Marshal(blocks)
		if err != nil {
			return nil, errInvalidInput("blocks", err)
		}
	}
	pt, err := r.CMSSvc.SetPageTypeDefaultBlocks(ctx, ptID, raw)
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) AssignCMSAttribute(ctx context.Context, input model.AssignCMSAttributeInput) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	pageTypeID, err := parseID(input.PageTypeID, "page type")
	if err != nil {
		return nil, err
	}
	attrID, err := parseID(input.AttributeID, "attribute")
	if err != nil {
		return nil, err
	}
	in := cms.AssignAttributeInput{
		PageTypeID:     pageTypeID,
		AttributeID:    attrID,
		Position:       intDeref(input.Position, 0),
		IsRequired:     boolDeref(input.IsRequired, false),
		ConditionValue: input.ConditionValue,
	}
	if v, err := parseUUIDOpt(input.GroupID); err != nil {
		return nil, err
	} else {
		in.GroupID = v
	}
	if v, err := parseUUIDOpt(input.ConditionAttributeID); err != nil {
		return nil, err
	} else {
		in.ConditionAttributeID = v
	}
	pt, err := r.CMSSvc.AssignAttribute(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *mutationResolver) UnassignCMSAttribute(ctx context.Context, pageTypeID string, attributeID string) (*model.CMSPageType, error) {
	if err := r.requireGlobal(ctx, "cms.types_manage"); err != nil {
		return nil, err
	}
	ptID, err := parseID(pageTypeID, "page type")
	if err != nil {
		return nil, err
	}
	atID, err := parseID(attributeID, "attribute")
	if err != nil {
		return nil, err
	}
	pt, err := r.CMSSvc.UnassignAttribute(ctx, ptID, atID)
	if err != nil {
		return nil, err
	}
	return toCMSPageTypeModel(pt), nil
}

func (r *queryResolver) CmsAttribute(ctx context.Context, slug *string, id *string) (*model.CMSAttribute, error) {
	// Schema introspection - admin-only. Reveals every field definition,
	// validation rules, and metadata; not safe to expose publicly.
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
		}
		a, err := r.CMSSvc.GetAttribute(ctx, uid)
		if err != nil {
			return nil, err
		}
		return toCMSAttributeModel(a), nil
	case slug != nil && *slug != "":
		// Service has no GetBySlug; lookup via list. Acceptable since attribute
		// counts are small in practice.
		all, err := r.CMSSvc.ListAttributes(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range all {
			if a.Slug == *slug {
				full, err := r.CMSSvc.GetAttribute(ctx, a.ID)
				if err != nil {
					return nil, err
				}
				return toCMSAttributeModel(full), nil
			}
		}
		return nil, nil
	}
	return nil, errInvalidInput("slug or id", errors.New("one is required"))
}

func (r *queryResolver) CmsAttributes(ctx context.Context) ([]*model.CMSAttribute, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	all, err := r.CMSSvc.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	if all, err = r.withCMSAttributeTranslations(ctx, all); err != nil {
		return nil, err
	}
	out := make([]*model.CMSAttribute, len(all))
	for i, a := range all {
		out[i] = toCMSAttributeModel(a)
	}
	return out, nil
}

func (r *queryResolver) CmsAttributeGroups(ctx context.Context) ([]*model.CMSAttributeGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	all, err := r.CMSSvc.ListAttributeGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CMSAttributeGroup, len(all))
	for i, g := range all {
		out[i] = toCMSAttributeGroupModel(g)
	}
	return out, nil
}

func (r *queryResolver) CmsPageType(ctx context.Context, slug *string, id *string) (*model.CMSPageType, error) {
	// Schema introspection - admin-only.
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
		}
		pt, err := r.CMSSvc.GetPageType(ctx, uid)
		if err != nil {
			return nil, err
		}
		return toCMSPageTypeModel(pt), nil
	case slug != nil && *slug != "":
		pt, err := r.CMSSvc.GetPageTypeBySlug(ctx, *slug)
		if err != nil {
			return nil, err
		}
		return toCMSPageTypeModel(pt), nil
	}
	return nil, errInvalidInput("slug or id", errors.New("one is required"))
}

func (r *queryResolver) CmsPageTypes(ctx context.Context) ([]*model.CMSPageType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "cms.view"); err != nil {
		return nil, err
	}
	all, err := r.CMSSvc.ListPageTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CMSPageType, len(all))
	for i, pt := range all {
		out[i] = toCMSPageTypeModel(pt)
	}
	return out, nil
}

func (r *Resolver) CMSPageType() CMSPageTypeResolver { return &cMSPageTypeResolver{r} }

type cMSPageTypeResolver struct{ *Resolver }
