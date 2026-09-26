package cms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

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
			Unit:               sqlnull.StringValueNonEmpty(in.Unit),
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
		unit = sqlnull.StringValueNonEmpty(*in.Unit)
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
		HelperText:  sqlnull.StringValueNonEmpty(in.HelperText),
		Placeholder: sqlnull.StringValueNonEmpty(in.Placeholder),
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

func toAttribute(row db.CmsAttribute) Attribute {
	a := Attribute{
		ID:                row.ID,
		Slug:              row.Slug,
		InputType:         InputType(row.InputType),
		IsTranslatable:    row.IsTranslatable,
		IsRequiredDefault: row.IsRequiredDefault,
		Unit:              sqlnull.StringPtr(row.Unit),
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
		HelperText:  sqlnull.StringPtr(row.HelperText),
		Placeholder: sqlnull.StringPtr(row.Placeholder),
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
