package emailtemplate

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// System email-group slugs - the groups themselves are schema seed data, and these constants are just the stable identifiers the code finds them by, with no group content hardcoded
const (
	GroupSlugTransactional = "transactional"
	GroupSlugMarketing     = "marketing"
)

// The starter reusable blocks seeded into a channel's block library - common sections built from the built-in block types, so they render everywhere and stay fully editable
func standardBlocks() []Snippet {
	center := &BlockStyle{Align: "center"}
	return []Snippet{
		{
			Slug: "header-logo", Name: "Header - logo", Icon: "🏷️",
			Blocks: []Block{
				{Type: "image", URL: "{{store.logoUrl}}", Alt: "{{store.name}}", Style: &BlockStyle{Align: "center", Width: 45, PaddingY: 20}},
			},
		},
		{
			Slug: "hero", Name: "Hero", Icon: "🖼️",
			Blocks: []Block{
				{Type: "heading", Text: "Your headline here", Style: &BlockStyle{Align: "center", FontSize: 28}},
				{Type: "text", Content: "A short line of supporting copy that sets up the offer or announcement.", Style: center},
				{Type: "button", Label: "Shop now", URL: "{{storefront.url}}", Style: center},
				{Type: "spacer", Size: 8},
			},
		},
		{
			Slug: "cta", Name: "Call to action", Icon: "👉",
			Blocks: []Block{
				{Type: "heading", Text: "Ready to get started?", Style: &BlockStyle{Align: "center", FontSize: 22}},
				{Type: "text", Content: "Tell customers what to do next in one clear sentence.", Style: center},
				{Type: "button", Label: "Browse the store", URL: "{{storefront.url}}", Style: &BlockStyle{Align: "center", PaddingY: 4}},
			},
		},
		{
			Slug: "social-follow", Name: "Social follow", Icon: "📣",
			Blocks: []Block{
				{Type: "text", Content: "Follow us", Style: &BlockStyle{Align: "center", Color: "#666666"}},
				{Type: "social", Style: center, Social: []SocialLink{
					{Platform: "facebook", URL: "https://facebook.com/"},
					{Platform: "instagram", URL: "https://instagram.com/"},
					{Platform: "x", URL: "https://x.com/"},
				}},
			},
		},
		{
			Slug: "footer", Name: "Footer", Icon: "📄",
			Blocks: []Block{
				{Type: "divider", Style: &BlockStyle{Color: "#e5e5e5", PaddingY: 8}},
				{Type: "text", Content: "You're receiving this because you're a {{store.name}} customer.", Style: &BlockStyle{Align: "center", Color: "#999999", FontSize: 12}},
				{Type: "text", Content: "{{store.name}} · {{storefront.url}}", Style: &BlockStyle{Align: "center", Color: "#999999", FontSize: 12}},
				{Type: "button", Label: "Unsubscribe", URL: "{{unsubscribe.link}}", Style: &BlockStyle{Align: "center", ButtonColor: "#eeeeee", ButtonTextColor: "#666666", FontSize: 12, Radius: 4}},
			},
		},
	}
}

// Ensures a channel's block library has the starter reusable blocks - idempotent, so a slug that already exists is left untouched and an operator's edits are never clobbered - Returns how many were created
func (s *Service) SeedStandardBlocks(ctx context.Context, channelID uuid.UUID) (int, error) {
	if channelID == uuid.Nil {
		return 0, fmt.Errorf("channel is required")
	}
	created := 0
	for _, b := range standardBlocks() {
		if _, ok, err := s.repo.GetBlock(ctx, channelID, b.Slug); err != nil {
			return created, err
		} else if ok {
			continue // already present - don't overwrite
		}
		b.ChannelID = channelID
		if err := s.SaveBlock(ctx, b); err != nil {
			return created, fmt.Errorf("seed block %s: %w", b.Slug, err)
		}
		created++
	}
	return created, nil
}

// The human name for a transactional slot, used as the seeded email's name and in the admin slot list - mirrors the seeded names
func SlotLabel(slot string) string {
	switch slot {
	case KeyOrderConfirmation:
		return "Order confirmation"
	case KeyOrderShipment:
		return "Shipment notification"
	case KeyOrderCancellation:
		return "Order cancellation"
	case KeyOrderRefund:
		return "Refund notification"
	case KeyVerification:
		return "Email verification"
	case KeyPasswordReset:
		return "Password reset"
	case KeySignupConfirmation:
		return "Signup confirmation"
	default:
		return slot
	}
}

// Ensures a channel has an editable library email for each transactional slot, so the built-in designs show up as real operator-editable rows in the Transactional group rather than invisible hardcoded fallbacks
// Idempotent - a slot that already has an assigned email, seeded earlier or customized, is left untouched, and the built-in Go render stays the ultimate safety net for any slot left unseeded - Returns how many were created
func (s *Service) SeedTransactional(ctx context.Context, channelID uuid.UUID) (int, error) {
	if channelID == uuid.Nil {
		return 0, fmt.Errorf("channel is required")
	}
	groups, err := s.repo.ListEmailGroups(ctx)
	if err != nil {
		return 0, err
	}
	var txGroup *uuid.UUID
	for _, g := range groups {
		if g.Slug == GroupSlugTransactional {
			id := g.ID
			txGroup = &id
			break
		}
	}
	if txGroup == nil {
		return 0, fmt.Errorf("the %q email group is missing - run migrations first", GroupSlugTransactional)
	}

	created := 0
	for _, slot := range TransactionalSlots {
		if _, ok, err := s.repo.GetSlotEmail(ctx, channelID, slot); err != nil {
			return created, err
		} else if ok {
			continue // already filled - don't clobber a customization
		}
		e := LibraryEmail{
			ChannelID: channelID,
			Name:      SlotLabel(slot),
			Subject:   DefaultSubject(slot),
			Blocks:    DefaultBlocks(slot),
			Styles:    EmailStyles{},
			GroupID:   txGroup,
			Slot:      slot,
		}
		if _, err := s.CreateLibraryEmail(ctx, e); err != nil {
			return created, fmt.Errorf("seed %s: %w", slot, err)
		}
		created++
	}
	return created, nil
}
