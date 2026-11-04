package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A report definition crosses as JSON text - Signals reads it strictly and answers what it refuses
func boardReportsIn(in []*model.BoardReportInput) ([]*signalsv1.BoardReport, error) {
	out := make([]*signalsv1.BoardReport, 0, len(in))
	for _, r := range in {
		raw, err := json.Marshal(r.Definition)
		if err != nil {
			return nil, fmt.Errorf("report %q: definition: %w", r.Title, err)
		}
		out = append(out, &signalsv1.BoardReport{Title: r.Title, DefinitionJson: string(raw)})
	}
	return out, nil
}

// boardPeople names the admins and roles on boards. Signals keeps only their ids, and an owner core no longer has
// is a removed one - which is what lets someone claim the board
type boardPeople struct {
	admins map[uuid.UUID]admin.AdminUser
	roles  map[uuid.UUID]string
}

func (r *Resolver) boardPeople(ctx context.Context) (*boardPeople, error) {
	admins, err := r.AdminSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := r.AdminSvc.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	p := &boardPeople{admins: make(map[uuid.UUID]admin.AdminUser, len(admins)), roles: make(map[uuid.UUID]string, len(roles))}
	for _, a := range admins {
		p.admins[a.ID] = a
	}
	for _, ro := range roles {
		p.roles[ro.ID] = ro.Name
	}
	return p, nil
}

func adminName(a admin.AdminUser) string {
	if n := strings.TrimSpace(a.FirstName + " " + a.LastName); n != "" {
		return n
	}
	return a.Email
}

func (p *boardPeople) owner(id string) *model.BoardOwner {
	if id == "" {
		return nil // a shop board
	}
	uid, _ := uuid.Parse(id)
	a, ok := p.admins[uid]
	if !ok {
		return &model.BoardOwner{ID: id, Name: "A removed admin", Removed: true}
	}
	return &model.BoardOwner{ID: id, Name: adminName(a)}
}

func (p *boardPeople) boardsOut(in []*signalsv1.Board) ([]*model.Board, error) {
	out := make([]*model.Board, 0, len(in))
	for _, b := range in {
		m, err := p.boardOut(b)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (p *boardPeople) boardOut(b *signalsv1.Board) (*model.Board, error) {
	out := &model.Board{
		ID: b.GetId(), ChannelID: b.GetChannelId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(),
		ReportCount: b.GetReportCount(), Reports: []*model.BoardReport{}, Shares: []*model.BoardShare{},
		Status: b.GetStatus(), Access: b.GetAccess(), Favourite: b.GetFavourite(), Owner: p.owner(b.GetOwnerAdminId()),
	}
	if t := b.GetDraftUpdatedAt(); t != nil {
		at := t.AsTime()
		out.DraftUpdatedAt = &at
	}
	for _, r := range b.GetReports() {
		var def any
		if err := json.Unmarshal([]byte(r.GetDefinitionJson()), &def); err != nil {
			return nil, fmt.Errorf("report %q: definition: %w", r.GetTitle(), err)
		}
		out.Reports = append(out.Reports, &model.BoardReport{Title: r.GetTitle(), Definition: def})
	}
	for _, sh := range b.GetShares() {
		s := &model.BoardShare{Level: sh.GetLevel()}
		if id := sh.GetAdminId(); id != "" {
			s.AdminID, s.Name = &id, p.owner(id).Name
		} else {
			id, uid := sh.GetRoleId(), uuid.MustParse(sh.GetRoleId())
			s.RoleID, s.Name = &id, p.roles[uid]
			if s.Name == "" {
				s.Name = "A removed role"
			}
		}
		out.Shares = append(out.Shares, s)
	}
	if d := b.GetDraft(); d != nil {
		var payload any
		if err := json.Unmarshal([]byte(d.GetPayloadJson()), &payload); err != nil {
			return nil, fmt.Errorf("board draft: %w", err)
		}
		out.Draft = &model.BoardDraft{Payload: payload, BaseUpdatedAt: d.GetBaseUpdatedAt().AsTime(), UpdatedAt: d.GetUpdatedAt().AsTime()}
	}
	return out, nil
}

func (r *Resolver) oneBoard(ctx context.Context, b *signalsv1.Board) (*model.Board, error) {
	p, err := r.boardPeople(ctx)
	if err != nil {
		return nil, err
	}
	return p.boardOut(b)
}

func (r *Resolver) manyBoards(ctx context.Context, bs []*signalsv1.Board) ([]*model.Board, error) {
	p, err := r.boardPeople(ctx)
	if err != nil {
		return nil, err
	}
	return p.boardsOut(bs)
}

// shareTargets is every active admin and every role, each with whether they'd see the board and its numbers on this
// shop - the share dialog warns rather than refuses, since a grant can be added after the share
func (r *Resolver) shareTargets(ctx context.Context, channelID uuid.UUID) ([]*model.BoardShareTarget, error) {
	sees := func(id uuid.UUID, superuser bool, grants []auth.Grant) (bool, bool) {
		if superuser {
			return true, true
		}
		c := auth.NewGrantChecker(id, auth.NewGrantSet(grants))
		boards, _ := c.HasChannelPermission(ctx, id, "boards.view", channelID, nil)
		data, _ := c.HasChannelPermission(ctx, id, "customers.view", channelID, nil)
		return boards, data
	}
	admins, err := r.AdminSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []*model.BoardShareTarget{}
	for _, a := range admins {
		if !a.IsActive {
			continue
		}
		grants, err := r.AdminSvc.Grants(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		b, d := sees(a.ID, a.IsSuperuser, grants)
		out = append(out, &model.BoardShareTarget{Kind: "admin", ID: a.ID.String(), Name: adminName(a), SeesBoards: b, SeesData: d})
	}
	roles, err := r.AdminSvc.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for _, ro := range roles {
		full, err := r.AdminSvc.GetRole(ctx, ro.ID)
		if err != nil {
			return nil, err
		}
		grants := make([]auth.Grant, 0, len(full.PermissionScopes))
		for _, sc := range full.PermissionScopes {
			grants = append(grants, auth.Grant{Slug: sc.Permission.Slug, ChannelID: sc.ChannelID, Locale: sc.Locale})
		}
		b, d := sees(uuid.Nil, false, grants)
		out = append(out, &model.BoardShareTarget{Kind: "role", ID: ro.ID.String(), Name: ro.Name, SeesBoards: b, SeesData: d})
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func propertyFiltersIn(in []*model.SynapsePropertyFilterInput) []*signalsv1.PropertyFilter {
	out := make([]*signalsv1.PropertyFilter, 0, len(in))
	for _, f := range in {
		out = append(out, &signalsv1.PropertyFilter{Key: strings.TrimSpace(f.Key), Op: f.Op, Values: f.Values})
	}
	return out
}
