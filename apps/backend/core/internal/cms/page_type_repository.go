package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) CreatePageType(ctx context.Context, in CreatePageTypeInput) (PageType, error) {
	var created PageType
	err := r.atomic(ctx, func(q *db.Queries) error {
		row, err := q.CreateCMSPageType(ctx, db.CreateCMSPageTypeParams{
			Slug: in.Slug, Icon: sqlnull.StringValueNonEmpty(in.Icon),
			IsSystem: in.IsSystem, Position: in.Position,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrSlugTaken
			}
			return fmt.Errorf("create page type: %w", err)
		}
		created = toPageType(row)
		if in.Locale != "" {
			c, err := q.UpsertCMSPageTypeContent(ctx, db.UpsertCMSPageTypeContentParams{
				PageTypeID:  created.ID,
				Locale:      in.Locale,
				Name:        in.Name,
				Description: sqlnull.StringValueNonEmpty(in.Description),
			})
			if err != nil {
				return fmt.Errorf("upsert page type content: %w", err)
			}
			created.AllContent = []PageTypeContent{toPageTypeContent(c)}
		}
		return nil
	})
	if err != nil {
		return PageType{}, err
	}
	return created, nil
}

func (r *postgresRepository) GetPageTypeByID(ctx context.Context, id uuid.UUID) (PageType, error) {
	row, err := r.queries.GetCMSPageTypeByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageType{}, ErrPageTypeNotFound
		}
		return PageType{}, fmt.Errorf("get page type: %w", err)
	}
	pt := toPageType(row)
	all, err := r.queries.ListCMSPageTypeContent(ctx, id)
	if err != nil {
		return PageType{}, fmt.Errorf("list page type content: %w", err)
	}
	pt.AllContent = make([]PageTypeContent, len(all))
	for i, c := range all {
		pt.AllContent[i] = toPageTypeContent(c)
	}
	roots, err := r.queries.ListCMSPageTypeChannelRoots(ctx, id)
	if err != nil {
		return PageType{}, fmt.Errorf("list page type channel roots: %w", err)
	}
	pt.ChannelRoots = make([]PageTypeChannelRoot, len(roots))
	for i, cr := range roots {
		pt.ChannelRoots[i] = PageTypeChannelRoot{ChannelID: cr.ChannelID, RootSlug: cr.RootSlug}
	}
	return pt, nil
}

func (r *postgresRepository) GetPageTypeBySlug(ctx context.Context, slug string) (PageType, error) {
	row, err := r.queries.GetCMSPageTypeBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageType{}, ErrPageTypeNotFound
		}
		return PageType{}, fmt.Errorf("get page type by slug: %w", err)
	}
	return r.GetPageTypeWithAttributes(ctx, row.ID)
}

func (r *postgresRepository) GetPageTypeWithAttributes(ctx context.Context, id uuid.UUID) (PageType, error) {
	pt, err := r.GetPageTypeByID(ctx, id)
	if err != nil {
		return PageType{}, err
	}
	rows, err := r.queries.ListCMSPageTypeAttributes(ctx, id)
	if err != nil {
		return PageType{}, fmt.Errorf("list page type attributes: %w", err)
	}
	if len(rows) == 0 {
		pt.Attributes = []PageTypeAttribute{}
		return pt, nil
	}
	pt.Attributes = make([]PageTypeAttribute, len(rows))
	valueAttrIDs := make([]uuid.UUID, 0)
	byID := make(map[uuid.UUID]*Attribute, len(rows))
	for i, row := range rows {
		var rt *RelationshipTarget
		if row.RelationshipTarget.Valid {
			t := RelationshipTarget(row.RelationshipTarget.String)
			rt = &t
		}
		attr := Attribute{
			ID:                 row.AttributeID,
			Slug:               row.AttributeSlug,
			InputType:          InputType(row.InputType),
			IsTranslatable:     row.IsTranslatable,
			IsRequiredDefault:  row.IsRequiredDefault,
			Unit:               sqlnull.StringPtr(row.Unit),
			Validation:         row.Validation,
			RelationshipTarget: rt,
			Metadata:           metadataFromJSON(row.Metadata),
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		}
		if row.DefaultValue.Valid {
			attr.DefaultValue = row.DefaultValue.RawMessage
		}
		byID[attr.ID] = &attr
		var groupID *uuid.UUID
		if row.GroupID.Valid {
			id := row.GroupID.UUID
			groupID = &id
		}
		var condAttr *uuid.UUID
		if row.ConditionAttributeID.Valid {
			cid := row.ConditionAttributeID.UUID
			condAttr = &cid
		}
		pt.Attributes[i] = PageTypeAttribute{
			Attribute:            attr,
			GroupID:              groupID,
			Position:             row.Position,
			IsRequired:           row.IsRequired,
			ConditionAttributeID: condAttr,
			ConditionValue:       sqlnull.StringPtr(row.ConditionValue),
		}
		if attr.InputType.AcceptsValues() {
			valueAttrIDs = append(valueAttrIDs, attr.ID)
		}
	}
	if len(valueAttrIDs) > 0 {
		vrows, err := r.queries.ListCMSAttributeValuesByAttributeIDs(ctx, valueAttrIDs)
		if err != nil {
			return PageType{}, fmt.Errorf("list attribute values: %w", err)
		}
		valuesByAttr := make(map[uuid.UUID][]AttributeValue)
		for _, vr := range vrows {
			valuesByAttr[vr.AttributeID] = append(valuesByAttr[vr.AttributeID], toAttributeValue(vr))
		}
		for i := range pt.Attributes {
			if vs, ok := valuesByAttr[pt.Attributes[i].Attribute.ID]; ok {
				pt.Attributes[i].Attribute.Values = vs
			}
		}
	}
	return pt, nil
}

func (r *postgresRepository) ListPageTypes(ctx context.Context) ([]PageType, error) {
	rows, err := r.queries.ListCMSPageTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list page types: %w", err)
	}
	out := make([]PageType, len(rows))
	for i, row := range rows {
		out[i] = toPageType(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdatePageType(ctx context.Context, in UpdatePageTypeInput) (PageType, error) {
	current, err := r.queries.GetCMSPageTypeByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageType{}, ErrPageTypeNotFound
		}
		return PageType{}, fmt.Errorf("load page type: %w", err)
	}
	icon := current.Icon
	if in.Icon != nil {
		icon = sqlnull.StringValueNonEmpty(*in.Icon)
	}
	position := current.Position
	if in.Position != nil {
		position = *in.Position
	}
	row, err := r.queries.UpdateCMSPageType(ctx, db.UpdateCMSPageTypeParams{
		ID: in.ID, Icon: icon, IsSystem: current.IsSystem, Position: position,
	})
	if err != nil {
		return PageType{}, fmt.Errorf("update page type: %w", err)
	}
	return toPageType(row), nil
}

func (r *postgresRepository) DeletePageType(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSPageType(ctx, id); err != nil {
		return fmt.Errorf("delete page type: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertPageTypeContent(ctx context.Context, pageTypeID uuid.UUID, in PageTypeContentInput) (PageTypeContent, error) {
	row, err := r.queries.UpsertCMSPageTypeContent(ctx, db.UpsertCMSPageTypeContentParams{
		PageTypeID: pageTypeID, Locale: in.Locale, Name: in.Name,
		Description: sqlnull.StringValueNonEmpty(in.Description),
	})
	if err != nil {
		return PageTypeContent{}, fmt.Errorf("upsert page type content: %w", err)
	}
	return toPageTypeContent(row), nil
}

func (r *postgresRepository) CountPagesByType(ctx context.Context, pageTypeID uuid.UUID) (int64, error) {
	return r.queries.CountCMSPagesByType(ctx, pageTypeID)
}

func (r *postgresRepository) UpdatePageTypeStructure(ctx context.Context, in ResolvedPageTypeStructure) (PageType, error) {
	row, err := r.queries.UpdateCMSPageTypeStructure(ctx, db.UpdateCMSPageTypeStructureParams{
		ID:                in.ID,
		RootSlug:          sqlnull.StringValueNonEmpty(in.RootSlug),
		DefaultMetaTitle:  sqlnull.StringValueNonEmpty(in.DefaultMetaTitle),
		DefaultSeoNoindex: in.DefaultSEONoindex,
		IsSingleton:       in.IsSingleton,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageType{}, ErrPageTypeNotFound
		}
		return PageType{}, fmt.Errorf("update page type structure: %w", err)
	}
	return toPageType(row), nil
}

func (r *postgresRepository) SetPageTypeDefaultBlocks(ctx context.Context, id uuid.UUID, blocks json.RawMessage) error {
	return r.queries.SetCMSPageTypeDefaultBlocks(ctx, db.SetCMSPageTypeDefaultBlocksParams{
		ID:            id,
		DefaultBlocks: pqtype.NullRawMessage{RawMessage: blocks, Valid: len(blocks) > 0},
	})
}

func (r *postgresRepository) UpsertPageTypeChannelRoot(ctx context.Context, pageTypeID, channelID uuid.UUID, rootSlug string) error {
	err := r.queries.UpsertCMSPageTypeChannelRoot(ctx, db.UpsertCMSPageTypeChannelRootParams{
		PageTypeID: pageTypeID, ChannelID: channelID, RootSlug: rootSlug,
	})
	if isForeignKeyViolation(err) {
		return ErrChannelNotFound
	}
	return err
}

func (r *postgresRepository) DeletePageTypeChannelRoot(ctx context.Context, pageTypeID, channelID uuid.UUID) error {
	return r.queries.DeleteCMSPageTypeChannelRoot(ctx, db.DeleteCMSPageTypeChannelRootParams{
		PageTypeID: pageTypeID, ChannelID: channelID,
	})
}

func (r *postgresRepository) AssignAttributeToPageType(ctx context.Context, in AssignAttributeInput) error {
	var groupID uuid.NullUUID
	if in.GroupID != nil {
		groupID = uuid.NullUUID{UUID: *in.GroupID, Valid: true}
	}
	var condAttr uuid.NullUUID
	if in.ConditionAttributeID != nil {
		condAttr = uuid.NullUUID{UUID: *in.ConditionAttributeID, Valid: true}
	}
	if err := r.queries.AssignCMSAttributeToPageType(ctx, db.AssignCMSAttributeToPageTypeParams{
		PageTypeID:           in.PageTypeID,
		AttributeID:          in.AttributeID,
		GroupID:              groupID,
		Position:             in.Position,
		IsRequired:           in.IsRequired,
		ConditionAttributeID: condAttr,
		ConditionValue:       sqlnull.StringNonEmpty(in.ConditionValue),
	}); err != nil {
		if isForeignKeyViolation(err) {
			return ErrAttributeGroupNotFound
		}
		return fmt.Errorf("assign attribute: %w", err)
	}
	return nil
}

func toPageType(row db.CmsPageType) PageType {
	pt := PageType{
		ID: row.ID, Slug: row.Slug, Icon: sqlnull.StringPtr(row.Icon),
		IsSystem: row.IsSystem, Position: row.Position,
		RootSlug:          sqlnull.StringValue(row.RootSlug),
		DefaultMetaTitle:  sqlnull.StringValue(row.DefaultMetaTitle),
		DefaultSEONoindex: row.DefaultSeoNoindex,
		IsSingleton:       row.IsSingleton,
		CreatedAt:         row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.DefaultBlocks.Valid {
		pt.DefaultBlocks = row.DefaultBlocks.RawMessage
	}
	return pt
}

func toPageTypeContent(row db.CmsPageTypeContent) PageTypeContent {
	return PageTypeContent{
		Locale: row.Locale, Name: row.Name,
		Description: sqlnull.StringPtr(row.Description),
		UpdatedAt:   row.UpdatedAt,
	}
}
