package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/emailtemplate"
)

func libraryEmailInput(in model.SaveLibraryEmailInput) (emailtemplate.LibraryEmail, error) {
	cid, err := uuid.Parse(in.ChannelID)
	if err != nil {
		return emailtemplate.LibraryEmail{}, errInvalidID("channelId", err)
	}
	blocks, err := blocksFromJSON(in.Blocks)
	if err != nil {
		return emailtemplate.LibraryEmail{}, err
	}
	styles, err := stylesFromJSON(in.Styles)
	if err != nil {
		return emailtemplate.LibraryEmail{}, err
	}
	e := emailtemplate.LibraryEmail{ChannelID: cid, Name: in.Name, Subject: in.Subject, Blocks: blocks, Styles: styles}
	if in.Preheader != nil {
		e.Preheader = *in.Preheader
	}
	if in.GroupID != nil && *in.GroupID != "" {
		gid, err := uuid.Parse(*in.GroupID)
		if err != nil {
			return emailtemplate.LibraryEmail{}, errInvalidID("groupId", err)
		}
		e.GroupID = &gid
	}
	if in.ID != nil && *in.ID != "" {
		id, err := uuid.Parse(*in.ID)
		if err != nil {
			return emailtemplate.LibraryEmail{}, errInvalidID("id", err)
		}
		e.ID = id
	}
	return e, nil
}

func toLibraryEmailModel(e emailtemplate.LibraryEmail) *model.LibraryEmail {
	m := &model.LibraryEmail{
		ID:        e.ID.String(),
		ChannelID: e.ChannelID.String(),
		Name:      e.Name,
		Subject:   e.Subject,
		Preheader: e.Preheader,
		Blocks:    blocksOrEmpty(e.Blocks),
		Styles:    e.Styles,
		Slot:      e.Slot,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if e.GroupID != nil {
		g := e.GroupID.String()
		m.GroupID = &g
	}
	return m
}

func emailGroupModel(g emailtemplate.EmailGroup, count int) *model.EmailGroup {
	return &model.EmailGroup{
		ID: g.ID.String(), Slug: g.Slug, Name: g.Name, IsSystem: g.IsSystem, EmailCount: int32(count),
	}
}

func emailRevisionModel(rev emailtemplate.EmailRevision) *model.EmailRevision {
	return &model.EmailRevision{
		ID:        rev.ID.String(),
		Name:      rev.Name,
		Subject:   rev.Subject,
		Preheader: rev.Preheader,
		Blocks:    blocksOrEmpty(rev.Blocks),
		Styles:    rev.Styles,
		CreatedAt: rev.CreatedAt,
	}
}

// previewVars builds merge-tag substitutions for a live editor preview. With a customerID it renders that customer's
// real data; otherwise representative sample data so tags don't show up as literal {{...}}
func (r *Resolver) previewVars(ctx context.Context, channelID uuid.UUID, customerID *string) (map[string]string, error) {
	ch, err := r.ChannelSvc.GetByID(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("channel: %w", err)
	}
	storefront := strings.TrimRight(strings.TrimSpace(ch.PublicURL), "/")
	logo := ""
	if r.ConfigSvc != nil {
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
		logo, _ = r.ConfigSvc.GetString(ctx, "general/branding/logo_url", scope)
	}
	vars := map[string]string{
		"store.name":     ch.Name,
		"store.logoUrl":  strings.TrimSpace(logo),
		"storefront.url": storefront,
		// Sample order/promo tags so previews render something sensible.
		"order.number":       "1001",
		"order.total":        "$129.00",
		"order.subtotal":     "$119.00",
		"order.shipping":     "$10.00",
		"order.tax":          "$0.00",
		"order.discount":     "$0.00",
		"unsubscribe.link":   storefront + "/unsubscribe/sample",
		"customer.firstName": "Alex",
		"customer.lastName":  "Rivera",
		"customer.email":     "alex@example.com",
		"customer.group":     "Retail",
	}
	if customerID != nil && *customerID != "" {
		cuid, err := uuid.Parse(*customerID)
		if err != nil {
			return nil, errInvalidID("customerId", err)
		}
		cust, err := r.CustomerSvc.GetByID(ctx, cuid)
		if err != nil {
			return nil, fmt.Errorf("customer: %w", err)
		}
		if cust.FirstName != "" {
			vars["customer.firstName"] = cust.FirstName
		}
		if cust.LastName != "" {
			vars["customer.lastName"] = cust.LastName
		}
		if cust.Email != "" {
			vars["customer.email"] = cust.Email
		}
		if cust.Group != nil && cust.Group.Name != "" {
			vars["customer.group"] = cust.Group.Name
		}
	}
	return vars, nil
}

// slotLabel is the human label for a transactional slot on the Templates tab.
// Delegates to the shared emailtemplate.SlotLabel so labels stay in one place.
func slotLabel(slot string) string { return emailtemplate.SlotLabel(slot) }
