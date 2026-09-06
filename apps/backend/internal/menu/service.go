package menu

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/events"
)

type LocaleValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

type Service struct {
	repo    Repository
	bus     *events.Bus
	cache   *cache.Cache
	locales LocaleValidator
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

func (s *Service) SetLocaleValidator(v LocaleValidator) { s.locales = v }

// Wires the Valkey cache shared with the storefront menu resolver - the resolver reads from it and this service drops it on every write, so operators editing or reordering menus see changes on the next request rather than waiting for TTL
func (s *Service) SetCache(c *cache.Cache) { s.cache = c }

// Exposes the underlying cache so the storefront resolver reads from the same Valkey namespace this service invalidates - nil when no cache is configured
func (s *Service) Cache() *cache.Cache { return s.cache }

// Drops every cached storefront-menu entry, called from publishMenuUpdated so every mutation routes through one invalidation point and a new write method can't forget it
// DelPattern is O(keys), but the menu namespace is small - one entry per slug, store and locale - and writes are rare
func (s *Service) invalidate(ctx context.Context) {
	if s.cache == nil {
		return
	}
	s.cache.DelPattern(ctx, "*")
}

func (s *Service) publishMenuUpdated(ctx context.Context, menuID uuid.UUID, slug string) {
	s.invalidate(ctx)
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &MenuUpdated{MenuID: menuID, Slug: slug})
}

func (s *Service) CreateMenu(ctx context.Context, input CreateMenuInput) (Menu, error) {
	if err := validateSlug(input.Slug); err != nil {
		return Menu{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Menu{}, fmt.Errorf("name is required")
	}
	m, err := s.repo.CreateMenu(ctx, input)
	if err != nil {
		return Menu{}, err
	}
	s.publishMenuUpdated(ctx, m.ID, m.Slug)
	return m, nil
}

func (s *Service) GetMenuByID(ctx context.Context, id uuid.UUID) (Menu, error) {
	return s.repo.GetMenuByID(ctx, id)
}

func (s *Service) GetMenuBySlug(ctx context.Context, slug string) (Menu, error) {
	return s.repo.GetMenuBySlug(ctx, slug)
}

func (s *Service) ListMenus(ctx context.Context) ([]Menu, error) {
	return s.repo.ListMenus(ctx)
}

func (s *Service) UpdateMenu(ctx context.Context, input UpdateMenuInput) (Menu, error) {
	if input.Slug != nil {
		if err := validateSlug(*input.Slug); err != nil {
			return Menu{}, err
		}
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Menu{}, fmt.Errorf("name is required")
	}
	m, err := s.repo.UpdateMenu(ctx, input)
	if err != nil {
		return Menu{}, err
	}
	s.publishMenuUpdated(ctx, m.ID, m.Slug)
	return m, nil
}

func (s *Service) DeleteMenu(ctx context.Context, id uuid.UUID) error {
	snap, _ := s.repo.GetMenuByID(ctx, id)
	if err := s.repo.DeleteMenu(ctx, id); err != nil {
		return err
	}
	s.publishMenuUpdated(ctx, id, snap.Slug)
	return nil
}

// Validates the target/url XOR and depth constraints in the service, rather than letting the DB CHECK catch them with an ugly message
func (s *Service) CreateMenuItem(ctx context.Context, input CreateMenuItemInput) (MenuItem, error) {
	if strings.TrimSpace(input.Label) == "" {
		return MenuItem{}, fmt.Errorf("label is required")
	}
	if err := validateTarget(input.TargetType, input.TargetID, input.URL); err != nil {
		return MenuItem{}, err
	}
	if input.ParentID != nil {
		if err := s.validateParent(ctx, input.MenuID, *input.ParentID, uuid.Nil); err != nil {
			return MenuItem{}, err
		}
	}
	item, err := s.repo.CreateMenuItem(ctx, input)
	if err != nil {
		return MenuItem{}, err
	}
	if m, err := s.repo.GetMenuByID(ctx, input.MenuID); err == nil {
		s.publishMenuUpdated(ctx, m.ID, m.Slug)
	}
	return item, nil
}

func (s *Service) ListMenuItems(ctx context.Context, menuID uuid.UUID) ([]MenuItem, error) {
	return s.repo.ListMenuItemsByMenu(ctx, menuID)
}

func (s *Service) UpdateMenuItem(ctx context.Context, input UpdateMenuItemInput) (MenuItem, error) {
	current, err := s.repo.GetMenuItemByID(ctx, input.ID)
	if err != nil {
		return MenuItem{}, err
	}
	if input.Label != nil && strings.TrimSpace(*input.Label) == "" {
		return MenuItem{}, fmt.Errorf("label cannot be empty")
	}
	// Recompute the effective {targetType, targetID, url} and validate the XOR - nil inputs mean keep current
	tt := current.TargetType
	if input.TargetType != nil {
		tt = *input.TargetType
	}
	tid := current.TargetID
	if input.TargetID != nil {
		tid = input.TargetID
	}
	url := current.URL
	if input.URL != nil {
		url = *input.URL
	}
	if err := validateTarget(tt, tid, url); err != nil {
		return MenuItem{}, err
	}
	// Parent change - validate cycle, same-menu and depth
	if input.ClearParent {
	} else if input.ParentID != nil {
		if err := s.validateParent(ctx, current.MenuID, *input.ParentID, current.ID); err != nil {
			return MenuItem{}, err
		}
	}
	item, err := s.repo.UpdateMenuItem(ctx, input)
	if err != nil {
		return MenuItem{}, err
	}
	if m, err := s.repo.GetMenuByID(ctx, current.MenuID); err == nil {
		s.publishMenuUpdated(ctx, m.ID, m.Slug)
	}
	return item, nil
}

// Re-stamps sort_order, and optionally parent_id, for a batch of items, backing the dashboard's drag-reorder
// Updates run one-by-one, which is fine at the menu sizes we expect and avoids a multi-row UPDATE Postgres can't always plan well
func (s *Service) ReorderMenuItems(ctx context.Context, menuID uuid.UUID, items []ReorderMenuItem) error {
	for _, it := range items {
		current, err := s.repo.GetMenuItemByID(ctx, it.ID)
		if err != nil {
			return err
		}
		if current.MenuID != menuID {
			return ErrParentInOtherMenu
		}
		parentInput := it.ParentID
		clearParent := it.ParentID == nil && current.ParentID != nil
		if it.ParentID != nil {
			if err := s.validateParent(ctx, menuID, *it.ParentID, it.ID); err != nil {
				return err
			}
		}
		_, err = s.repo.UpdateMenuItem(ctx, UpdateMenuItemInput{
			ID:          it.ID,
			ParentID:    parentInput,
			ClearParent: clearParent,
			SortOrder:   &it.SortOrder,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeleteMenuItem(ctx context.Context, id uuid.UUID) error {
	current, _ := s.repo.GetMenuItemByID(ctx, id)
	if err := s.repo.DeleteMenuItem(ctx, id); err != nil {
		return err
	}
	if m, err := s.repo.GetMenuByID(ctx, current.MenuID); err == nil {
		s.publishMenuUpdated(ctx, m.ID, m.Slug)
	}
	return nil
}

// The storefront's entry point - ErrMenuNotFound when the slug doesn't exist OR isn't assigned to channelID
func (s *Service) GetMenuBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Menu, error) {
	return s.repo.GetMenuBySlugForChannel(ctx, slug, channelID)
}

func (s *Service) ListMenuChannelAssignments(ctx context.Context, menuID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.ListMenuChannelAssignments(ctx, menuID)
}

func (s *Service) ListMenuChannelAssignmentsByMenuIDs(ctx context.Context, menuIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return s.repo.ListMenuChannelAssignmentsByMenuIDs(ctx, menuIDs)
}

// Replaces the entire assignment set in one call, dropping everything then inserting the desired set - the admin UI ships the full list per save, so partial-update semantics aren't needed
func (s *Service) SetMenuStores(ctx context.Context, menuID uuid.UUID, channelIDs []uuid.UUID) error {
	if _, err := s.repo.GetMenuByID(ctx, menuID); err != nil {
		return err
	}
	if err := s.repo.DeleteAllMenuChannelAssignments(ctx, menuID); err != nil {
		return err
	}
	seen := make(map[uuid.UUID]bool, len(channelIDs))
	for _, sv := range channelIDs {
		if seen[sv] {
			continue
		}
		seen[sv] = true
		if err := s.repo.InsertMenuChannelAssignment(ctx, menuID, sv); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ListMenuItemTranslations(ctx context.Context, itemID uuid.UUID) ([]MenuItemTranslation, error) {
	return s.repo.ListMenuItemTranslations(ctx, itemID)
}

func (s *Service) ListMenuItemTranslationsByItemIDs(ctx context.Context, itemIDs []uuid.UUID) (map[uuid.UUID][]MenuItemTranslation, error) {
	return s.repo.ListMenuItemTranslationsByItemIDs(ctx, itemIDs)
}

func (s *Service) UpsertMenuItemTranslation(ctx context.Context, itemID uuid.UUID, locale, label string) (MenuItemTranslation, error) {
	if strings.TrimSpace(locale) == "" {
		return MenuItemTranslation{}, fmt.Errorf("locale is required")
	}
	if strings.TrimSpace(label) == "" {
		return MenuItemTranslation{}, fmt.Errorf("label is required")
	}
	if s.locales != nil {
		if err := s.locales.EnsureExists(ctx, locale); err != nil {
			return MenuItemTranslation{}, err
		}
	}
	return s.repo.UpsertMenuItemTranslation(ctx, itemID, locale, label)
}

func (s *Service) DeleteMenuItemTranslation(ctx context.Context, itemID uuid.UUID, locale string) error {
	return s.repo.DeleteMenuItemTranslation(ctx, itemID, locale)
}

// Picks the locale-specific label when available, else the item's base label - used by the front resolver and any other read site wanting the user-facing label
func ResolveItemLabel(item MenuItem, translations []MenuItemTranslation, locale string) string {
	if locale == "" {
		return item.Label
	}
	for _, t := range translations {
		if t.Locale == locale {
			return t.Label
		}
	}
	return item.Label
}

func validateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	for _, r := range slug {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("slug must contain only lowercase letters, digits, hyphens, and underscores")
		}
	}
	return nil
}

func validateTarget(targetType TargetType, targetID *uuid.UUID, url string) error {
	hasTarget := targetType != "" && targetID != nil
	hasURL := strings.TrimSpace(url) != ""
	if hasTarget == hasURL {
		// Either both set or neither - both invalid for the XOR
		return ErrTargetXorURL
	}
	if hasTarget {
		if !targetType.IsValid() {
			return ErrInvalidTargetType
		}
	}
	return nil
}

// Checks the parent exists, lives in the same menu, wouldn't form a cycle through childID (Nil means a brand-new item, so no cycle is possible), and that the resulting depth is within MaxDepth
func (s *Service) validateParent(ctx context.Context, menuID, parentID, childID uuid.UUID) error {
	parent, err := s.repo.GetMenuItemByID(ctx, parentID)
	if err != nil {
		return ErrMenuItemNotFound
	}
	if parent.MenuID != menuID {
		return ErrParentInOtherMenu
	}
	// Walk up from parent - hitting childID is a cycle, and count depth along the way
	depth := 1
	cur := parent
	for {
		if childID != uuid.Nil && cur.ID == childID {
			return ErrParentCycle
		}
		if cur.ParentID == nil {
			break
		}
		if depth >= MaxDepth {
			return ErrDepthExceeded
		}
		next, err := s.repo.GetMenuItemByID(ctx, *cur.ParentID)
		if err != nil {
			return ErrMenuItemNotFound
		}
		cur = next
		depth++
	}
	if depth >= MaxDepth {
		return ErrDepthExceeded
	}
	return nil
}
