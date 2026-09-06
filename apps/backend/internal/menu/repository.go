package menu

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

// --- menus ---

func (r *postgresRepository) CreateMenu(ctx context.Context, input CreateMenuInput) (Menu, error) {
	row, err := r.queries.CreateMenu(ctx, db.CreateMenuParams{
		Slug: input.Slug,
		Name: input.Name,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Menu{}, ErrMenuSlugTaken
		}
		return Menu{}, fmt.Errorf("create menu: %w", err)
	}
	return toMenu(row, 0), nil
}

func (r *postgresRepository) GetMenuByID(ctx context.Context, id uuid.UUID) (Menu, error) {
	row, err := r.queries.GetMenuByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Menu{}, ErrMenuNotFound
		}
		return Menu{}, fmt.Errorf("get menu: %w", err)
	}
	return toMenu(row, 0), nil
}

func (r *postgresRepository) GetMenuBySlug(ctx context.Context, slug string) (Menu, error) {
	row, err := r.queries.GetMenuBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Menu{}, ErrMenuNotFound
		}
		return Menu{}, fmt.Errorf("get menu by slug: %w", err)
	}
	return toMenu(row, 0), nil
}

func (r *postgresRepository) ListMenus(ctx context.Context) ([]Menu, error) {
	rows, err := r.queries.ListMenus(ctx)
	if err != nil {
		return nil, fmt.Errorf("list menus: %w", err)
	}
	out := make([]Menu, len(rows))
	for i, row := range rows {
		out[i] = toMenu(db.Menu{
			ID:        row.ID,
			Slug:      row.Slug,
			Name:      row.Name,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}, row.ItemCount)
	}
	return out, nil
}

func (r *postgresRepository) UpdateMenu(ctx context.Context, input UpdateMenuInput) (Menu, error) {
	current, err := r.queries.GetMenuByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Menu{}, ErrMenuNotFound
		}
		return Menu{}, fmt.Errorf("load menu: %w", err)
	}
	slug := current.Slug
	if input.Slug != nil {
		slug = *input.Slug
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	row, err := r.queries.UpdateMenu(ctx, db.UpdateMenuParams{
		ID:   input.ID,
		Slug: slug,
		Name: name,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Menu{}, ErrMenuSlugTaken
		}
		return Menu{}, fmt.Errorf("update menu: %w", err)
	}
	return toMenu(row, 0), nil
}

func (r *postgresRepository) DeleteMenu(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteMenu(ctx, id); err != nil {
		return fmt.Errorf("delete menu: %w", err)
	}
	return nil
}

// --- menu items ---

func (r *postgresRepository) CreateMenuItem(ctx context.Context, input CreateMenuItemInput) (MenuItem, error) {
	row, err := r.queries.CreateMenuItem(ctx, db.CreateMenuItemParams{
		MenuID:     input.MenuID,
		ParentID:   nullableUUID(input.ParentID),
		Label:      input.Label,
		TargetType: nullableString(string(input.TargetType)),
		TargetID:   nullableUUID(input.TargetID),
		Url:        nullableString(input.URL),
		SortOrder:  input.SortOrder,
	})
	if err != nil {
		return MenuItem{}, fmt.Errorf("create menu item: %w", err)
	}
	return toMenuItem(row), nil
}

func (r *postgresRepository) GetMenuItemByID(ctx context.Context, id uuid.UUID) (MenuItem, error) {
	row, err := r.queries.GetMenuItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MenuItem{}, ErrMenuItemNotFound
		}
		return MenuItem{}, fmt.Errorf("get menu item: %w", err)
	}
	return toMenuItem(row), nil
}

func (r *postgresRepository) ListMenuItemsByMenu(ctx context.Context, menuID uuid.UUID) ([]MenuItem, error) {
	rows, err := r.queries.ListMenuItemsByMenu(ctx, menuID)
	if err != nil {
		return nil, fmt.Errorf("list menu items: %w", err)
	}
	out := make([]MenuItem, len(rows))
	for i, row := range rows {
		out[i] = toMenuItem(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateMenuItem(ctx context.Context, input UpdateMenuItemInput) (MenuItem, error) {
	current, err := r.queries.GetMenuItemByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MenuItem{}, ErrMenuItemNotFound
		}
		return MenuItem{}, fmt.Errorf("load menu item: %w", err)
	}
	label := current.Label
	if input.Label != nil {
		label = *input.Label
	}
	tt := current.TargetType
	if input.TargetType != nil {
		tt = nullableString(string(*input.TargetType))
	}
	tid := current.TargetID
	if input.TargetID != nil {
		tid = nullableUUID(input.TargetID)
	}
	url := current.Url
	if input.URL != nil {
		url = nullableString(*input.URL)
	}
	parent := current.ParentID
	if input.ClearParent {
		parent = uuid.NullUUID{}
	} else if input.ParentID != nil {
		parent = nullableUUID(input.ParentID)
	}
	sortOrder := current.SortOrder
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}
	row, err := r.queries.UpdateMenuItem(ctx, db.UpdateMenuItemParams{
		ID:         input.ID,
		Label:      label,
		TargetType: tt,
		TargetID:   tid,
		Url:        url,
		ParentID:   parent,
		SortOrder:  sortOrder,
	})
	if err != nil {
		return MenuItem{}, fmt.Errorf("update menu item: %w", err)
	}
	return toMenuItem(row), nil
}

func (r *postgresRepository) UpdateMenuItemSortOrder(ctx context.Context, id uuid.UUID, sortOrder int32) error {
	if err := r.queries.UpdateMenuItemSortOrder(ctx, db.UpdateMenuItemSortOrderParams{
		ID:        id,
		SortOrder: sortOrder,
	}); err != nil {
		return fmt.Errorf("update menu item sort_order: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteMenuItem(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteMenuItem(ctx, id); err != nil {
		return fmt.Errorf("delete menu item: %w", err)
	}
	return nil
}

// --- store assignments ---

func (r *postgresRepository) GetMenuBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Menu, error) {
	row, err := r.queries.GetMenuBySlugForChannel(ctx, db.GetMenuBySlugForChannelParams{
		Slug:      slug,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Menu{}, ErrMenuNotFound
		}
		return Menu{}, fmt.Errorf("get menu by slug for store: %w", err)
	}
	return toMenu(row, 0), nil
}

func (r *postgresRepository) ListMenuChannelAssignments(ctx context.Context, menuID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.queries.ListMenuChannelAssignments(ctx, menuID)
	if err != nil {
		return nil, fmt.Errorf("list menu store assignments: %w", err)
	}
	return rows, nil
}

func (r *postgresRepository) ListMenuChannelAssignmentsByMenuIDs(ctx context.Context, menuIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(menuIDs) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}
	rows, err := r.queries.ListMenuChannelAssignmentsByMenuIDs(ctx, menuIDs)
	if err != nil {
		return nil, fmt.Errorf("list menu store assignments batched: %w", err)
	}
	out := make(map[uuid.UUID][]uuid.UUID, len(menuIDs))
	for _, row := range rows {
		out[row.MenuID] = append(out[row.MenuID], row.ChannelID)
	}
	return out, nil
}

func (r *postgresRepository) InsertMenuChannelAssignment(ctx context.Context, menuID, channelID uuid.UUID) error {
	if err := r.queries.InsertMenuChannelAssignment(ctx, db.InsertMenuChannelAssignmentParams{
		MenuID:    menuID,
		ChannelID: channelID,
	}); err != nil {
		return fmt.Errorf("insert menu store assignment: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteMenuChannelAssignment(ctx context.Context, menuID, channelID uuid.UUID) error {
	if err := r.queries.DeleteMenuChannelAssignment(ctx, db.DeleteMenuChannelAssignmentParams{
		MenuID:    menuID,
		ChannelID: channelID,
	}); err != nil {
		return fmt.Errorf("delete menu store assignment: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteAllMenuChannelAssignments(ctx context.Context, menuID uuid.UUID) error {
	if err := r.queries.DeleteAllMenuChannelAssignments(ctx, menuID); err != nil {
		return fmt.Errorf("delete all menu store assignments: %w", err)
	}
	return nil
}

// --- translations ---

func (r *postgresRepository) ListMenuItemTranslations(ctx context.Context, itemID uuid.UUID) ([]MenuItemTranslation, error) {
	rows, err := r.queries.ListMenuItemTranslations(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list menu item translations: %w", err)
	}
	out := make([]MenuItemTranslation, len(rows))
	for i, row := range rows {
		out[i] = toMenuItemTranslation(row)
	}
	return out, nil
}

func (r *postgresRepository) ListMenuItemTranslationsByItemIDs(ctx context.Context, itemIDs []uuid.UUID) (map[uuid.UUID][]MenuItemTranslation, error) {
	if len(itemIDs) == 0 {
		return map[uuid.UUID][]MenuItemTranslation{}, nil
	}
	rows, err := r.queries.ListMenuItemTranslationsByItemIDs(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("list menu item translations batched: %w", err)
	}
	out := make(map[uuid.UUID][]MenuItemTranslation, len(itemIDs))
	for _, row := range rows {
		out[row.ItemID] = append(out[row.ItemID], toMenuItemTranslation(row))
	}
	return out, nil
}

func (r *postgresRepository) UpsertMenuItemTranslation(ctx context.Context, itemID uuid.UUID, locale, label string) (MenuItemTranslation, error) {
	row, err := r.queries.UpsertMenuItemTranslation(ctx, db.UpsertMenuItemTranslationParams{
		ItemID: itemID,
		Locale: locale,
		Label:  label,
	})
	if err != nil {
		return MenuItemTranslation{}, fmt.Errorf("upsert menu item translation: %w", err)
	}
	return toMenuItemTranslation(row), nil
}

func (r *postgresRepository) DeleteMenuItemTranslation(ctx context.Context, itemID uuid.UUID, locale string) error {
	if err := r.queries.DeleteMenuItemTranslation(ctx, db.DeleteMenuItemTranslationParams{
		ItemID: itemID,
		Locale: locale,
	}); err != nil {
		return fmt.Errorf("delete menu item translation: %w", err)
	}
	return nil
}

func toMenuItemTranslation(row db.MenuItemTranslation) MenuItemTranslation {
	return MenuItemTranslation{
		ItemID:    row.ItemID,
		Locale:    row.Locale,
		Label:     row.Label,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// --- mappers ---

func toMenu(row db.Menu, itemCount int64) Menu {
	return Menu{
		ID:        row.ID,
		Slug:      row.Slug,
		Name:      row.Name,
		ItemCount: itemCount,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toMenuItem(row db.MenuItem) MenuItem {
	out := MenuItem{
		ID:        row.ID,
		MenuID:    row.MenuID,
		Label:     row.Label,
		SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.ParentID.Valid {
		v := row.ParentID.UUID
		out.ParentID = &v
	}
	if row.TargetType.Valid {
		out.TargetType = TargetType(row.TargetType.String)
	}
	if row.TargetID.Valid {
		v := row.TargetID.UUID
		out.TargetID = &v
	}
	if row.Url.Valid {
		out.URL = row.Url.String
	}
	return out
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullableUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
