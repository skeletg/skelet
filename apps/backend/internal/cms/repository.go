package cms

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

	"github.com/skeletg/skelet/apps/backend/internal/blockkit"
	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *tx.Manager
}

func NewRepository(queries *db.Queries, txMgr *tx.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) *postgresRepository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

// ==================== Attributes ====================

func (r *postgresRepository) CreateAttribute(ctx context.Context, in CreateAttributeInput) (Attribute, error) {
	var created Attribute
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		var rt sql.NullString
		if in.RelationshipTarget != nil {
			rt = sql.NullString{String: string(*in.RelationshipTarget), Valid: true}
		}
		row, err := q.CreateCMSAttribute(ctx, db.CreateCMSAttributeParams{
			Slug:               in.Slug,
			InputType:          string(in.InputType),
			IsTranslatable:     in.IsTranslatable,
			IsRequiredDefault:  in.IsRequiredDefault,
			Unit:               nullableString(in.Unit),
			Validation:         emptyJSON(in.Validation),
			RelationshipTarget: rt,
			Metadata:           metadataToJSON(in.Metadata),
			DefaultValue:       pqtype.NullRawMessage{RawMessage: in.DefaultValue, Valid: len(in.DefaultValue) > 0},
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrSlugTaken
			}
			return fmt.Errorf("create cms attribute: %w", err)
		}
		created = toAttribute(row)
		for _, v := range in.Values {
			vrow, err := q.CreateCMSAttributeValue(ctx, db.CreateCMSAttributeValueParams{
				AttributeID: created.ID,
				Slug:        v.Slug,
				Position:    v.Position,
			})
			if err != nil {
				if isUniqueViolation(err) {
					return fmt.Errorf("attribute value %q: %w", v.Slug, ErrSlugTaken)
				}
				return fmt.Errorf("create attribute value: %w", err)
			}
			created.Values = append(created.Values, toAttributeValue(vrow))
		}
		return nil
	})
	if err != nil {
		return Attribute{}, err
	}
	return created, nil
}

func (r *postgresRepository) GetAttributeByID(ctx context.Context, id uuid.UUID) (Attribute, error) {
	row, err := r.queries.GetCMSAttributeByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attribute{}, ErrAttributeNotFound
		}
		return Attribute{}, fmt.Errorf("get cms attribute: %w", err)
	}
	a := toAttribute(row)
	if a.InputType.AcceptsValues() {
		vals, err := r.queries.ListCMSAttributeValues(ctx, id)
		if err != nil {
			return Attribute{}, fmt.Errorf("list values: %w", err)
		}
		a.Values = make([]AttributeValue, len(vals))
		for i, v := range vals {
			a.Values[i] = toAttributeValue(v)
		}
		r.hydrateValueContent(ctx, a.Values)
	}
	all, err := r.queries.ListCMSAttributeContent(ctx, id)
	if err == nil {
		a.AllContent = make([]AttributeContent, len(all))
		for i, c := range all {
			a.AllContent[i] = toAttributeContent(c)
		}
	}
	return a, nil
}

func (r *postgresRepository) GetAttributeBySlug(ctx context.Context, slug string) (Attribute, error) {
	row, err := r.queries.GetCMSAttributeBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attribute{}, ErrAttributeNotFound
		}
		return Attribute{}, fmt.Errorf("get cms attribute by slug: %w", err)
	}
	return r.GetAttributeByID(ctx, row.ID)
}

func (r *postgresRepository) ListAttributes(ctx context.Context) ([]Attribute, error) {
	rows, err := r.queries.ListCMSAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cms attributes: %w", err)
	}
	out := make([]Attribute, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	byID := make(map[uuid.UUID]*Attribute, len(rows))
	for i, row := range rows {
		out[i] = toAttribute(row)
		byID[out[i].ID] = &out[i]
		if out[i].InputType.AcceptsValues() {
			ids = append(ids, out[i].ID)
			out[i].Values = []AttributeValue{}
		}
	}
	if len(ids) > 0 {
		vrows, err := r.queries.ListCMSAttributeValuesByAttributeIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("list attribute values: %w", err)
		}
		for _, vr := range vrows {
			if a, ok := byID[vr.AttributeID]; ok {
				a.Values = append(a.Values, toAttributeValue(vr))
			}
		}
	}
	return out, nil
}

func (r *postgresRepository) UpdateAttribute(ctx context.Context, in UpdateAttributeInput) (Attribute, error) {
	current, err := r.queries.GetCMSAttributeByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attribute{}, ErrAttributeNotFound
		}
		return Attribute{}, fmt.Errorf("load cms attribute: %w", err)
	}
	isTranslatable := current.IsTranslatable
	if in.IsTranslatable != nil {
		isTranslatable = *in.IsTranslatable
	}
	isRequiredDefault := current.IsRequiredDefault
	if in.IsRequiredDefault != nil {
		isRequiredDefault = *in.IsRequiredDefault
	}
	unit := current.Unit
	if in.Unit != nil {
		unit = nullableString(*in.Unit)
	}
	validation := current.Validation
	if in.Validation != nil {
		validation = emptyJSON(in.Validation)
	}
	rt := current.RelationshipTarget
	if in.RelationshipTarget != nil {
		rt = sql.NullString{String: string(*in.RelationshipTarget), Valid: true}
	}
	metadata := current.Metadata
	if in.Metadata != nil {
		metadata = metadataToJSON(in.Metadata)
	}
	defaultValue := current.DefaultValue
	if in.DefaultValue != nil {
		defaultValue = pqtype.NullRawMessage{RawMessage: in.DefaultValue, Valid: len(in.DefaultValue) > 0}
	}
	row, err := r.queries.UpdateCMSAttribute(ctx, db.UpdateCMSAttributeParams{
		ID:                 in.ID,
		IsTranslatable:     isTranslatable,
		IsRequiredDefault:  isRequiredDefault,
		Unit:               unit,
		Validation:         validation,
		RelationshipTarget: rt,
		Metadata:           metadata,
		DefaultValue:       defaultValue,
	})
	if err != nil {
		return Attribute{}, fmt.Errorf("update cms attribute: %w", err)
	}
	return r.GetAttributeByID(ctx, row.ID)
}

func (r *postgresRepository) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSAttribute(ctx, id); err != nil {
		return fmt.Errorf("delete cms attribute: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPageTypesUsingAttribute(ctx context.Context, attributeID uuid.UUID) ([]uuid.UUID, error) {
	return r.queries.ListPageTypesUsingAttribute(ctx, attributeID)
}

func (r *postgresRepository) UpsertAttributeContent(ctx context.Context, attributeID uuid.UUID, in AttributeContentInput) (AttributeContent, error) {
	row, err := r.queries.UpsertCMSAttributeContent(ctx, db.UpsertCMSAttributeContentParams{
		AttributeID: attributeID,
		Locale:      in.Locale,
		Name:        in.Name,
		HelperText:  nullableString(in.HelperText),
		Placeholder: nullableString(in.Placeholder),
	})
	if err != nil {
		return AttributeContent{}, fmt.Errorf("upsert attribute content: %w", err)
	}
	return toAttributeContent(row), nil
}

func (r *postgresRepository) ListAttributeContent(ctx context.Context, attributeID uuid.UUID) ([]AttributeContent, error) {
	rows, err := r.queries.ListCMSAttributeContent(ctx, attributeID)
	if err != nil {
		return nil, err
	}
	out := make([]AttributeContent, len(rows))
	for i, row := range rows {
		out[i] = toAttributeContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteAttributeContent(ctx context.Context, attributeID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteCMSAttributeContent(ctx, db.DeleteCMSAttributeContentParams{
		AttributeID: attributeID, Locale: locale,
	})
	return err
}

// ==================== Attribute Values ====================

func (r *postgresRepository) CreateAttributeValue(ctx context.Context, attributeID uuid.UUID, in AttributeValueInput) (AttributeValue, error) {
	row, err := r.queries.CreateCMSAttributeValue(ctx, db.CreateCMSAttributeValueParams{
		AttributeID: attributeID, Slug: in.Slug, Position: in.Position,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return AttributeValue{}, ErrSlugTaken
		}
		return AttributeValue{}, fmt.Errorf("create attribute value: %w", err)
	}
	return toAttributeValue(row), nil
}

func (r *postgresRepository) UpdateAttributeValue(ctx context.Context, id uuid.UUID, in AttributeValueInput) (AttributeValue, error) {
	row, err := r.queries.UpdateCMSAttributeValue(ctx, db.UpdateCMSAttributeValueParams{
		ID: id, Slug: in.Slug, Position: in.Position,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttributeValue{}, ErrAttributeValueNotFound
		}
		if isUniqueViolation(err) {
			return AttributeValue{}, ErrSlugTaken
		}
		return AttributeValue{}, fmt.Errorf("update attribute value: %w", err)
	}
	return toAttributeValue(row), nil
}

func (r *postgresRepository) DeleteAttributeValue(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSAttributeValue(ctx, id); err != nil {
		return fmt.Errorf("delete attribute value: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListAttributeValues(ctx context.Context, attributeID uuid.UUID) ([]AttributeValue, error) {
	rows, err := r.queries.ListCMSAttributeValues(ctx, attributeID)
	if err != nil {
		return nil, fmt.Errorf("list attribute values: %w", err)
	}
	out := make([]AttributeValue, len(rows))
	for i, row := range rows {
		out[i] = toAttributeValue(row)
	}
	return out, nil
}

// ==================== Attribute Groups ====================

func (r *postgresRepository) CreateAttributeGroup(ctx context.Context, slug string, position int32) (AttributeGroup, error) {
	row, err := r.queries.CreateCMSAttributeGroup(ctx, db.CreateCMSAttributeGroupParams{
		Slug: slug, Position: position,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return AttributeGroup{}, ErrSlugTaken
		}
		return AttributeGroup{}, fmt.Errorf("create attribute group: %w", err)
	}
	return toAttributeGroup(row), nil
}

func (r *postgresRepository) GetAttributeGroupByID(ctx context.Context, id uuid.UUID) (AttributeGroup, error) {
	row, err := r.queries.GetCMSAttributeGroupByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttributeGroup{}, ErrAttributeGroupNotFound
		}
		return AttributeGroup{}, fmt.Errorf("get attribute group: %w", err)
	}
	return toAttributeGroup(row), nil
}

func (r *postgresRepository) ListAttributeGroups(ctx context.Context) ([]AttributeGroup, error) {
	rows, err := r.queries.ListCMSAttributeGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list attribute groups: %w", err)
	}
	out := make([]AttributeGroup, len(rows))
	for i, row := range rows {
		out[i] = toAttributeGroup(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateAttributeGroup(ctx context.Context, id uuid.UUID, slug string, position int32) (AttributeGroup, error) {
	row, err := r.queries.UpdateCMSAttributeGroup(ctx, db.UpdateCMSAttributeGroupParams{
		ID: id, Slug: slug, Position: position,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttributeGroup{}, ErrAttributeGroupNotFound
		}
		return AttributeGroup{}, fmt.Errorf("update attribute group: %w", err)
	}
	return toAttributeGroup(row), nil
}

func (r *postgresRepository) DeleteAttributeGroup(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSAttributeGroup(ctx, id); err != nil {
		return fmt.Errorf("delete attribute group: %w", err)
	}
	return nil
}

// ==================== Page Types ====================

func (r *postgresRepository) CreatePageType(ctx context.Context, in CreatePageTypeInput) (PageType, error) {
	var created PageType
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateCMSPageType(ctx, db.CreateCMSPageTypeParams{
			Slug: in.Slug, Icon: nullableString(in.Icon),
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
				Description: nullableString(in.Description),
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
	if err == nil {
		pt.AllContent = make([]PageTypeContent, len(all))
		for i, c := range all {
			pt.AllContent[i] = toPageTypeContent(c)
		}
	}
	roots, err := r.queries.ListCMSPageTypeChannelRoots(ctx, id)
	if err == nil {
		pt.ChannelRoots = make([]PageTypeChannelRoot, len(roots))
		for i, cr := range roots {
			pt.ChannelRoots[i] = PageTypeChannelRoot{ChannelID: cr.ChannelID, RootSlug: cr.RootSlug}
		}
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
			Unit:               nullStringPtr(row.Unit),
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
			ConditionValue:       nullStringPtr(row.ConditionValue),
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
		icon = nullableString(*in.Icon)
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
		Description: nullableString(in.Description),
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
		RootSlug:          nullableString(in.RootSlug),
		DefaultMetaTitle:  nullableString(in.DefaultMetaTitle),
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
	return r.queries.UpsertCMSPageTypeChannelRoot(ctx, db.UpsertCMSPageTypeChannelRootParams{
		PageTypeID: pageTypeID, ChannelID: channelID, RootSlug: rootSlug,
	})
}

func (r *postgresRepository) DeletePageTypeChannelRoot(ctx context.Context, pageTypeID, channelID uuid.UUID) error {
	return r.queries.DeleteCMSPageTypeChannelRoot(ctx, db.DeleteCMSPageTypeChannelRootParams{
		PageTypeID: pageTypeID, ChannelID: channelID,
	})
}

func (r *postgresRepository) ListPagesByType(ctx context.Context, pageTypeID uuid.UUID) ([]Page, error) {
	rows, err := r.queries.ListCMSPagesByType(ctx, pageTypeID)
	if err != nil {
		return nil, fmt.Errorf("list pages by type: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdatePagePaths(ctx context.Context, id uuid.UUID, basePath, urlPath string) error {
	return r.queries.UpdateCMSPagePaths(ctx, db.UpdateCMSPagePathsParams{ID: id, BasePath: basePath, UrlPath: urlPath})
}

func (r *postgresRepository) UpdatePageExperimentKey(ctx context.Context, id uuid.UUID, key string) error {
	return r.queries.UpdateCMSPageExperimentKey(ctx, db.UpdateCMSPageExperimentKeyParams{ID: id, ExperimentKey: key})
}

func (r *postgresRepository) ListPageVariantContent(ctx context.Context, pageID uuid.UUID, locale string) ([]PageVariant, error) {
	rows, err := r.queries.ListCMSPageVariantContent(ctx, db.ListCMSPageVariantContentParams{PageID: pageID, Locale: locale})
	if err != nil {
		return nil, fmt.Errorf("list page variant content: %w", err)
	}
	out := make([]PageVariant, len(rows))
	for i, row := range rows {
		var body json.RawMessage
		if row.Body.Valid {
			body = row.Body.RawMessage
		}
		out[i] = PageVariant{VariantKey: row.VariantKey, Body: body}
	}
	return out, nil
}

func (r *postgresRepository) UpsertPageVariantContent(ctx context.Context, pageID uuid.UUID, locale, variantKey string, body json.RawMessage) error {
	return r.queries.UpsertCMSPageVariantContent(ctx, db.UpsertCMSPageVariantContentParams{
		PageID: pageID, Locale: locale, VariantKey: variantKey,
		Body: pqtype.NullRawMessage{RawMessage: body, Valid: len(body) > 0},
	})
}

func (r *postgresRepository) DeletePageVariantContent(ctx context.Context, pageID uuid.UUID, locale, variantKey string) error {
	return r.queries.DeleteCMSPageVariantContent(ctx, db.DeleteCMSPageVariantContentParams{
		PageID: pageID, Locale: locale, VariantKey: variantKey,
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
		ConditionValue:       nullableStringPtr(in.ConditionValue),
	}); err != nil {
		return fmt.Errorf("assign attribute: %w", err)
	}
	return nil
}

func nullableStringPtr(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func (r *postgresRepository) UnassignAttributeFromPageType(ctx context.Context, pageTypeID, attributeID uuid.UUID) error {
	_, err := r.queries.UnassignCMSAttributeFromPageType(ctx, db.UnassignCMSAttributeFromPageTypeParams{
		PageTypeID: pageTypeID, AttributeID: attributeID,
	})
	if err != nil {
		return fmt.Errorf("unassign attribute: %w", err)
	}
	return nil
}

// ==================== Pages ====================

func (r *postgresRepository) CreatePage(ctx context.Context, in CreatePageInput) (Page, error) {
	row, err := r.queries.CreateCMSPage(ctx, db.CreateCMSPageParams{
		PageTypeID:      in.PageTypeID,
		ParentID:        nullUUID(in.ParentID),
		Slug:            in.Slug,
		BasePath:        in.Slug, // overwritten by service after base/url paths are computed
		UrlPath:         in.Slug,
		Position:        in.Position,
		IsPublished:     in.IsPublished,
		SeoNoindex:      in.SEONoindex,
		PublishedAt:     nullTime(in.PublishedAt),
		Data:            emptyJSON(in.Data),
		Metadata:        metadataToJSON(in.Metadata),
		PrivateMetadata: metadataToJSON(in.PrivateMetadata),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Page{}, ErrSlugTaken
		}
		return Page{}, fmt.Errorf("create page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) GetPageByID(ctx context.Context, id uuid.UUID) (Page, error) {
	row, err := r.queries.GetCMSPageByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get page: %w", err)
	}
	p := toPage(row)
	all, err := r.queries.ListCMSPageContent(ctx, id)
	if err == nil {
		p.AllContent = make([]PageContent, len(all))
		for i, c := range all {
			p.AllContent[i] = toPageContent(c)
		}
	}
	sels, err := r.queries.ListCMSPageAttributeValues(ctx, id)
	if err == nil {
		p.Selections = make([]PageAttributeSelection, len(sels))
		for i, s := range sels {
			p.Selections[i] = PageAttributeSelection{AttributeID: s.AttributeID, ValueID: s.ValueID}
		}
	}
	rels, err := r.queries.ListCMSPageRelationships(ctx, id)
	if err == nil {
		p.Relationships = make([]PageRelationship, len(rels))
		for i, rl := range rels {
			p.Relationships[i] = PageRelationship{
				AttributeID: rl.AttributeID,
				TargetType:  RelationshipTarget(rl.TargetType),
				TargetID:    rl.TargetID,
				Position:    rl.Position,
			}
		}
	}
	listings, err := r.queries.ListCMSPageChannelListings(ctx, id)
	if err == nil {
		p.ChannelListings = make([]PageChannelListing, len(listings))
		for i, l := range listings {
			p.ChannelListings[i] = toPageChannelListing(l)
		}
	}
	return p, nil
}

func (r *postgresRepository) GetPageBySlug(ctx context.Context, slug string) (Page, error) {
	row, err := r.queries.GetCMSPageBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get page by slug: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) GetPageByURLPath(ctx context.Context, urlPath string) (Page, error) {
	row, err := r.queries.GetCMSPageByURLPath(ctx, urlPath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get page by url_path: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) ListPages(ctx context.Context, filter PageListFilter) ([]Page, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var pageTypeID, parentID uuid.NullUUID
	if filter.PageTypeID != nil {
		pageTypeID = uuid.NullUUID{UUID: *filter.PageTypeID, Valid: true}
	}
	if filter.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *filter.ParentID, Valid: true}
	}
	// Storefront paths: when ChannelID is supplied, the join goes against cms_page_channel_listings so only pages with an active listing on
	// that store come back. cms_pages.is_published is also required (the query enforces it), so OnlyPublished is implicit
	if filter.ChannelID != nil {
		rows, err := r.queries.ListPublishedCMSPagesForChannel(ctx, db.ListPublishedCMSPagesForChannelParams{
			ChannelID:  *filter.ChannelID,
			Limit:      limit,
			Offset:     filter.Offset,
			PageTypeID: pageTypeID,
			ParentID:   parentID,
		})
		if err != nil {
			return nil, fmt.Errorf("list pages for store: %w", err)
		}
		out := make([]Page, len(rows))
		for i, row := range rows {
			out[i] = toPage(row)
		}
		return out, nil
	}
	rows, err := r.queries.ListCMSPages(ctx, db.ListCMSPagesParams{
		Limit:         limit,
		Offset:        filter.Offset,
		PageTypeID:    pageTypeID,
		ParentID:      parentID,
		OnlyPublished: sql.NullBool{Bool: filter.OnlyPublished, Valid: true},
		WorkflowState: nullableStringPtr(filter.WorkflowState),
	})
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

// PageSlugExists reports whether any page already uses this slug (used to pick
// a unique slug when cloning)
func (r *postgresRepository) PageSlugExists(ctx context.Context, slug string) (bool, error) {
	exists, err := r.queries.CMSPageSlugExists(ctx, slug)
	if err != nil {
		return false, fmt.Errorf("page slug exists: %w", err)
	}
	return exists, nil
}

// ListPagesSearch runs the admin full-text search over a locale's content, ranked by relevance - Filters mirror
// ListPages (page type / parent / published / workflow) - Pages with no content row in the requested locale don't match
func (r *postgresRepository) ListPagesSearch(ctx context.Context, filter PageListFilter, locale, query string) ([]Page, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var pageTypeID, parentID uuid.NullUUID
	if filter.PageTypeID != nil {
		pageTypeID = uuid.NullUUID{UUID: *filter.PageTypeID, Valid: true}
	}
	if filter.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *filter.ParentID, Valid: true}
	}
	rows, err := r.queries.ListCMSPagesSearch(ctx, db.ListCMSPagesSearchParams{
		Locale:        locale,
		Q:             query,
		Limit:         limit,
		Offset:        filter.Offset,
		PageTypeID:    pageTypeID,
		ParentID:      parentID,
		OnlyPublished: sql.NullBool{Bool: filter.OnlyPublished, Valid: true},
		WorkflowState: nullableStringPtr(filter.WorkflowState),
	})
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAllPages(ctx context.Context) ([]Page, error) {
	rows, err := r.queries.ListAllCMSPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPageChildren(ctx context.Context, parentID uuid.UUID) ([]Page, error) {
	rows, err := r.queries.ListCMSPageChildren(ctx, uuid.NullUUID{UUID: parentID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list page children: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdatePage(ctx context.Context, in UpdatePageInput) (Page, error) {
	current, err := r.queries.GetCMSPageByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("load page: %w", err)
	}
	parentID := current.ParentID
	if in.ClearParent {
		parentID = uuid.NullUUID{}
	} else if in.ParentID != nil {
		parentID = uuid.NullUUID{UUID: *in.ParentID, Valid: true}
	}
	slug := current.Slug
	if in.Slug != nil {
		slug = *in.Slug
	}
	position := current.Position
	if in.Position != nil {
		position = *in.Position
	}
	isPublished := current.IsPublished
	if in.IsPublished != nil {
		isPublished = *in.IsPublished
	}
	seoNoindex := current.SeoNoindex
	if in.SEONoindex != nil {
		seoNoindex = *in.SEONoindex
	}
	publishedAt := current.PublishedAt
	if in.ClearPublished {
		publishedAt = sql.NullTime{}
	} else if in.PublishedAt != nil {
		publishedAt = sql.NullTime{Time: *in.PublishedAt, Valid: true}
	}
	data := current.Data
	if in.Data != nil {
		data = emptyJSON(in.Data)
	}
	metadata := current.Metadata
	if in.Metadata != nil {
		metadata = metadataToJSON(in.Metadata)
	}
	privateMetadata := current.PrivateMetadata
	if in.PrivateMetadata != nil {
		privateMetadata = metadataToJSON(in.PrivateMetadata)
	}
	tags := current.Tags
	if in.Tags != nil {
		tags = normalizeTags(*in.Tags)
	}
	row, err := r.queries.UpdateCMSPage(ctx, db.UpdateCMSPageParams{
		ID:              in.ID,
		ParentID:        parentID,
		Slug:            slug,
		Position:        position,
		IsPublished:     isPublished,
		SeoNoindex:      seoNoindex,
		PublishedAt:     publishedAt,
		Data:            data,
		Metadata:        metadata,
		PrivateMetadata: privateMetadata,
		Tags:            tags,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Page{}, ErrSlugTaken
		}
		return Page{}, fmt.Errorf("update page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) SetPageTags(ctx context.Context, id uuid.UUID, tags []string) error {
	if err := r.queries.SetCMSPageTags(ctx, db.SetCMSPageTagsParams{ID: id, Tags: tags}); err != nil {
		return fmt.Errorf("set page tags: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpdatePageURLPath(ctx context.Context, id uuid.UUID, urlPath string) error {
	if err := r.queries.UpdateCMSPageURLPath(ctx, db.UpdateCMSPageURLPathParams{
		ID: id, UrlPath: urlPath,
	}); err != nil {
		return fmt.Errorf("update page url_path: %w", err)
	}
	return nil
}

func (r *postgresRepository) SetPagePublished(ctx context.Context, id uuid.UUID, isPublished bool, publishedAt *time.Time) (Page, error) {
	row, err := r.queries.SetCMSPagePublished(ctx, db.SetCMSPagePublishedParams{
		ID: id, IsPublished: isPublished, PublishedAt: nullTime(publishedAt),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("set page published: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) DeletePage(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSPage(ctx, id); err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertPageContent(ctx context.Context, pageID uuid.UUID, in PageContentInput) (PageContent, error) {
	row, err := r.queries.UpsertCMSPageContent(ctx, db.UpsertCMSPageContentParams{
		PageID:          pageID,
		Locale:          in.Locale,
		Title:           in.Title,
		Body:            pqtype.NullRawMessage{RawMessage: in.Body, Valid: len(in.Body) > 0},
		MetaTitle:       nullableString(in.MetaTitle),
		MetaDescription: nullableString(in.MetaDescription),
		MetaKeywords:    nullableString(in.MetaKeywords),
		Data:            emptyJSON(in.Data),
	})
	if err != nil {
		return PageContent{}, fmt.Errorf("upsert page content: %w", err)
	}
	// Repopulate the full-text search index for this (page, locale) - Best-effort
	// - a content save must not fail because indexing did; a stale vector is
	// corrected on the next save or by the backfill datapatch
	if err := r.queries.SetCMSPageContentSearchVector(ctx, db.SetCMSPageContentSearchVectorParams{
		PageID: pageID,
		Locale: in.Locale,
		Doc:    buildSearchText(in.Title, in.MetaTitle, in.MetaDescription, in.MetaKeywords, in.Body),
	}); err != nil {
		return toPageContent(row), nil
	}
	return toPageContent(row), nil
}

// buildSearchText assembles the indexable text for a page-content row: title + meta + the human-readable text extracted
// from the block body - Non-block freeform bodies contribute no block text
func buildSearchText(title, metaTitle, metaDescription, metaKeywords string, body json.RawMessage) string {
	parts := []string{title, metaTitle, metaDescription, metaKeywords}
	if doc, ok := blockkit.ParseDocument(body); ok {
		parts = append(parts, blockkit.PlainText(doc))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (r *postgresRepository) GetPageContent(ctx context.Context, pageID uuid.UUID, locale string) (PageContent, error) {
	row, err := r.queries.GetCMSPageContent(ctx, db.GetCMSPageContentParams{
		PageID: pageID, Locale: locale,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageContent{}, ErrPageContentNotFound
		}
		return PageContent{}, fmt.Errorf("get page content: %w", err)
	}
	return toPageContent(row), nil
}

func (r *postgresRepository) ListPageContent(ctx context.Context, pageID uuid.UUID) ([]PageContent, error) {
	rows, err := r.queries.ListCMSPageContent(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page content: %w", err)
	}
	out := make([]PageContent, len(rows))
	for i, row := range rows {
		out[i] = toPageContent(row)
	}
	return out, nil
}

// ListPageContentByScope batch-loads one locale's content for many pages in a single query, keyed by page id - Used by
// ListPages so list views serve translated content without an N+1 (mirrors blog's ListPostContentByScope)
func (r *postgresRepository) ListPageContentByScope(ctx context.Context, pageIDs []uuid.UUID, locale string) (map[uuid.UUID]PageContent, error) {
	if len(pageIDs) == 0 {
		return map[uuid.UUID]PageContent{}, nil
	}
	rows, err := r.queries.ListCMSPageContentByScope(ctx, db.ListCMSPageContentByScopeParams{
		PageIds: pageIDs,
		Locale:  locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list page content by scope: %w", err)
	}
	out := make(map[uuid.UUID]PageContent, len(rows))
	for _, row := range rows {
		out[row.PageID] = toPageContent(row)
	}
	return out, nil
}

func (r *postgresRepository) DeletePageContent(ctx context.Context, pageID uuid.UUID, locale string) error {
	_, err := r.queries.DeleteCMSPageContent(ctx, db.DeleteCMSPageContentParams{
		PageID: pageID, Locale: locale,
	})
	return err
}

// ==================== Selections + Relationships ====================

func (r *postgresRepository) SetPageSelections(ctx context.Context, pageID, attributeID uuid.UUID, valueIDs []uuid.UUID) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.ClearCMSPageAttributeValuesForAttribute(ctx, db.ClearCMSPageAttributeValuesForAttributeParams{
			PageID: pageID, AttributeID: attributeID,
		}); err != nil {
			return err
		}
		for _, vid := range valueIDs {
			if err := q.SetCMSPageAttributeValue(ctx, db.SetCMSPageAttributeValueParams{
				PageID: pageID, AttributeID: attributeID, ValueID: vid,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *postgresRepository) ListPageSelections(ctx context.Context, pageID uuid.UUID) ([]PageAttributeSelection, error) {
	rows, err := r.queries.ListCMSPageAttributeValues(ctx, pageID)
	if err != nil {
		return nil, err
	}
	out := make([]PageAttributeSelection, len(rows))
	for i, s := range rows {
		out[i] = PageAttributeSelection{AttributeID: s.AttributeID, ValueID: s.ValueID}
	}
	return out, nil
}

func (r *postgresRepository) SetPageRelationships(ctx context.Context, pageID, attributeID uuid.UUID, refs []PageRelationship) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.ClearCMSPageRelationshipsForAttribute(ctx, db.ClearCMSPageRelationshipsForAttributeParams{
			PageID: pageID, AttributeID: attributeID,
		}); err != nil {
			return err
		}
		for _, rl := range refs {
			if err := q.SetCMSPageRelationship(ctx, db.SetCMSPageRelationshipParams{
				PageID: pageID, AttributeID: attributeID,
				TargetType: string(rl.TargetType), TargetID: rl.TargetID, Position: rl.Position,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *postgresRepository) ListPageRelationships(ctx context.Context, pageID uuid.UUID) ([]PageRelationship, error) {
	rows, err := r.queries.ListCMSPageRelationships(ctx, pageID)
	if err != nil {
		return nil, err
	}
	out := make([]PageRelationship, len(rows))
	for i, rl := range rows {
		out[i] = PageRelationship{
			AttributeID: rl.AttributeID,
			TargetType:  RelationshipTarget(rl.TargetType),
			TargetID:    rl.TargetID,
			Position:    rl.Position,
		}
	}
	return out, nil
}

// ==================== Store listings ====================

func (r *postgresRepository) UpsertPageChannelListing(ctx context.Context, in SetPageChannelListingInput) (PageChannelListing, error) {
	row, err := r.queries.UpsertCMSPageChannelListing(ctx, db.UpsertCMSPageChannelListingParams{
		PageID:      in.PageID,
		ChannelID:   in.ChannelID,
		IsPublished: in.IsPublished,
		SeoNoindex:  in.SEONoindex,
		IsHomepage:  in.IsHomepage,
	})
	if err != nil {
		return PageChannelListing{}, fmt.Errorf("upsert cms page store listing: %w", err)
	}
	return toPageChannelListing(row), nil
}

// ClearChannelHomepage unsets the homepage flag on every page of a channel except keepPageID - Run before flagging a
// new homepage so the per-channel partial unique index isn't violated mid-transaction
func (r *postgresRepository) ClearChannelHomepage(ctx context.Context, channelID, keepPageID uuid.UUID) error {
	if err := r.queries.ClearCMSChannelHomepage(ctx, db.ClearCMSChannelHomepageParams{
		ChannelID: channelID,
		PageID:    keepPageID,
	}); err != nil {
		return fmt.Errorf("clear cms channel homepage: %w", err)
	}
	return nil
}

// GetHomePageForChannel returns the channel's homepage (the flagged, published
// page) - Returns ErrPageNotFound when the channel has no homepage set
func (r *postgresRepository) GetHomePageForChannel(ctx context.Context, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetCMSHomePageForChannel(ctx, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get cms homepage for channel: %w", err)
	}
	// GetCMSHomePageForChannel returns the bare page row only - Reuse GetPageByID
	// to hydrate AllContent (+ selections/relationships/listings) so the
	// resolver's `content` field resolves - without this, fillPageContent finds
	// no AllContent and the storefront falls back to its built-in homepage
	return r.GetPageByID(ctx, toPage(row).ID)
}

func (r *postgresRepository) DeletePageChannelListing(ctx context.Context, pageID, channelID uuid.UUID) error {
	_, err := r.queries.DeleteCMSPageChannelListing(ctx, db.DeleteCMSPageChannelListingParams{
		PageID:    pageID,
		ChannelID: channelID,
	})
	if err != nil {
		return fmt.Errorf("delete cms page store listing: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPageChannelListings(ctx context.Context, pageID uuid.UUID) ([]PageChannelListing, error) {
	rows, err := r.queries.ListCMSPageChannelListings(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list cms page store listings: %w", err)
	}
	out := make([]PageChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toPageChannelListing(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPageChannelListingsByPageIDs(ctx context.Context, pageIDs []uuid.UUID) (map[uuid.UUID][]PageChannelListing, error) {
	if len(pageIDs) == 0 {
		return map[uuid.UUID][]PageChannelListing{}, nil
	}
	rows, err := r.queries.ListCMSPageChannelListingsByPageIDs(ctx, pageIDs)
	if err != nil {
		return nil, fmt.Errorf("list cms page store listings by ids: %w", err)
	}
	out := make(map[uuid.UUID][]PageChannelListing)
	for _, row := range rows {
		out[row.PageID] = append(out[row.PageID], toPageChannelListing(row))
	}
	return out, nil
}

func (r *postgresRepository) GetPublishedPageForChannel(ctx context.Context, pageID, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetPublishedCMSPageForChannel(ctx, db.GetPublishedCMSPageForChannelParams{
		ID:        pageID,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get published cms page for store: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) GetPublishedPageBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetPublishedCMSPageBySlugForChannel(ctx, db.GetPublishedCMSPageBySlugForChannelParams{
		Slug:      slug,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get published cms page by slug for store: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) GetPublishedPageByURLPathForChannel(ctx context.Context, urlPath string, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetPublishedCMSPageByURLPathForChannel(ctx, db.GetPublishedCMSPageByURLPathForChannelParams{
		UrlPath:   urlPath,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get published cms page by url for store: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func toPageChannelListing(row db.CmsPageChannelListing) PageChannelListing {
	return PageChannelListing{
		ChannelID:   row.ChannelID,
		IsPublished: row.IsPublished,
		SEONoindex:  row.SeoNoindex,
		IsHomepage:  row.IsHomepage,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

// ==================== mappers ====================

func toAttribute(row db.CmsAttribute) Attribute {
	a := Attribute{
		ID:                row.ID,
		Slug:              row.Slug,
		InputType:         InputType(row.InputType),
		IsTranslatable:    row.IsTranslatable,
		IsRequiredDefault: row.IsRequiredDefault,
		Unit:              nullStringPtr(row.Unit),
		Validation:        row.Validation,
		Metadata:          metadataFromJSON(row.Metadata),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.RelationshipTarget.Valid {
		t := RelationshipTarget(row.RelationshipTarget.String)
		a.RelationshipTarget = &t
	}
	if row.DefaultValue.Valid {
		a.DefaultValue = row.DefaultValue.RawMessage
	}
	return a
}

func toAttributeContent(row db.CmsAttributeContent) AttributeContent {
	return AttributeContent{
		Locale:      row.Locale,
		Name:        row.Name,
		HelperText:  nullStringPtr(row.HelperText),
		Placeholder: nullStringPtr(row.Placeholder),
		UpdatedAt:   row.UpdatedAt,
	}
}

func toAttributeValue(row db.CmsAttributeValue) AttributeValue {
	return AttributeValue{
		ID:          row.ID,
		AttributeID: row.AttributeID,
		Slug:        row.Slug,
		Position:    row.Position,
		CreatedAt:   row.CreatedAt,
	}
}

// hydrateValueContent batch-loads per-locale names onto a set of attribute
// values (their AllContent), so the dashboard can edit localized option names
func (r *postgresRepository) hydrateValueContent(ctx context.Context, values []AttributeValue) {
	if len(values) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(values))
	for i, v := range values {
		ids[i] = v.ID
	}
	rows, err := r.queries.ListCMSAttributeValueContentByValueIDs(ctx, ids)
	if err != nil {
		return
	}
	byValue := make(map[uuid.UUID][]AttributeValueContent, len(values))
	for _, row := range rows {
		byValue[row.ValueID] = append(byValue[row.ValueID], AttributeValueContent{
			Locale: row.Locale, Name: row.Name, UpdatedAt: row.UpdatedAt,
		})
	}
	for i := range values {
		values[i].AllContent = byValue[values[i].ID]
	}
}

func (r *postgresRepository) GetAttributeValueByID(ctx context.Context, id uuid.UUID) (AttributeValue, error) {
	row, err := r.queries.GetCMSAttributeValueByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttributeValue{}, ErrAttributeValueNotFound
		}
		return AttributeValue{}, fmt.Errorf("get attribute value: %w", err)
	}
	return toAttributeValue(row), nil
}

func (r *postgresRepository) UpsertAttributeValueContent(ctx context.Context, valueID uuid.UUID, locale, name string) error {
	return r.queries.UpsertCMSAttributeValueContent(ctx, db.UpsertCMSAttributeValueContentParams{
		ValueID: valueID, Locale: locale, Name: name,
	})
}

func toAttributeGroup(row db.CmsAttributeGroup) AttributeGroup {
	return AttributeGroup{
		ID: row.ID, Slug: row.Slug, Position: row.Position,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func toPageType(row db.CmsPageType) PageType {
	pt := PageType{
		ID: row.ID, Slug: row.Slug, Icon: nullStringPtr(row.Icon),
		IsSystem: row.IsSystem, Position: row.Position,
		RootSlug:          nullStr(row.RootSlug),
		DefaultMetaTitle:  nullStr(row.DefaultMetaTitle),
		DefaultSEONoindex: row.DefaultSeoNoindex,
		IsSingleton:       row.IsSingleton,
		CreatedAt:         row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.DefaultBlocks.Valid {
		pt.DefaultBlocks = row.DefaultBlocks.RawMessage
	}
	return pt
}

func nullStr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func toPageTypeContent(row db.CmsPageTypeContent) PageTypeContent {
	return PageTypeContent{
		Locale: row.Locale, Name: row.Name,
		Description: nullStringPtr(row.Description),
		UpdatedAt:   row.UpdatedAt,
	}
}

func toPage(row db.CmsPage) Page {
	var publishedAt *time.Time
	if row.PublishedAt.Valid {
		t := row.PublishedAt.Time
		publishedAt = &t
	}
	var scheduledAt *time.Time
	if row.ScheduledAt.Valid {
		t := row.ScheduledAt.Time
		scheduledAt = &t
	}
	return Page{
		ID:              row.ID,
		PageTypeID:      row.PageTypeID,
		ParentID:        nullUUIDPtr(row.ParentID),
		Slug:            row.Slug,
		BasePath:        row.BasePath,
		URLPath:         row.UrlPath,
		ExperimentKey:   row.ExperimentKey,
		Position:        row.Position,
		IsPublished:     row.IsPublished,
		SEONoindex:      row.SeoNoindex,
		PublishedAt:     publishedAt,
		ScheduledAt:     scheduledAt,
		Data:            row.Data,
		Metadata:        metadataFromJSON(row.Metadata),
		PrivateMetadata: metadataFromJSON(row.PrivateMetadata),
		Tags:            tagsOrEmpty(row.Tags),
		WorkflowState:   WorkflowState(row.WorkflowState),
		AssignedTo:      nullUUIDPtr(row.AssignedTo),
		SubmittedBy:     nullUUIDPtr(row.SubmittedBy),
		SubmittedAt:     nullTimePtr(row.SubmittedAt),
		ApprovedBy:      nullUUIDPtr(row.ApprovedBy),
		ApprovedAt:      nullTimePtr(row.ApprovedAt),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// nullTimePtr converts a nullable timestamp read into a *time.Time (nil when
// NULL), the domain representation for optional times
func nullTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}

// tagsOrEmpty normalizes a nullable TEXT[] read into a non-nil slice so the
// domain/GraphQL contract (tags is always a list) holds even for a NULL column
func tagsOrEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

// normalizeTags cleans a tag set before persisting: trim whitespace, drop empties, lower-case (so "Sale"/"sale" filter
// as one), dedupe preserving first-seen order, and cap to a sane ceiling - Returns a non-nil slice
func normalizeTags(in []string) []string {
	const maxTags = 50
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= maxTags {
			break
		}
	}
	return out
}

func toPageContent(row db.CmsPageContent) PageContent {
	var body json.RawMessage
	if row.Body.Valid {
		body = row.Body.RawMessage
	}
	return PageContent{
		Locale: row.Locale, Title: row.Title, Body: body,
		MetaTitle:       nullStringPtr(row.MetaTitle),
		MetaDescription: nullStringPtr(row.MetaDescription),
		MetaKeywords:    nullStringPtr(row.MetaKeywords),
		Data:            row.Data,
		UpdatedAt:       row.UpdatedAt,
	}
}

// ==================== helpers ====================

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func nullUUIDPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	v := n.UUID
	return &v
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func emptyJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func metadataToJSON(m map[string]string) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func metadataFromJSON(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return map[string]string{}
	}
	return m
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching

// ==================== Snippets ====================

func toSnippet(r db.CmsSnippet) Snippet {
	return Snippet{ID: r.ID, Slug: r.Slug, Name: r.Name, Blocks: r.Blocks, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *postgresRepository) CreateSnippet(ctx context.Context, slug, name string, blocks json.RawMessage) (Snippet, error) {
	row, err := r.queries.CreateCMSSnippet(ctx, db.CreateCMSSnippetParams{Slug: slug, Name: name, Blocks: blocks})
	if err != nil {
		if isUniqueViolation(err) {
			return Snippet{}, ErrSlugTaken
		}
		return Snippet{}, fmt.Errorf("create cms snippet: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) GetSnippetByID(ctx context.Context, id uuid.UUID) (Snippet, error) {
	row, err := r.queries.GetCMSSnippetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrSnippetNotFound
		}
		return Snippet{}, fmt.Errorf("get cms snippet: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) GetSnippetBySlug(ctx context.Context, slug string) (Snippet, error) {
	row, err := r.queries.GetCMSSnippetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrSnippetNotFound
		}
		return Snippet{}, fmt.Errorf("get cms snippet by slug: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) ListSnippets(ctx context.Context) ([]Snippet, error) {
	rows, err := r.queries.ListCMSSnippets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cms snippets: %w", err)
	}
	out := make([]Snippet, len(rows))
	for i, row := range rows {
		out[i] = toSnippet(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateSnippet(ctx context.Context, id uuid.UUID, name string, blocks json.RawMessage) (Snippet, error) {
	row, err := r.queries.UpdateCMSSnippet(ctx, db.UpdateCMSSnippetParams{
		ID:     id,
		Name:   sql.NullString{String: name, Valid: true},
		Blocks: pqtype.NullRawMessage{RawMessage: blocks, Valid: blocks != nil},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrSnippetNotFound
		}
		return Snippet{}, fmt.Errorf("update cms snippet: %w", err)
	}
	return toSnippet(row), nil
}

func (r *postgresRepository) DeleteSnippet(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSSnippet(ctx, id); err != nil {
		return fmt.Errorf("delete cms snippet: %w", err)
	}
	return nil
}

// ==================== Block Types ====================

func toBlockType(r db.CmsBlockType) BlockType {
	return BlockType{ID: r.ID, Key: r.Key, Name: r.Name, Icon: r.Icon, Fields: r.Fields, HTMLTemplate: r.HtmlTemplate, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *postgresRepository) CreateBlockType(ctx context.Context, key, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	row, err := r.queries.CreateCMSBlockType(ctx, db.CreateCMSBlockTypeParams{Key: key, Name: name, Icon: icon, Fields: fields, HtmlTemplate: html})
	if err != nil {
		if isUniqueViolation(err) {
			return BlockType{}, ErrSlugTaken // the block type's key is its slug
		}
		return BlockType{}, fmt.Errorf("create cms block type: %w", err)
	}
	return toBlockType(row), nil
}

func (r *postgresRepository) GetBlockTypeByID(ctx context.Context, id uuid.UUID) (BlockType, error) {
	row, err := r.queries.GetCMSBlockTypeByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockType{}, ErrBlockTypeNotFound
		}
		return BlockType{}, fmt.Errorf("get cms block type: %w", err)
	}
	return toBlockType(row), nil
}

func (r *postgresRepository) ListBlockTypes(ctx context.Context) ([]BlockType, error) {
	rows, err := r.queries.ListCMSBlockTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cms block types: %w", err)
	}
	out := make([]BlockType, len(rows))
	for i, row := range rows {
		out[i] = toBlockType(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateBlockType(ctx context.Context, id uuid.UUID, name, icon string, fields json.RawMessage, html string) (BlockType, error) {
	row, err := r.queries.UpdateCMSBlockType(ctx, db.UpdateCMSBlockTypeParams{
		ID:           id,
		Name:         sql.NullString{String: name, Valid: true},
		Icon:         sql.NullString{String: icon, Valid: true},
		Fields:       pqtype.NullRawMessage{RawMessage: fields, Valid: fields != nil},
		HtmlTemplate: sql.NullString{String: html, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockType{}, ErrBlockTypeNotFound
		}
		return BlockType{}, fmt.Errorf("update cms block type: %w", err)
	}
	return toBlockType(row), nil
}

func (r *postgresRepository) DeleteBlockType(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSBlockType(ctx, id); err != nil {
		return fmt.Errorf("delete cms block type: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetBlockTypeByKey(ctx context.Context, key string) (BlockType, error) {
	row, err := r.queries.GetCMSBlockTypeByKey(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockType{}, ErrBlockTypeNotFound
		}
		return BlockType{}, fmt.Errorf("get cms block type by key: %w", err)
	}
	return toBlockType(row), nil
}

// ==================== Page Revisions + Scheduling ====================

func toPageRevision(r db.CmsPageRevision) PageRevision {
	rev := PageRevision{
		ID: r.ID, PageID: r.PageID, Locale: r.Locale, Title: r.Title,
		Data: r.Data, CreatedAt: r.CreatedAt,
	}
	if r.Body.Valid {
		rev.Body = r.Body.RawMessage
	}
	if r.MetaTitle.Valid {
		rev.MetaTitle = &r.MetaTitle.String
	}
	if r.MetaDescription.Valid {
		rev.MetaDescription = &r.MetaDescription.String
	}
	if r.MetaKeywords.Valid {
		rev.MetaKeywords = &r.MetaKeywords.String
	}
	if r.AuthorAdminID.Valid {
		id := r.AuthorAdminID.UUID
		rev.AuthorAdminID = &id
	}
	return rev
}

func (r *postgresRepository) CreateRevision(ctx context.Context, pageID uuid.UUID, in PageContentInput, author *uuid.UUID) (PageRevision, error) {
	data := in.Data
	if data == nil {
		data = json.RawMessage("{}")
	}
	row, err := r.queries.CreateCMSPageRevision(ctx, db.CreateCMSPageRevisionParams{
		PageID:          pageID,
		Locale:          in.Locale,
		Title:           in.Title,
		Body:            pqtype.NullRawMessage{RawMessage: in.Body, Valid: in.Body != nil},
		MetaTitle:       sql.NullString{String: in.MetaTitle, Valid: in.MetaTitle != ""},
		MetaDescription: sql.NullString{String: in.MetaDescription, Valid: in.MetaDescription != ""},
		MetaKeywords:    sql.NullString{String: in.MetaKeywords, Valid: in.MetaKeywords != ""},
		Data:            data,
		AuthorAdminID:   uuid.NullUUID{UUID: ptr.Deref(author), Valid: author != nil},
	})
	if err != nil {
		return PageRevision{}, fmt.Errorf("create cms page revision: %w", err)
	}
	return toPageRevision(row), nil
}

func (r *postgresRepository) ListRevisions(ctx context.Context, pageID uuid.UUID, locale string, limit int32) ([]PageRevision, error) {
	rows, err := r.queries.ListCMSPageRevisions(ctx, db.ListCMSPageRevisionsParams{PageID: pageID, Locale: locale, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list cms page revisions: %w", err)
	}
	out := make([]PageRevision, len(rows))
	for i, row := range rows {
		out[i] = toPageRevision(row)
	}
	return out, nil
}

func (r *postgresRepository) GetRevision(ctx context.Context, id uuid.UUID) (PageRevision, error) {
	row, err := r.queries.GetCMSPageRevisionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PageRevision{}, ErrPageNotFound
		}
		return PageRevision{}, fmt.Errorf("get cms page revision: %w", err)
	}
	return toPageRevision(row), nil
}

func (r *postgresRepository) PruneRevisions(ctx context.Context, pageID uuid.UUID, locale string, keep int32) error {
	return r.queries.PruneCMSPageRevisions(ctx, db.PruneCMSPageRevisionsParams{PageID: pageID, Locale: locale, KeepCount: keep})
}

func (r *postgresRepository) SetScheduledAt(ctx context.Context, pageID uuid.UUID, at *time.Time) (Page, error) {
	var st sql.NullTime
	if at != nil {
		st = sql.NullTime{Time: *at, Valid: true}
	}
	row, err := r.queries.SetCMSPageScheduledAt(ctx, db.SetCMSPageScheduledAtParams{ID: pageID, ScheduledAt: st})
	if err != nil {
		return Page{}, fmt.Errorf("set cms page scheduled_at: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) ListDueScheduledPages(ctx context.Context) ([]Page, error) {
	rows, err := r.queries.ListDueScheduledCMSPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list due scheduled cms pages: %w", err)
	}
	out := make([]Page, len(rows))
	for i, row := range rows {
		out[i] = toPage(row)
	}
	return out, nil
}

func toNullUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

// ==================== Editorial workflow ====================

func (r *postgresRepository) SubmitPageForReview(ctx context.Context, id uuid.UUID, by *uuid.UUID) (Page, error) {
	row, err := r.queries.SubmitCMSPageForReview(ctx, db.SubmitCMSPageForReviewParams{ID: id, SubmittedBy: toNullUUID(by)})
	if err != nil {
		return Page{}, fmt.Errorf("submit for review: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) ApprovePageWorkflow(ctx context.Context, id uuid.UUID, by *uuid.UUID) (Page, error) {
	row, err := r.queries.ApproveCMSPageWorkflow(ctx, db.ApproveCMSPageWorkflowParams{ID: id, ApprovedBy: toNullUUID(by)})
	if err != nil {
		return Page{}, fmt.Errorf("approve page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) SetPageWorkflowState(ctx context.Context, id uuid.UUID, state WorkflowState) (Page, error) {
	row, err := r.queries.SetCMSPageWorkflowState(ctx, db.SetCMSPageWorkflowStateParams{ID: id, WorkflowState: string(state)})
	if err != nil {
		return Page{}, fmt.Errorf("set workflow state: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) AssignPage(ctx context.Context, id uuid.UUID, to *uuid.UUID) (Page, error) {
	row, err := r.queries.AssignCMSPage(ctx, db.AssignCMSPageParams{ID: id, AssignedTo: toNullUUID(to)})
	if err != nil {
		return Page{}, fmt.Errorf("assign page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) CreateComment(ctx context.Context, in CommentInput, authorID uuid.UUID) (PageComment, error) {
	row, err := r.queries.CreateCMSPageComment(ctx, db.CreateCMSPageCommentParams{
		PageID:   in.PageID,
		Locale:   nullableStringPtr(in.Locale),
		BlockID:  nullableStringPtr(in.BlockID),
		AuthorID: authorID,
		Body:     in.Body,
	})
	if err != nil {
		return PageComment{}, fmt.Errorf("create comment: %w", err)
	}
	return toPageComment(row), nil
}

func (r *postgresRepository) ListComments(ctx context.Context, pageID uuid.UUID) ([]PageComment, error) {
	rows, err := r.queries.ListCMSPageComments(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	out := make([]PageComment, len(rows))
	for i, row := range rows {
		out[i] = toPageComment(row)
	}
	return out, nil
}

func (r *postgresRepository) ResolveComment(ctx context.Context, id uuid.UUID, resolved bool, by *uuid.UUID) (PageComment, error) {
	row, err := r.queries.ResolveCMSPageComment(ctx, db.ResolveCMSPageCommentParams{ID: id, Resolved: resolved, ResolvedBy: toNullUUID(by)})
	if err != nil {
		return PageComment{}, fmt.Errorf("resolve comment: %w", err)
	}
	return toPageComment(row), nil
}

func (r *postgresRepository) DeleteComment(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSPageComment(ctx, id); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}

func toPageComment(row db.CmsPageComment) PageComment {
	return PageComment{
		ID:         row.ID,
		PageID:     row.PageID,
		Locale:     nullStringPtr(row.Locale),
		BlockID:    nullStringPtr(row.BlockID),
		AuthorID:   row.AuthorID,
		Body:       row.Body,
		Resolved:   row.Resolved,
		ResolvedBy: nullUUIDPtr(row.ResolvedBy),
		ResolvedAt: nullTimePtr(row.ResolvedAt),
		CreatedAt:  row.CreatedAt,
	}
}

// ==================== Tag library ====================

func toTag(row db.CmsTag) Tag {
	return Tag{Slug: row.Slug, Name: row.Name, CreatedAt: row.CreatedAt}
}

func (r *postgresRepository) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := r.queries.ListCMSTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	out := make([]Tag, len(rows))
	for i, row := range rows {
		out[i] = toTag(row)
	}
	return out, nil
}

func (r *postgresRepository) UpsertTag(ctx context.Context, slug, name string) (Tag, error) {
	row, err := r.queries.UpsertCMSTag(ctx, db.UpsertCMSTagParams{Slug: slug, Name: name})
	if err != nil {
		return Tag{}, fmt.Errorf("upsert tag: %w", err)
	}
	return toTag(row), nil
}

func (r *postgresRepository) RenameTag(ctx context.Context, slug, name string) (Tag, error) {
	row, err := r.queries.RenameCMSTag(ctx, db.RenameCMSTagParams{Slug: slug, Name: name})
	if err != nil {
		return Tag{}, fmt.Errorf("rename tag: %w", err)
	}
	return toTag(row), nil
}

// DeleteTag removes a tag from the vocabulary AND strips it from every page that
// carries it (so no dangling slug survives in cms_pages.tags)
func (r *postgresRepository) DeleteTag(ctx context.Context, slug string) error {
	if err := r.queries.StripTagFromPages(ctx, slug); err != nil {
		return fmt.Errorf("strip tag from pages: %w", err)
	}
	if err := r.queries.DeleteCMSTag(ctx, slug); err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	return nil
}

func (r *postgresRepository) TagUsageCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.queries.CMSTagUsageCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("tag usage counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		if slug, ok := row.Slug.(string); ok {
			out[slug] = int(row.Count)
		}
	}
	return out, nil
}
