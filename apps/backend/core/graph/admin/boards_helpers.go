package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A definition's unset fields cross as zero values, and come back to the dashboard as null
func boardReportsIn(in []*model.BoardReportInput) []*signalsv1.BoardReport {
	out := make([]*signalsv1.BoardReport, 0, len(in))
	for _, r := range in {
		d := r.Definition
		filters := make([]*signalsv1.FunnelFilter, 0, len(d.Filters))
		for _, f := range d.Filters {
			filters = append(filters, &signalsv1.FunnelFilter{Step: f.Step, Key: f.Key, Value: f.Value})
		}
		out = append(out, &signalsv1.BoardReport{Title: r.Title, Size: strings.ToLower(string(ptrOr(r.Size, ""))), Definition: &signalsv1.BoardReportDefinition{
			Kind: strings.ToLower(string(d.Kind)), SinceDays: ptrOr(d.SinceDays, 0), Event: ptrOr(d.Event, ""),
			Measure: ptrOr(d.Measure, ""), Unit: ptrOr(d.Unit, ""), Breakdown: ptrOr(d.Breakdown, ""), Steps: d.Steps,
			Filters: filters, WindowHours: ptrOr(d.WindowHours, 0), StartEvent: ptrOr(d.StartEvent, ""),
			ReturnEvent: ptrOr(d.ReturnEvent, ""), Periods: ptrOr(d.Periods, 0), Depth: ptrOr(d.Depth, 0),
			WindowDays: ptrOr(d.WindowDays, 0),
		}})
	}
	return out
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

func (p *boardPeople) summariesOut(in []*signalsv1.BoardSummary) []*model.BoardSummary {
	out := make([]*model.BoardSummary, 0, len(in))
	for _, b := range in {
		out = append(out, &model.BoardSummary{
			ID: b.GetId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(), ReportCount: b.GetReportCount(),
			Status: b.GetStatus(), Access: b.GetAccess(), Favourite: b.GetFavourite(), Owner: p.owner(b.GetOwnerAdminId()),
			DraftUpdatedAt: timeOrNil(b.GetDraftUpdatedAt()),
		})
	}
	return out
}

// timeOrNil is an unset timestamp as null
func timeOrNil(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	at := t.AsTime()
	return &at
}

func (p *boardPeople) boardOut(b *signalsv1.Board) (*model.Board, error) {
	out := &model.Board{
		ID: b.GetId(), ChannelID: b.GetChannelId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(),
		Reports: []*model.BoardReport{}, Shares: []*model.BoardShare{},
		Status: b.GetStatus(), Access: b.GetAccess(), Favourite: b.GetFavourite(), Owner: p.owner(b.GetOwnerAdminId()),
		DraftUpdatedAt: timeOrNil(b.GetDraftUpdatedAt()),
	}
	for _, r := range b.GetReports() {
		d := r.GetDefinition()
		var filters []*model.BoardReportFilter
		for _, f := range d.GetFilters() {
			filters = append(filters, &model.BoardReportFilter{Step: f.GetStep(), Key: f.GetKey(), Value: f.GetValue()})
		}
		out.Reports = append(out.Reports, &model.BoardReport{Title: r.GetTitle(), Size: nilIfZero(model.BoardReportSize(strings.ToUpper(r.GetSize()))), Definition: &model.BoardReportDefinition{
			Kind: model.BoardReportKind(strings.ToUpper(d.GetKind())), SinceDays: nilIfZero(d.GetSinceDays()), Event: nilIfZero(d.GetEvent()),
			Measure: nilIfZero(d.GetMeasure()), Unit: nilIfZero(d.GetUnit()), Breakdown: nilIfZero(d.GetBreakdown()), Steps: d.GetSteps(),
			Filters: filters, WindowHours: nilIfZero(d.GetWindowHours()), StartEvent: nilIfZero(d.GetStartEvent()),
			ReturnEvent: nilIfZero(d.GetReturnEvent()), Periods: nilIfZero(d.GetPeriods()), Depth: nilIfZero(d.GetDepth()),
			WindowDays: nilIfZero(d.GetWindowDays()),
		}})
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

func (r *Resolver) manyBoards(ctx context.Context, bs []*signalsv1.BoardSummary) ([]*model.BoardSummary, error) {
	p, err := r.boardPeople(ctx)
	if err != nil {
		return nil, err
	}
	return p.summariesOut(bs), nil
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

func nilIfZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
