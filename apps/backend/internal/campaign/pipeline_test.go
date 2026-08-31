package campaign

import (
	"errors"
	"strings"
	"testing"
	"time"

	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
)

// A campaign is the one place the platform mails thousands of people on a schedule, so the gates matter more than the
// pipeline: what may be edited, what may be scheduled, and which subject line the held-back majority ends up receiving

type fakeCampaignRepo struct {
	Repository
	campaign   Campaign
	found      bool
	getErr     error
	updateOK   bool
	scheduleOK bool
	cancelOK   bool
	stats      []VariantStat
	statsErr   error

	created     []Campaign
	updated     []Campaign
	scheduledAt time.Time
	didSchedule bool
	didCancel   bool
}

func (f *fakeCampaignRepo) Create(_ context.Context, c Campaign) (Campaign, error) {
	f.created = append(f.created, c)
	c.ID = uuid.New()
	return c, nil
}

func (f *fakeCampaignRepo) Get(context.Context, uuid.UUID) (Campaign, bool, error) {
	return f.campaign, f.found, f.getErr
}

func (f *fakeCampaignRepo) Update(_ context.Context, c Campaign) (bool, error) {
	f.updated = append(f.updated, c)
	return f.updateOK, nil
}

func (f *fakeCampaignRepo) Schedule(_ context.Context, _ uuid.UUID, when time.Time) (bool, error) {
	f.didSchedule, f.scheduledAt = true, when
	return f.scheduleOK, nil
}

func (f *fakeCampaignRepo) Cancel(context.Context, uuid.UUID) (bool, error) {
	f.didCancel = true
	return f.cancelOK, nil
}

func (f *fakeCampaignRepo) VariantStats(context.Context, uuid.UUID) ([]VariantStat, error) {
	return f.stats, f.statsErr
}

func composer(repo Repository) *Service {
	return &Service{repo: repo, tmpl: emailtemplate.NewService(nil, nil, nil), unsubKey: []byte("campaign-test-key")}
}

func sendable() Input {
	return Input{ChannelID: uuid.New(), Name: "Spring sale", AudienceKind: AudienceAll, Subject: "Hello"}
}

func TestNormalizeABFillsAHalfSpecifiedTest(t *testing.T) {
	// The composer can save an A/B with only a subject B filled in - leaving the percent at zero would
	// send the test slice to nobody and the window at zero would resolve a winner off no data
	off := sendable()
	off.ABTestPercent, off.ABTestMinutes = 0, 0
	off.normalizeAB()
	if off.ABTestPercent != 0 || off.ABTestMinutes != 0 || off.ABWinnerMetric != "" {
		t.Errorf("a non-A/B input was rewritten: %+v", off)
	}

	on := sendable()
	on.ABEnabled, on.ABSubjectB = true, "Hello again"
	on.normalizeAB()
	if on.ABTestPercent != 20 || on.ABTestMinutes != 240 || on.ABWinnerMetric != ABMetricOpen {
		t.Errorf("defaults = %d%%/%dm/%q, want 20/240/open", on.ABTestPercent, on.ABTestMinutes, on.ABWinnerMetric)
	}

	explicit := sendable()
	explicit.ABEnabled, explicit.ABSubjectB = true, "Hello again"
	explicit.ABTestPercent, explicit.ABTestMinutes, explicit.ABWinnerMetric = 35, 60, ABMetricClick
	explicit.normalizeAB()
	if explicit.ABTestPercent != 35 || explicit.ABTestMinutes != 60 || explicit.ABWinnerMetric != ABMetricClick {
		t.Errorf("explicit values were overwritten: %+v", explicit)
	}
}

func TestValidateGuardsTheAudienceAndTheABSetup(t *testing.T) {
	id := uuid.New()
	withAudience := func(kind string, audienceID *uuid.UUID) Input {
		in := sendable()
		in.AudienceKind, in.AudienceID = kind, audienceID
		return in
	}
	withAB := func(mutate func(*Input)) Input {
		in := sendable()
		in.ABEnabled, in.ABSubjectB, in.ABWinnerMetric, in.ABTestPercent = true, "Subject B", ABMetricOpen, 20
		mutate(&in)
		return in
	}
	blankName := sendable()
	blankName.Name = "  "
	badBlocks := sendable()
	badBlocks.Blocks = []emailtemplate.Block{{Type: "video"}}

	for name, tc := range map[string]struct {
		in      Input
		wantErr bool
	}{
		"a nameless campaign":         {blankName, true},
		"an unknown audience":         {withAudience("everyone-ever", nil), true},
		"a group with no target":      {withAudience(AudienceGroup, nil), true},
		"a segment with no target":    {withAudience(AudienceSegment, nil), true},
		"a group with a target":       {withAudience(AudienceGroup, &id), false},
		"the whole channel":           {withAudience(AudienceAll, nil), false},
		"an unfinished block":         {badBlocks, true},
		"A/B with no subject B":       {withAB(func(in *Input) { in.ABSubjectB = " " }), true},
		"A/B on an unknown metric":    {withAB(func(in *Input) { in.ABWinnerMetric = "revenue" }), true},
		"A/B on clicks":               {withAB(func(in *Input) { in.ABWinnerMetric = ABMetricClick }), false},
		"A/B testing nobody":          {withAB(func(in *Input) { in.ABTestPercent = 0 }), true},
		"A/B testing almost everyone": {withAB(func(in *Input) { in.ABTestPercent = 91 }), true},
		"A/B at the low boundary":     {withAB(func(in *Input) { in.ABTestPercent = 1 }), false},
		"A/B at the high boundary":    {withAB(func(in *Input) { in.ABTestPercent = 90 }), false},
	} {
		t.Run(name, func(t *testing.T) {
			err := composer(&fakeCampaignRepo{}).validate(tc.in)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validate err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCreateStoresTheNormalizedABSetup(t *testing.T) { // validate would otherwise reject the percent Create was about to default
	repo := &fakeCampaignRepo{}
	in := sendable()
	in.ABEnabled, in.ABSubjectB = true, "Subject B"

	if _, err := composer(repo).Create(context.Background(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created %d campaigns, want 1", len(repo.created))
	}
	got := repo.created[0]
	if got.ABTestPercent != 20 || got.ABTestMinutes != 240 || got.ABWinnerMetric != ABMetricOpen {
		t.Errorf("stored A/B setup = %d%%/%dm/%q, want the filled defaults", got.ABTestPercent, got.ABTestMinutes, got.ABWinnerMetric)
	}
}

func TestOnlyDraftsCanBeEdited(t *testing.T) {
	// The draft check is the repository's UPDATE predicate - a sent campaign whose content changed
	// underneath would make its recipient log describe an email nobody received
	repo := &fakeCampaignRepo{updateOK: false}
	_, err := composer(repo).Update(context.Background(), uuid.New(), sendable())
	if err == nil || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("err = %v, want the draft-only refusal", err)
	}
	if len(repo.updated) != 1 {
		t.Error("Update never reached the repository, so the predicate was not the thing that refused")
	}
}

func TestScheduleRefusesCampaignsThatCannotSend(t *testing.T) {
	// Scheduling is the point of no return: the next scheduler tick materializes the audience and
	// starts mailing, so a contentless or already-sending campaign has to be stopped here
	emailID := uuid.New()
	draft := func(mutate func(*Campaign)) Campaign {
		c := Campaign{ID: uuid.New(), Status: StatusDraft, Blocks: []emailtemplate.Block{{Type: "text"}}}
		mutate(&c)
		return c
	}
	for name, tc := range map[string]struct {
		campaign   Campaign
		found      bool
		scheduleOK bool
		wantErr    string
	}{
		"a draft with inline content":  {draft(func(*Campaign) {}), true, true, ""},
		"a draft with a library email": {draft(func(c *Campaign) { c.Blocks, c.EmailID = nil, &emailID }), true, true, ""},
		"an already scheduled one":     {draft(func(c *Campaign) { c.Status = StatusScheduled }), true, true, ""},
		"a missing campaign":           {Campaign{}, false, true, "not found"},
		"one already sending":          {draft(func(c *Campaign) { c.Status = StatusSending }), true, true, "draft or scheduled"},
		"one already sent":             {draft(func(c *Campaign) { c.Status = StatusSent }), true, true, "draft or scheduled"},
		"a draft with no email":        {draft(func(c *Campaign) { c.Blocks = nil }), true, true, "no email"},
		"one the write refused":        {draft(func(*Campaign) {}), true, false, "could not be scheduled"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeCampaignRepo{campaign: tc.campaign, found: tc.found, scheduleOK: tc.scheduleOK}
			err := composer(repo).Schedule(context.Background(), tc.campaign.ID, nil)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Schedule: %v", err)
				}
				if !repo.didSchedule {
					t.Fatal("the write never happened")
				}
				if drift := time.Since(repo.scheduledAt); drift > time.Minute || drift < -time.Minute {
					t.Errorf("a nil time scheduled for %v, want about now", repo.scheduledAt)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.wantErr)
			}
		})
	}
}

func TestScheduleKeepsAnExplicitSendTime(t *testing.T) {
	repo := &fakeCampaignRepo{
		campaign:   Campaign{Status: StatusDraft, Blocks: []emailtemplate.Block{{Type: "text"}}},
		found:      true,
		scheduleOK: true,
	}
	when := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	if err := composer(repo).Schedule(context.Background(), uuid.New(), &when); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if !repo.scheduledAt.Equal(when) {
		t.Errorf("scheduled for %v, want %v", repo.scheduledAt, when)
	}
}

func TestCancelOnlyReportsSuccessWhenARowChanged(t *testing.T) {
	// A cancel that silently no-ops on a sent campaign would tell the operator the send was stopped
	for name, tc := range map[string]struct {
		cancelOK bool
		wantErr  bool
	}{
		"a scheduled campaign": {true, false},
		"nothing cancellable":  {false, true},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeCampaignRepo{cancelOK: tc.cancelOK}
			err := composer(repo).Cancel(context.Background(), uuid.New())
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !repo.didCancel {
				t.Error("Cancel never reached the repository")
			}
		})
	}
}

func TestWinnerIsTheBetterRateNotTheBiggerNumber(t *testing.T) {
	// The winner's subject goes to every held-back recipient, so comparing raw counts would hand the
	// majority to whichever variant happened to get the larger test slice
	for name, tc := range map[string]struct {
		metric string
		stats  []VariantStat
		err    error
		want   string
	}{
		"B converts better": {ABMetricOpen, []VariantStat{
			{Variant: VariantA, Sent: 100, Opened: 10}, {Variant: VariantB, Sent: 100, Opened: 25},
		}, nil, VariantB},
		"A converts better": {ABMetricOpen, []VariantStat{
			{Variant: VariantA, Sent: 100, Opened: 30}, {Variant: VariantB, Sent: 100, Opened: 25},
		}, nil, VariantA},
		"B has more opens but a worse rate": {ABMetricOpen, []VariantStat{
			{Variant: VariantA, Sent: 10, Opened: 5}, {Variant: VariantB, Sent: 100, Opened: 20},
		}, nil, VariantA},
		"a dead heat goes to A": {ABMetricOpen, []VariantStat{
			{Variant: VariantA, Sent: 100, Opened: 20}, {Variant: VariantB, Sent: 50, Opened: 10},
		}, nil, VariantA},
		"clicks decide when that is the metric": {ABMetricClick, []VariantStat{
			{Variant: VariantA, Sent: 100, Opened: 90, Clicked: 1}, {Variant: VariantB, Sent: 100, Opened: 10, Clicked: 9},
		}, nil, VariantB},
		"a variant that reached nobody": {ABMetricOpen, []VariantStat{
			{Variant: VariantA, Sent: 0, Opened: 0}, {Variant: VariantB, Sent: 0, Opened: 0},
		}, nil, VariantA},
		"no measurements at all": {ABMetricOpen, nil, nil, VariantA},
		"a stats read that failed": {ABMetricOpen, []VariantStat{
			{Variant: VariantB, Sent: 100, Opened: 99},
		}, errors.New("gone"), VariantA},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeCampaignRepo{stats: tc.stats, statsErr: tc.err}
			got := composer(repo).pickWinner(context.Background(), Campaign{ABWinnerMetric: tc.metric})
			if got != tc.want {
				t.Errorf("winner = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTruncateBoundsTheStoredBounceReason(t *testing.T) { // SMTP rejections can carry a whole message body back
	for name, tc := range map[string]struct {
		in   string
		n    int
		want string
	}{
		"shorter":      {"550 no", 240, "550 no"},
		"exact length": {"abcde", 5, "abcde"},
		"longer":       {"abcdefghij", 4, "abcd"},
		"zero":         {"abc", 0, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := truncate(tc.in, tc.n); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestStorefrontURLPrefersTheChannelThenTheEnvironment(t *testing.T) {
	// The unsubscribe and tracking links are built on this - getting an empty or double-slashed base
	// produces links that 404, which is a compliance problem rather than a cosmetic one
	t.Setenv("STOREFRONT_URL", "https://fallback.example/")
	for name, tc := range map[string]struct {
		publicURL string
		want      string
	}{
		"the channel's own url":    {"https://shop.example", "https://shop.example"},
		"a trailing slash trimmed": {"https://shop.example/", "https://shop.example"},
		"blank falls back to env":  {"  ", "https://fallback.example"},
		"unset falls back to env":  {"", "https://fallback.example"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := storefrontURL(channel.Channel{PublicURL: tc.publicURL}); got != tc.want {
				t.Errorf("storefrontURL = %q, want %q", got, tc.want)
			}
		})
	}
	t.Setenv("STOREFRONT_URL", "")
	if got := storefrontURL(channel.Channel{}); got != "" {
		t.Errorf("with nothing configured storefrontURL = %q, want empty", got)
	}
}

func TestFromAddressFallsBackToTheStoreName(t *testing.T) {
	// With no config service wired the send still needs a legal From - an empty one is rejected by
	// every relay, so the campaign would fail per recipient rather than visibly
	s := composer(&fakeCampaignRepo{})
	if got, want := s.fromAddress(context.Background(), uuid.New(), "Skelet Shop"), "Skelet Shop <no-reply@skelet.local>"; got != want {
		t.Errorf("fromAddress = %q, want %q", got, want)
	}
	if got, want := s.fromAddress(context.Background(), uuid.New(), ""), "no-reply@skelet.local"; got != want {
		t.Errorf("nameless fromAddress = %q, want %q", got, want)
	}
}

func TestRecipientVarsCarryAWorkingUnsubscribeLink(t *testing.T) {
	// Every marketing send is legally required to carry one, and it has to verify back to THIS
	// customer - a link that fails verification is an unsubscribe button that silently does nothing
	s := composer(&fakeCampaignRepo{})
	ch := channel.Channel{ID: uuid.New(), Name: "Skelet Shop", PublicURL: "https://shop.example/"}
	customerID := uuid.New()

	vars := s.recipientVars(context.Background(), ch, Recipient{
		CustomerID: &customerID, Email: "shopper@example.invalid", FirstName: "",
	})
	if vars["customer.firstName"] != "there" {
		t.Errorf("firstName = %q, want the generic greeting", vars["customer.firstName"])
	}
	if vars["customer.email"] != "shopper@example.invalid" {
		t.Errorf("email = %q", vars["customer.email"])
	}
	if vars["storefront.url"] != "https://shop.example" {
		t.Errorf("storefront.url = %q, want the trimmed base", vars["storefront.url"])
	}
	link := vars["unsubscribe.link"]
	prefix := "https://shop.example/unsubscribe/"
	if !strings.HasPrefix(link, prefix) {
		t.Fatalf("unsubscribe.link = %q, want it under %q", link, prefix)
	}
	got, err := s.verifyToken(strings.TrimPrefix(link, prefix))
	if err != nil {
		t.Fatalf("the emailed token does not verify: %v", err)
	}
	if got != customerID {
		t.Errorf("token resolves to %v, want %v", got, customerID)
	}
}

func TestThePreviewLinkCannotUnsubscribeAnyone(t *testing.T) {
	// Sample vars go into the composer preview and test sends, so a real signed token there would let
	// anyone opening a preview opt somebody out
	s := composer(&fakeCampaignRepo{})
	vars := s.sampleVars(context.Background(), channel.Channel{PublicURL: "https://shop.example"})
	if want := "https://shop.example/unsubscribe/sample"; vars["unsubscribe.link"] != want {
		t.Errorf("sample link = %q, want %q", vars["unsubscribe.link"], want)
	}
	if _, err := s.verifyToken("sample"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("the sample token verified: err = %v, want ErrInvalidToken", err)
	}
}

func TestMergeTagPaletteMatchesWhatTheVarsSupply(t *testing.T) {
	// The composer offers these tags for insertion - one the send never fills renders as a literal
	// {{tag}} in the customer's inbox
	s := composer(&fakeCampaignRepo{})
	customerID := uuid.New()
	vars := s.recipientVars(context.Background(), channel.Channel{Name: "Shop", PublicURL: "https://shop.example"},
		Recipient{CustomerID: &customerID, Email: "a@b.invalid", FirstName: "Alex"})

	for _, tag := range CampaignMergeTags() {
		if _, ok := vars[tag]; !ok {
			t.Errorf("the composer offers %q but the send supplies no value for it", tag)
		}
	}
}
