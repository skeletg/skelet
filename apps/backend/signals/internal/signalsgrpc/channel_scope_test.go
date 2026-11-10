package signalsgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

func grantOn(slug string, channelID uuid.UUID) auth.Grant {
	return auth.Grant{Slug: slug, ChannelID: &channelID}
}

func TestAChannelGrantCannotNameAnotherChannel(t *testing.T) {
	conn := testConn(t)
	prism, pulse := signalsv1.NewPrismServiceClient(conn), signalsv1.NewPulseServiceClient(conn)
	assistant := signalsv1.NewAssistantServiceClient(conn)
	mine, theirs := uuid.New(), uuid.New()
	ctx := tokenCtx(t,
		grantOn("experiments.manage", mine), grantOn("emails.view", mine), grantOn("assistant.view", mine))
	other := theirs.String()

	for name, call := range map[string]func() error{
		"ListExperiments": func() error {
			_, e := prism.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: other})
			return e
		},
		"GetExperimentsOverview": func() error {
			_, e := prism.GetExperimentsOverview(ctx, &signalsv1.GetExperimentsOverviewRequest{ChannelId: other})
			return e
		},
		"ListGoalEvents": func() error {
			_, e := prism.ListGoalEvents(ctx, &signalsv1.ListGoalEventsRequest{ChannelId: other})
			return e
		},
		"CreateExperiment": func() error {
			_, e := prism.CreateExperiment(ctx, &signalsv1.CreateExperimentRequest{Experiment: &signalsv1.ExperimentInput{ChannelId: other, Key: "k", Name: "n"}})
			return e
		},
		"GetEngagement": func() error {
			_, e := pulse.GetEngagement(ctx, &signalsv1.GetEngagementRequest{ChannelId: other})
			return e
		},
		"ListAssistantConversations": func() error {
			_, e := assistant.ListAssistantConversations(ctx, &signalsv1.ListAssistantConversationsRequest{ChannelId: other})
			return e
		},
		"GetAssistantConversation": func() error {
			_, e := assistant.GetAssistantConversation(ctx, &signalsv1.GetAssistantConversationRequest{ChannelId: other, ConversationId: uuid.NewString()})
			return e
		},
		"GetAssistantTurnRounds": func() error {
			_, e := assistant.GetAssistantTurnRounds(ctx, &signalsv1.GetAssistantTurnRoundsRequest{ChannelId: other, TurnId: uuid.NewString()})
			return e
		},
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s on another channel answered %s, want PermissionDenied", name, got)
		}
	}
	if got := status.Code(mustErr(prism.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: mine.String()}))); got == codes.PermissionDenied {
		t.Error("a grant was refused on its own channel")
	}
}

func channelScopeServer(t *testing.T) (*signalsdb.Queries, Deps) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	qs := signalsdb.New(conn)
	return qs, Deps{
		ServiceKeys: testKeys,
		Channels:    analytics.NewChannelRoster(nil),
		Experiment:  experiment.NewService(experiment.NewRepository(qs, nil)),
		Lens:        lens.NewService(qs, nil, nil, nil),
		Builder:     emailtemplate.NewService(emailtemplate.NewRepository(qs), nil),
	}
}

func TestAnExperimentByIdIsGatedOnItsOwnChannel(t *testing.T) {
	qs, deps := channelScopeServer(t)
	c := signalsv1.NewPrismServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	exp, err := deps.Experiment.Create(context.Background(), experiment.Input{
		ChannelID: theirs, Key: "hero", Name: "Hero", Status: "draft", Coverage: 100,
		Variations: []experiment.Variation{{Key: "a", Weight: 50}, {Key: "b", Weight: 50}},
	})
	if err != nil {
		t.Fatalf("seed experiment: %v", err)
	}
	id := exp.ID.String()
	ctx := tokenCtx(t, grantOn("experiments.manage", mine))

	for name, call := range map[string]func() error{
		"GetExperiment": func() error { _, e := c.GetExperiment(ctx, &signalsv1.GetExperimentRequest{Id: id}); return e },
		"GetExperimentResults": func() error {
			_, e := c.GetExperimentResults(ctx, &signalsv1.GetExperimentResultsRequest{Id: id})
			return e
		},
		"UpdateExperiment": func() error {
			_, e := c.UpdateExperiment(ctx, &signalsv1.UpdateExperimentRequest{Id: id, Experiment: &signalsv1.ExperimentInput{
				ChannelId: mine.String(), Key: "hero", Name: "Hijacked", Status: "running", Coverage: 100,
				Variations: []*signalsv1.ExperimentVariation{{Key: "a", Weight: 50}, {Key: "b", Weight: 50}},
			}})
			return e
		},
		"DeleteExperiment": func() error { _, e := c.DeleteExperiment(ctx, &signalsv1.DeleteExperimentRequest{Id: id}); return e },
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s on another channel's experiment answered %s, want PermissionDenied", name, got)
		}
	}
	row, err := qs.GetExperiment(context.Background(), exp.ID)
	if err != nil {
		t.Fatalf("the experiment is gone: %v", err)
	}
	if row.Name != "Hero" || row.Status != "draft" {
		t.Errorf("the experiment was written through a foreign grant: name %q, status %q", row.Name, row.Status)
	}
	if _, err := c.GetExperiment(tokenCtx(t, grantOn("experiments.view", theirs)), &signalsv1.GetExperimentRequest{Id: id}); err != nil {
		t.Errorf("a grant on the experiment's own channel was refused: %v", err)
	}
}

func TestARevisionIsGatedBeforeItIsRestored(t *testing.T) {
	_, deps := channelScopeServer(t)
	c := signalsv1.NewEmailBuilderServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	bg := context.Background()
	e, err := deps.Builder.CreateLibraryEmail(bg, emailtemplate.LibraryEmail{ChannelID: theirs, Name: "Welcome", Subject: "old"})
	if err != nil {
		t.Fatalf("seed email: %v", err)
	}
	if err := deps.Builder.Snapshot(bg, e, nil); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	revs, err := deps.Builder.ListRevisions(bg, e.ID, 1)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions: %v (%d)", err, len(revs))
	}
	e.Subject = "current"
	if _, err := deps.Builder.UpdateLibraryEmail(bg, e); err != nil {
		t.Fatalf("update email: %v", err)
	}

	_, err = c.RestoreEmailRevision(tokenCtx(t, grantOn("emails.manage", mine)), &signalsv1.RestoreEmailRevisionRequest{RevisionId: revs[0].ID.String()})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Errorf("restoring another channel's revision answered %s, want PermissionDenied", got)
	}
	got, _, err := deps.Builder.GetLibraryEmail(bg, e.ID)
	if err != nil {
		t.Fatalf("reload email: %v", err)
	}
	if got.Subject != "current" {
		t.Errorf("a refused restore still wrote the email: subject %q", got.Subject)
	}
}

func TestVisitorSessionsKeepToTheCallersChannels(t *testing.T) {
	qs, deps := channelScopeServer(t)
	c := signalsv1.NewLensServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	anon := "visitor-" + uuid.NewString()[:8]
	now := time.Now()
	ownSession, foreignSession := uuid.New(), uuid.New()
	for _, s := range []struct{ id, channel uuid.UUID }{{ownSession, mine}, {foreignSession, theirs}} {
		if _, err := qs.LensUpsertSession(context.Background(), signalsdb.LensUpsertSessionParams{
			ID: s.id, ChannelID: s.channel, AnonID: anon, StartedAt: now, LastEventAt: now, EventCount: 1,
		}); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}

	res, err := c.ListVisitorSessions(tokenCtx(t, grantOn("lens.view", mine)), &signalsv1.ListVisitorSessionsRequest{AnonId: anon})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.GetSessions()) != 1 || res.GetSessions()[0].GetId() != ownSession.String() {
		t.Errorf("sessions = %+v, want only the one on the caller's channel", res.GetSessions())
	}
	all, err := c.ListVisitorSessions(tokenCtx(t), &signalsv1.ListVisitorSessionsRequest{AnonId: anon})
	if err != nil {
		t.Fatalf("list as superuser: %v", err)
	}
	if len(all.GetSessions()) != 2 {
		t.Errorf("a superuser saw %d sessions, want both", len(all.GetSessions()))
	}
}

func TestAReplayFlushCannotJoinAForeignSession(t *testing.T) {
	qs, _ := channelScopeServer(t)
	bg := context.Background()
	id, channel, now := uuid.New(), uuid.New(), time.Now()
	seed := signalsdb.LensUpsertSessionParams{ID: id, ChannelID: channel, AnonID: "owner", StartedAt: now, LastEventAt: now, EventCount: 1}
	if n, err := qs.LensUpsertSession(bg, seed); err != nil || n != 1 {
		t.Fatalf("seed session: n=%d err=%v", n, err)
	}
	otherChannel, otherVisitor := seed, seed
	otherChannel.ChannelID = uuid.New()
	otherVisitor.AnonID = "intruder"
	for name, p := range map[string]signalsdb.LensUpsertSessionParams{"another channel": otherChannel, "another visitor": otherVisitor} {
		if n, err := qs.LensUpsertSession(bg, p); err != nil || n != 0 {
			t.Errorf("a flush from %s touched the session: n=%d err=%v", name, n, err)
		}
	}
	if n, err := qs.LensUpsertSession(bg, seed); err != nil || n != 1 {
		t.Errorf("the owner's own next flush was refused: n=%d err=%v", n, err)
	}
}

// An email's language versions belong to its channel: its own admin writes, reads, replaces and drops them, and a
// grant on another channel touches none of it
func TestAnEmailsTranslationsStayOnItsChannel(t *testing.T) {
	_, deps := channelScopeServer(t)
	c := signalsv1.NewEmailBuilderServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	e, err := deps.Builder.CreateLibraryEmail(context.Background(), emailtemplate.LibraryEmail{ChannelID: mine, Name: "Order confirmed", Subject: "Thanks"})
	if err != nil {
		t.Fatalf("seed email: %v", err)
	}
	own := tokenCtx(t, grantOn("emails.manage", mine), grantOn("emails.view", mine))
	other := tokenCtx(t, grantOn("emails.manage", theirs), grantOn("emails.view", theirs))
	save := func(ctx context.Context, subject string) error {
		_, err := c.SaveEmailTranslation(ctx, &signalsv1.SaveEmailTranslationRequest{Translation: &signalsv1.EmailTranslation{
			EmailId: e.ID.String(), Locale: "DE", Subject: subject, BlocksJson: `[{"type":"text","content":"Hallo"}]`,
		}})
		return err
	}

	if got := status.Code(save(other, "Fremd")); got != codes.PermissionDenied {
		t.Errorf("another channel's admin saving a translation answered %s, want PermissionDenied", got)
	}
	if err := save(own, "Danke"); err != nil {
		t.Fatalf("own admin saving: %v", err)
	}
	if err := save(own, "Vielen Dank"); err != nil {
		t.Fatalf("own admin replacing: %v", err)
	}
	if _, err := c.ListEmailTranslations(other, &signalsv1.ListEmailTranslationsRequest{EmailId: e.ID.String()}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("another channel listing translations answered %v, want PermissionDenied", status.Code(err))
	}
	res, err := c.ListEmailTranslations(own, &signalsv1.ListEmailTranslationsRequest{EmailId: e.ID.String()})
	if err != nil || len(res.GetTranslations()) != 1 || res.GetTranslations()[0].GetLocale() != "de" || res.GetTranslations()[0].GetSubject() != "Vielen Dank" {
		t.Fatalf("listed %v, %v - want one de version, replaced", res.GetTranslations(), err)
	}
	if got, _ := deps.Builder.Localize(context.Background(), e, "de-at"); got.Subject != "Vielen Dank" {
		t.Errorf("a de-at shopper gets %q, want the de version", got.Subject)
	}
	if _, err := c.DeleteEmailTranslation(other, &signalsv1.DeleteEmailTranslationRequest{EmailId: e.ID.String(), Locale: "de"}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("another channel deleting answered %v, want PermissionDenied", status.Code(err))
	}
	if _, err := c.DeleteEmailTranslation(own, &signalsv1.DeleteEmailTranslationRequest{EmailId: e.ID.String(), Locale: "de"}); err != nil {
		t.Fatalf("own admin deleting: %v", err)
	}
	if got, ok := deps.Builder.Localize(context.Background(), e, "de"); got.Subject != "Thanks" || ok {
		t.Errorf("after the delete a de shopper gets %q, want the email itself", got.Subject)
	}
}
