package admin

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A definition's unset fields cross as zero values, and come back to the dashboard as null
func boardReportsIn(in []*model.BoardReportInput) []*signalsv1.BoardReport {
	out := make([]*signalsv1.BoardReport, 0, len(in))
	for _, r := range in {
		out = append(out, reportIn(r))
	}
	return out
}

func reportIn(r *model.BoardReportInput) *signalsv1.BoardReport {
	d := r.Definition
	filters := make([]*signalsv1.FunnelFilter, 0, len(d.Filters))
	for _, f := range d.Filters {
		filters = append(filters, &signalsv1.FunnelFilter{Step: f.Step, Key: f.Key, Value: f.Value})
	}
	return &signalsv1.BoardReport{Title: r.Title, Size: strings.ToLower(string(ptrOr(r.Size, ""))), Definition: &signalsv1.BoardReportDefinition{
		Kind: strings.ToLower(string(d.Kind)), SinceDays: ptrOr(d.SinceDays, 0), Event: ptrOr(d.Event, ""),
		Measure: ptrOr(d.Measure, ""), Unit: ptrOr(d.Unit, ""), Breakdown: ptrOr(d.Breakdown, ""), Steps: d.Steps,
		Filters: filters, WindowHours: ptrOr(d.WindowHours, 0), StartEvent: ptrOr(d.StartEvent, ""),
		ReturnEvent: ptrOr(d.ReturnEvent, ""), Periods: ptrOr(d.Periods, 0), Depth: ptrOr(d.Depth, 0),
		WindowDays: ptrOr(d.WindowDays, 0),
	}}
}

// boardPeople names the admins and roles on boards. Signals keeps only their ids, and an owner core no longer has
// is a removed one - which is what lets someone claim the board
type boardPeople struct {
	admins map[uuid.UUID]admin.AdminUser
	roles  map[uuid.UUID]string
}

// boardPeople reads the admins - one query, whatever their number - and the roles only when withRoles: a board names
// roles only on its role shares, and the list never does
func (r *Resolver) boardPeople(ctx context.Context, withRoles bool) (*boardPeople, error) {
	admins, err := r.AdminSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	p := &boardPeople{admins: make(map[uuid.UUID]admin.AdminUser, len(admins)), roles: map[uuid.UUID]string{}}
	for _, a := range admins {
		p.admins[a.ID] = a
	}
	if !withRoles {
		return p, nil
	}
	roles, err := r.AdminSvc.ListRoles(ctx)
	if err != nil {
		return nil, err
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
			Access: model.BoardAccess(strings.ToUpper(b.GetAccess())), Favourite: b.GetFavourite(), Owner: p.owner(b.GetOwnerAdminId()),
			DraftUpdatedAt: timeOrNil(b.GetDraftUpdatedAt()), AccessRequestCount: b.GetAccessRequestCount(),
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

func (p *boardPeople) boardOut(b *signalsv1.Board) *model.Board {
	out := &model.Board{
		ID: b.GetId(), ChannelID: b.GetChannelId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(),
		Reports: []*model.BoardReport{}, Shares: []*model.BoardShare{}, AccessRequests: []*model.BoardAccessRequest{},
		Access: model.BoardAccess(strings.ToUpper(b.GetAccess())), Favourite: b.GetFavourite(), Owner: p.owner(b.GetOwnerAdminId()),
		DraftUpdatedAt: timeOrNil(b.GetDraftUpdatedAt()),
	}
	for _, r := range b.GetReports() {
		out.Reports = append(out.Reports, reportOut(r))
	}
	for _, sh := range b.GetShares() {
		s := &model.BoardShare{Level: model.BoardShareLevel(strings.ToUpper(sh.GetLevel()))}
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
	for _, rq := range b.GetAccessRequests() {
		out.AccessRequests = append(out.AccessRequests, &model.BoardAccessRequest{AdminID: rq.GetAdminId(), Name: p.owner(rq.GetAdminId()).Name, Level: model.BoardShareLevel(strings.ToUpper(rq.GetLevel())), RequestedAt: rq.GetRequestedAt().AsTime()})
	}
	if d := b.GetDraft(); d != nil {
		out.Draft = &model.BoardDraft{Name: d.GetName(), Reports: []*model.BoardDraftReport{}, BaseUpdatedAt: d.GetBaseUpdatedAt().AsTime(), UpdatedAt: d.GetUpdatedAt().AsTime()}
		for _, r := range d.GetReports() {
			dr := &model.BoardDraftReport{Report: reportOut(r.GetReport())}
			if r.From != nil {
				dr.From = new(r.GetFrom())
			}
			out.Draft.Reports = append(out.Draft.Reports, dr)
		}
	}
	return out
}

func reportOut(r *signalsv1.BoardReport) *model.BoardReport {
	d := r.GetDefinition()
	var filters []*model.BoardReportFilter
	for _, f := range d.GetFilters() {
		filters = append(filters, &model.BoardReportFilter{Step: f.GetStep(), Key: f.GetKey(), Value: f.GetValue()})
	}
	return &model.BoardReport{Title: r.GetTitle(), Size: nilIfZero(model.BoardReportSize(strings.ToUpper(r.GetSize()))), Definition: &model.BoardReportDefinition{
		Kind: model.BoardReportKind(strings.ToUpper(d.GetKind())), SinceDays: nilIfZero(d.GetSinceDays()), Event: nilIfZero(d.GetEvent()),
		Measure: nilIfZero(d.GetMeasure()), Unit: nilIfZero(d.GetUnit()), Breakdown: nilIfZero(d.GetBreakdown()), Steps: d.GetSteps(),
		Filters: filters, WindowHours: nilIfZero(d.GetWindowHours()), StartEvent: nilIfZero(d.GetStartEvent()),
		ReturnEvent: nilIfZero(d.GetReturnEvent()), Periods: nilIfZero(d.GetPeriods()), Depth: nilIfZero(d.GetDepth()),
		WindowDays: nilIfZero(d.GetWindowDays()),
	}}
}

func (r *Resolver) oneBoard(ctx context.Context, b *signalsv1.Board) (*model.Board, error) {
	p, err := r.boardPeople(ctx, slices.ContainsFunc(b.GetShares(), func(s *signalsv1.BoardShare) bool { return s.GetRoleId() != "" }))
	if err != nil {
		return nil, err
	}
	return p.boardOut(b), nil
}

func (r *Resolver) manyBoards(ctx context.Context, bs []*signalsv1.BoardSummary) ([]*model.BoardSummary, error) {
	p, err := r.boardPeople(ctx, false)
	if err != nil {
		return nil, err
	}
	return p.summariesOut(bs), nil
}

// shareTargets is every active admin and every role, each with whether they'd see the board and its numbers on this
// shop - the share dialog warns rather than refuses, since a grant can be added after the share
func (r *Resolver) shareTargets(ctx context.Context, channelID uuid.UUID) ([]*model.BoardShareTarget, error) {
	sees := func(id uuid.UUID, superuser bool, grants []auth.Grant) (bool, bool) {
		return boardSight(ctx, id, superuser, grants, channelID)
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

// boardSight is whether someone with these grants would open boards on the shop, and see the numbers on them
func boardSight(ctx context.Context, id uuid.UUID, superuser bool, grants []auth.Grant, channelID uuid.UUID) (boards, data bool) {
	if superuser {
		return true, true
	}
	c := auth.NewGrantChecker(id, auth.NewGrantSet(grants))
	boards, _ = c.HasChannelPermission(ctx, id, "boards.view", channelID, nil)
	data, _ = c.HasChannelPermission(ctx, id, "customers.view", channelID, nil)
	return boards, data
}

// checkBoardOwner holds a board's new owner to an active admin who can open boards on its shop - anyone else would
// leave it orphaned the moment it's handed over. Signals can't know who that is, so core checks before relaying
func (r *Resolver) checkBoardOwner(ctx context.Context, channelID, adminID string) error {
	refuse := apperr.New("BOARD_OWNER_INVALID", apperr.ClassValidation, "a board can only be handed to an active admin who can open boards on this shop")
	aid, err := uuid.Parse(adminID)
	if err != nil {
		return refuse
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return refuse
	}
	a, err := r.AdminSvc.GetByID(ctx, aid)
	if errors.Is(err, admin.ErrAdminNotFound) || (err == nil && !a.IsActive) {
		return refuse
	}
	if err != nil {
		return err
	}
	grants, err := r.AdminSvc.Grants(ctx, aid)
	if err != nil {
		return err
	}
	if boards, _ := boardSight(ctx, aid, a.IsSuperuser, grants, cid); !boards {
		return refuse
	}
	return nil
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

// notifyBoardAccessRequest puts a request for access in the board owner's inbox, theirs alone. Signals names the owner
// only when it kept the request, so nothing here tells the asker whether the board exists
func (r *Resolver) notifyBoardAccessRequest(ctx context.Context, channelID, boardID string, level model.BoardShareLevel, res *signalsv1.RequestBoardAccessResponse) error {
	if res.GetOwnerId() == "" || r.NotificationSvc == nil {
		return nil
	}
	owner, err := uuid.Parse(res.GetOwnerId())
	if err != nil {
		return err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return err
	}
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return err
	}
	asker, err := r.AdminSvc.GetByID(ctx, claims.AdminID)
	if err != nil {
		return err
	}
	lvl := strings.ToLower(string(level))
	return r.NotificationSvc.Create(ctx, notification.Input{
		Category:           notification.CategoryBoard,
		Title:              adminName(asker) + " asked to " + lvl + " " + res.GetBoardName(),
		Body:               "Share the board with them, or dismiss the request, from the board.",
		RequiredPermission: notification.PermBoards,
		ChannelID:          &cid,
		EntityType:         "board",
		EntityID:           boardID,
		ActionURL:          "/synapse/boards/" + boardID,
		// One notice a day per asker and level - asking again before the owner answers adds nothing
		DedupeKey:        "board_access:" + boardID + ":" + claims.AdminID.String() + ":" + lvl + ":" + time.Now().UTC().Format(time.DateOnly),
		RecipientAdminID: &owner,
	})
}
