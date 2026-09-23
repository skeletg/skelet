package signalsgrpc

// The email builder's admin surface, relayed by core with the caller's grants: the saved emails,
// their groups and slots, version history, the pre-send checklist and the reusable blocks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

type builderServer struct {
	signalsv1.UnimplementedEmailBuilderServiceServer
	svc      *emailtemplate.Service
	stores   emailtemplate.StoreReader
	contacts previewContacts
}

// One roster entry, for a preview rendered as a real customer
type previewContacts interface {
	Get(ctx context.Context, customerID uuid.UUID) (previewContact, bool, error)
}

type previewContact struct {
	Email     string
	FirstName string
}

func (s *builderServer) ListLibraryEmails(ctx context.Context, req *signalsv1.ListLibraryEmailsRequest) (*signalsv1.ListLibraryEmailsResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "emails.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	list, err := s.svc.ListLibraryEmails(ctx, cid, req.GetGroupSlug())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.LibraryEmail, 0, len(list))
	for _, e := range list {
		out = append(out, libraryEmailProto(e))
	}
	return &signalsv1.ListLibraryEmailsResponse{Emails: out}, nil
}

func (s *builderServer) GetLibraryEmail(ctx context.Context, req *signalsv1.GetLibraryEmailRequest) (*signalsv1.GetLibraryEmailResponse, error) {
	e, err := s.load(ctx, "emails.view", req.GetId())
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetLibraryEmailResponse{Email: libraryEmailProto(e)}, nil
}

func (s *builderServer) SaveLibraryEmail(ctx context.Context, req *signalsv1.SaveLibraryEmailRequest) (*signalsv1.SaveLibraryEmailResponse, error) {
	in := req.GetEmail()
	if in == nil {
		return nil, invalidArgument("email is required")
	}
	e, err := libraryEmailInput(in)
	if err != nil {
		return nil, err
	}
	var saved emailtemplate.LibraryEmail
	if e.ID != uuid.Nil {
		existing, err := s.load(ctx, "emails.manage", in.GetId())
		if err != nil {
			return nil, err
		}
		e.ChannelID = existing.ChannelID
		saved, err = s.svc.UpdateLibraryEmail(ctx, e)
		if err != nil {
			return nil, domainErr(err)
		}
	} else {
		if _, err := channelScopeOrSystem(ctx, "emails.manage", in.GetChannelId()); err != nil {
			return nil, err
		}
		saved, err = s.svc.CreateLibraryEmail(ctx, e)
		if err != nil {
			return nil, domainErr(err)
		}
	}
	// Snapshot the saved state into version history (best-effort)
	_ = s.svc.Snapshot(ctx, saved, callingAdmin(ctx))
	return &signalsv1.SaveLibraryEmailResponse{Email: libraryEmailProto(saved)}, nil
}

func (s *builderServer) DeleteLibraryEmail(ctx context.Context, req *signalsv1.DeleteLibraryEmailRequest) (*signalsv1.DeleteLibraryEmailResponse, error) {
	e, err := s.load(ctx, "emails.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteLibraryEmail(ctx, e.ID); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteLibraryEmailResponse{}, nil
}

// Groups are global rather than per channel, so the permission alone gates them
func (s *builderServer) ListEmailGroups(ctx context.Context, _ *signalsv1.ListEmailGroupsRequest) (*signalsv1.ListEmailGroupsResponse, error) {
	if err := requirePermissionOrSystem(ctx, "emails.view"); err != nil {
		return nil, err
	}
	groups, err := s.svc.ListEmailGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.EmailGroup, 0, len(groups))
	for _, g := range groups {
		count, _ := s.svc.CountEmailsInGroup(ctx, g.ID)
		out = append(out, groupProto(g, count))
	}
	return &signalsv1.ListEmailGroupsResponse{Groups: out}, nil
}

func (s *builderServer) CreateEmailGroup(ctx context.Context, req *signalsv1.CreateEmailGroupRequest) (*signalsv1.CreateEmailGroupResponse, error) {
	if err := requirePermissionOrSystem(ctx, "emails.manage"); err != nil {
		return nil, err
	}
	g, err := s.svc.CreateEmailGroup(ctx, req.GetName())
	if err != nil {
		return nil, domainErr(err)
	}
	return &signalsv1.CreateEmailGroupResponse{Group: groupProto(g, 0)}, nil
}

func (s *builderServer) RenameEmailGroup(ctx context.Context, req *signalsv1.RenameEmailGroupRequest) (*signalsv1.RenameEmailGroupResponse, error) {
	if err := requirePermissionOrSystem(ctx, "emails.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	g, err := s.svc.RenameEmailGroup(ctx, id, req.GetName())
	if err != nil {
		return nil, domainErr(err)
	}
	return &signalsv1.RenameEmailGroupResponse{Group: groupProto(g, 0)}, nil
}

func (s *builderServer) DeleteEmailGroup(ctx context.Context, req *signalsv1.DeleteEmailGroupRequest) (*signalsv1.DeleteEmailGroupResponse, error) {
	if err := requirePermissionOrSystem(ctx, "emails.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteEmailGroup(ctx, id); err != nil {
		return nil, domainErr(err)
	}
	return &signalsv1.DeleteEmailGroupResponse{}, nil
}

func (s *builderServer) ListTransactionalSlots(ctx context.Context, req *signalsv1.ListTransactionalSlotsRequest) (*signalsv1.ListTransactionalSlotsResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "emails.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.TransactionalSlot, 0, len(emailtemplate.TransactionalSlots))
	for _, slot := range emailtemplate.TransactionalSlots {
		ts := &signalsv1.TransactionalSlot{Slot: slot, Label: emailtemplate.SlotLabel(slot)}
		if e, ok, _ := s.svc.SlotEmail(ctx, cid, slot); ok {
			id := e.ID.String()
			ts.EmailId = &id
			ts.EmailName = e.Name
		}
		out = append(out, ts)
	}
	return &signalsv1.ListTransactionalSlotsResponse{Slots: out}, nil
}

func (s *builderServer) AssignEmailSlot(ctx context.Context, req *signalsv1.AssignEmailSlotRequest) (*signalsv1.AssignEmailSlotResponse, error) {
	e, err := s.load(ctx, "emails.manage", req.GetEmailId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.AssignSlot(ctx, e.ID, req.GetSlot()); err != nil {
		return nil, domainErr(err)
	}
	return &signalsv1.AssignEmailSlotResponse{}, nil
}

func (s *builderServer) PreflightEmail(ctx context.Context, req *signalsv1.PreflightEmailRequest) (*signalsv1.PreflightEmailResponse, error) {
	e, err := s.load(ctx, "emails.view", req.GetId())
	if err != nil {
		return nil, err
	}
	warnings := emailtemplate.Preflight(e.Subject, e.Preheader, e.Blocks)
	out := make([]*signalsv1.EmailWarning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, &signalsv1.EmailWarning{Severity: w.Severity, Message: w.Message})
	}
	return &signalsv1.PreflightEmailResponse{Warnings: out}, nil
}

func (s *builderServer) ListEmailRevisions(ctx context.Context, req *signalsv1.ListEmailRevisionsRequest) (*signalsv1.ListEmailRevisionsResponse, error) {
	e, err := s.load(ctx, "emails.view", req.GetId())
	if err != nil {
		return nil, err
	}
	revs, err := s.svc.ListRevisions(ctx, e.ID, req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.EmailRevision, 0, len(revs))
	for _, r := range revs {
		out = append(out, &signalsv1.EmailRevision{
			Id: r.ID.String(), Name: r.Name, Subject: r.Subject, Preheader: r.Preheader,
			BlocksJson: blocksJSON(r.Blocks), StylesJson: stylesJSON(r.Styles), CreatedAt: ts(r.CreatedAt),
		})
	}
	return &signalsv1.ListEmailRevisionsResponse{Revisions: out}, nil
}

func (s *builderServer) RestoreEmailRevision(ctx context.Context, req *signalsv1.RestoreEmailRevisionRequest) (*signalsv1.RestoreEmailRevisionResponse, error) {
	if err := requirePermissionOrSystem(ctx, "emails.manage"); err != nil {
		return nil, err
	}
	rid, err := parseID("revision_id", req.GetRevisionId())
	if err != nil {
		return nil, err
	}
	e, err := s.svc.RestoreRevision(ctx, rid, callingAdmin(ctx))
	if err != nil {
		return nil, domainErr(err)
	}
	if err := requireChannelOrSystem(ctx, "emails.manage", e.ChannelID); err != nil {
		return nil, err
	}
	return &signalsv1.RestoreEmailRevisionResponse{Email: libraryEmailProto(e)}, nil
}

func (s *builderServer) RenderLibraryPreview(ctx context.Context, req *signalsv1.RenderLibraryPreviewRequest) (*signalsv1.RenderLibraryPreviewResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "emails.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	blocks, err := blocksFromJSON(req.GetBlocksJson())
	if err != nil {
		return nil, err
	}
	styles, err := stylesFromJSON(req.GetStylesJson())
	if err != nil {
		return nil, err
	}
	vars, err := s.previewVars(ctx, cid, req.CustomerId)
	if err != nil {
		return nil, err
	}
	subject, html, _, err := s.svc.RenderContent(ctx, cid, req.GetSubject(), req.GetPreheader(), blocks, styles, vars)
	if err != nil {
		return nil, domainErr(err)
	}
	return &signalsv1.RenderLibraryPreviewResponse{Subject: subject, Html: html}, nil
}

// The merge tags a live editor preview renders with - a named customer's real data off the roster,
// otherwise representative sample data so tags don't show up as literal {{...}}
func (s *builderServer) previewVars(ctx context.Context, channelID uuid.UUID, customerID *string) (map[string]string, error) {
	var store emailtemplate.Store
	if s.stores != nil {
		if st, err := s.stores.Get(ctx, channelID); err == nil {
			store = st
		}
	}
	storefront := strings.TrimRight(strings.TrimSpace(store.PublicURL), "/")
	vars := map[string]string{
		"store.name":     store.Name,
		"store.logoUrl":  strings.TrimSpace(store.LogoURL),
		"storefront.url": storefront,
		// Sample order tags so previews render something sensible
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
	}
	if customerID != nil && strings.TrimSpace(*customerID) != "" && s.contacts != nil {
		id, err := parseID("customer_id", *customerID)
		if err != nil {
			return nil, err
		}
		c, ok, err := s.contacts.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, notFound("customer")
		}
		if c.FirstName != "" {
			vars["customer.firstName"] = c.FirstName
		}
		if c.Email != "" {
			vars["customer.email"] = c.Email
		}
	}
	return vars, nil
}

func (s *builderServer) ListEmailBlocks(ctx context.Context, req *signalsv1.ListEmailBlocksRequest) (*signalsv1.ListEmailBlocksResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "emails.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	snips, err := s.svc.ListBlocks(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.EmailBlock, 0, len(snips))
	for _, sn := range snips {
		out = append(out, snippetProto(sn))
	}
	return &signalsv1.ListEmailBlocksResponse{Blocks: out}, nil
}

func (s *builderServer) SaveEmailBlock(ctx context.Context, req *signalsv1.SaveEmailBlockRequest) (*signalsv1.SaveEmailBlockResponse, error) {
	in := req.GetBlock()
	cid, err := channelScopeOrSystem(ctx, "emails.manage", in.GetChannelId())
	if err != nil {
		return nil, err
	}
	blocks, err := blocksFromJSON(in.GetBlocksJson())
	if err != nil {
		return nil, err
	}
	var fields []emailtemplate.FieldDef
	if raw := strings.TrimSpace(in.GetFieldsJson()); raw != "" {
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			return nil, invalidArgument("fields must be a JSON array of {key,label,kind,...}: %v", err)
		}
	}
	sn := emailtemplate.Snippet{
		ChannelID: cid, Slug: in.GetSlug(), Name: in.GetName(), Icon: in.GetIcon(), Fields: fields, Blocks: blocks,
		HTMLTemplate: in.GetHtmlTemplate(), TextTemplate: in.GetTextTemplate(), CSS: in.GetCss(), Scripted: in.GetScripted(),
	}
	if err := s.svc.SaveBlock(ctx, sn); err != nil {
		return nil, domainErr(err)
	}
	return &signalsv1.SaveEmailBlockResponse{Block: snippetProto(sn)}, nil
}

func (s *builderServer) DeleteEmailBlock(ctx context.Context, req *signalsv1.DeleteEmailBlockRequest) (*signalsv1.DeleteEmailBlockResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "emails.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteBlock(ctx, cid, req.GetSlug()); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteEmailBlockResponse{}, nil
}

func (s *builderServer) load(ctx context.Context, perm, rawID string) (emailtemplate.LibraryEmail, error) {
	if err := requirePermissionOrSystem(ctx, perm); err != nil {
		return emailtemplate.LibraryEmail{}, err
	}
	id, err := parseID("id", rawID)
	if err != nil {
		return emailtemplate.LibraryEmail{}, err
	}
	e, ok, err := s.svc.GetLibraryEmail(ctx, id)
	if err != nil {
		return emailtemplate.LibraryEmail{}, err
	}
	if !ok {
		return emailtemplate.LibraryEmail{}, notFound("email")
	}
	if err := requireChannelOrSystem(ctx, perm, e.ChannelID); err != nil {
		return emailtemplate.LibraryEmail{}, err
	}
	return e, nil
}

func libraryEmailInput(in *signalsv1.LibraryEmailInput) (emailtemplate.LibraryEmail, error) {
	e := emailtemplate.LibraryEmail{Name: in.GetName(), Subject: in.GetSubject(), Preheader: in.GetPreheader()}
	if raw := strings.TrimSpace(in.GetId()); raw != "" {
		id, err := parseID("id", raw)
		if err != nil {
			return e, err
		}
		e.ID = id
	}
	if raw := strings.TrimSpace(in.GetChannelId()); raw != "" {
		cid, err := parseID("channel_id", raw)
		if err != nil {
			return e, err
		}
		e.ChannelID = cid
	}
	blocks, err := blocksFromJSON(in.GetBlocksJson())
	if err != nil {
		return e, err
	}
	e.Blocks = blocks
	if e.Styles, err = stylesFromJSON(in.GetStylesJson()); err != nil {
		return e, err
	}
	if e.GroupID, err = optionalIDFrom(in.GroupId, "group_id"); err != nil {
		return e, err
	}
	return e, nil
}

func libraryEmailProto(e emailtemplate.LibraryEmail) *signalsv1.LibraryEmail {
	return &signalsv1.LibraryEmail{
		Id: e.ID.String(), ChannelId: e.ChannelID.String(), Name: e.Name, Subject: e.Subject, Preheader: e.Preheader,
		BlocksJson: blocksJSON(e.Blocks), StylesJson: stylesJSON(e.Styles), GroupId: optionalIDString(e.GroupID), Slot: e.Slot,
		CreatedAt: ts(e.CreatedAt), UpdatedAt: ts(e.UpdatedAt),
	}
}

func groupProto(g emailtemplate.EmailGroup, count int64) *signalsv1.EmailGroup {
	return &signalsv1.EmailGroup{Id: g.ID.String(), Slug: g.Slug, Name: g.Name, IsSystem: g.IsSystem, EmailCount: int32(count)}
}

func snippetProto(sn emailtemplate.Snippet) *signalsv1.EmailBlock {
	fields := sn.Fields
	if fields == nil {
		fields = []emailtemplate.FieldDef{}
	}
	rawFields, _ := json.Marshal(fields)
	return &signalsv1.EmailBlock{
		ChannelId: sn.ChannelID.String(), Slug: sn.Slug, Name: sn.Name, Icon: sn.Icon, FieldsJson: string(rawFields),
		BlocksJson: blocksJSON(sn.Blocks), HtmlTemplate: sn.HTMLTemplate, TextTemplate: sn.TextTemplate, Css: sn.CSS, Scripted: sn.Scripted,
	}
}
