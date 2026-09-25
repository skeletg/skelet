package campaign

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/engagement"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
)

const (
	perRunSendCap = 500 // recipients one send pass handles per campaign - the rest resume next pass
	claimLimit    = 10  // due campaigns leased per claim
)

// Orchestrates campaign CRUD, audience resolution and the scheduled send pipeline - the scheduler job lease serializes ProcessDue, so recipient processing needs no extra cross-replica locking
type Service struct {
	repo     Repository
	tmpl     *emailtemplate.Service
	contacts audienceReader
	segments segmentAudience
	stores   storeReader
	config   *config.Service // the suite's settings - the deliverability switches the bounce loop reads
	sender   email.Sender
	recorder sendTracker
	track    *emailtrack.Signer
	suppress suppressionChecker
}

type suppressionChecker interface { // the slice of the suppression service the send loop needs
	IsSuppressed(ctx context.Context, email string) bool
	SuppressedSet(ctx context.Context, emails []string) map[string]bool // one query for a whole recipient page, keyed by the caller's original email strings
	Suppress(ctx context.Context, email, reason, detail string) error
	RecordSoftBounce(ctx context.Context, email, detail string, threshold int) (bool, int, error)
}

// The record of what this service sends: the send log the tracking token names, the spine event
// beside it, and the per-variant tallies the winner decision reads. Satisfied by *engagement.Tracker
type sendTracker interface {
	Sent(ctx context.Context, spec sendlog.Spec) error
	Open(ctx context.Context, sendID uuid.UUID, hit analytics.Hit) (engagement.Resolved, error)
	Click(ctx context.Context, sendID uuid.UUID, url string, linkIndex int, hit analytics.Hit) (engagement.Resolved, error)
	VariantStats(ctx context.Context, campaignID uuid.UUID) ([]sendlog.VariantStat, error)
}

// The audience a scheduled send mails - declared here so campaigns name the question, not the suite
type segmentAudience interface {
	SegmentMemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error)
}

// unsubKey signs unsubscribe and tracking links, the same key core verifies them with
func NewService(repo Repository, tmpl *emailtemplate.Service, contacts audienceReader, segments segmentAudience, stores storeReader, cfg *config.Service, sender email.Sender, recorder sendTracker, suppress suppressionChecker, unsubKey []byte) *Service {
	return &Service{repo: repo, tmpl: tmpl, contacts: contacts, segments: segments, stores: stores, config: cfg, sender: sender, recorder: recorder, suppress: suppress, track: emailtrack.NewSigner(unsubKey)}
}

// Turns a send failure into a suppression signal when auto-suppress is on - a permanent 5xx SMTP rejection hard-suppresses the address, a transient one accrues toward the soft-bounce threshold, and unknown errors like timeouts and resets are ignored since they aren't a verdict on the address (this is the out-of-the-box feedback loop for pure-SMTP setups, a provider webhook supplies richer signals)
func (s *Service) captureBounce(ctx context.Context, addr string, sendErr error) {
	if s.suppress == nil || s.config == nil {
		return
	}
	if on, _ := s.config.GetBool(ctx, "signals/deliverability/auto_suppress", config.DefaultScope); !on {
		return
	}
	permanent, matched := email.ClassifyBounce(sendErr)
	if !matched {
		return
	}
	if permanent {
		if err := s.suppress.Suppress(ctx, addr, "hard_bounce", truncate(sendErr.Error(), 240)); err != nil {
			log.Printf("[campaign] suppress hard bounce for %s: %v", addr, err) // dropping it silently keeps mailing a dead address, which costs sender reputation
		}
		return
	}
	threshold := 3
	if n, err := s.config.GetInt(ctx, "signals/deliverability/soft_bounce_threshold", config.DefaultScope); err == nil && n > 0 {
		threshold = int(n)
	}
	if _, _, err := s.suppress.RecordSoftBounce(ctx, addr, truncate(sendErr.Error(), 240), threshold); err != nil {
		log.Printf("[campaign] record soft bounce for %s: %v", addr, err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type Input struct {
	ChannelID      uuid.UUID
	Name           string
	EmailID        *uuid.UUID
	Subject        string
	Blocks         []emailtemplate.Block
	Styles         emailtemplate.EmailStyles
	AudienceKind   string
	AudienceID     *uuid.UUID
	ABEnabled      bool
	ABSubjectB     string
	ABTestPercent  int
	ABWinnerMetric string
	ABTestMinutes  int
}

func (in *Input) normalizeAB() { // fills A/B defaults so a half-specified test still runs sensibly
	if !in.ABEnabled {
		return
	}
	if in.ABTestPercent <= 0 {
		in.ABTestPercent = 20
	}
	if in.ABTestMinutes <= 0 {
		in.ABTestMinutes = 240
	}
	if in.ABWinnerMetric == "" {
		in.ABWinnerMetric = ABMetricOpen
	}
}

func (s *Service) validate(in Input) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("campaign name is required")
	}
	if !validAudience(in.AudienceKind) {
		return fmt.Errorf("invalid audience %q", in.AudienceKind)
	}
	if (in.AudienceKind == AudienceGroup || in.AudienceKind == AudienceSegment) && in.AudienceID == nil {
		return fmt.Errorf("a %s audience requires a target id", in.AudienceKind)
	}
	if in.ABEnabled { // subject-line A/B only - variant A keeps the email's own subject, library or inline, while B overrides it with ab_subject_b, and the content is the same either way
		if strings.TrimSpace(in.ABSubjectB) == "" {
			return fmt.Errorf("A/B test needs a subject B")
		}
		if in.ABWinnerMetric != ABMetricOpen && in.ABWinnerMetric != ABMetricClick {
			return fmt.Errorf("A/B winner metric must be 'open' or 'click'")
		}
		if in.ABTestPercent < 1 || in.ABTestPercent > 90 {
			return fmt.Errorf("A/B test percent must be between 1 and 90")
		}
	}
	return s.tmpl.ValidateContent(in.Blocks, in.Styles)
}

// A campaign sends only a library email of its own channel - another channel's would carry that store's content and branding
func (s *Service) libraryEmail(ctx context.Context, channelID uuid.UUID, emailID *uuid.UUID) (*emailtemplate.LibraryEmail, error) {
	if emailID == nil {
		return nil, nil
	}
	le, ok, err := s.tmpl.GetLibraryEmail(ctx, *emailID)
	if err != nil {
		return nil, err
	}
	if !ok || le.ChannelID != channelID {
		return nil, fmt.Errorf("library email %s not found", *emailID)
	}
	return &le, nil
}

func (s *Service) Create(ctx context.Context, in Input) (Campaign, error) {
	in.normalizeAB()
	if err := s.validate(in); err != nil {
		return Campaign{}, err
	}
	if _, err := s.libraryEmail(ctx, in.ChannelID, in.EmailID); err != nil {
		return Campaign{}, err
	}
	return s.repo.Create(ctx, Campaign{
		ChannelID: in.ChannelID, Name: in.Name, EmailID: in.EmailID, Subject: in.Subject,
		Blocks: in.Blocks, Styles: in.Styles, AudienceKind: in.AudienceKind, AudienceID: in.AudienceID,
		ABEnabled: in.ABEnabled, ABSubjectB: in.ABSubjectB, ABTestPercent: in.ABTestPercent,
		ABWinnerMetric: in.ABWinnerMetric, ABTestMinutes: in.ABTestMinutes,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (Campaign, error) {
	in.normalizeAB()
	if err := s.validate(in); err != nil {
		return Campaign{}, err
	}
	if _, err := s.libraryEmail(ctx, in.ChannelID, in.EmailID); err != nil {
		return Campaign{}, err
	}
	ok, err := s.repo.Update(ctx, Campaign{
		ID: id, Name: in.Name, EmailID: in.EmailID, Subject: in.Subject, Blocks: in.Blocks, Styles: in.Styles,
		AudienceKind: in.AudienceKind, AudienceID: in.AudienceID,
		ABEnabled: in.ABEnabled, ABSubjectB: in.ABSubjectB, ABTestPercent: in.ABTestPercent,
		ABWinnerMetric: in.ABWinnerMetric, ABTestMinutes: in.ABTestMinutes,
	})
	if err != nil {
		return Campaign{}, err
	}
	if !ok {
		return Campaign{}, fmt.Errorf("only draft campaigns can be edited")
	}
	c, _, err := s.repo.Get(ctx, id)
	return c, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Campaign, bool, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Campaign, error) {
	return s.repo.List(ctx, channelID, includeDeleted)
}

func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error { // a sent campaign keeps its recipient rows and engagement stats, and the scheduler skips deleted rows so a deleted scheduled campaign never goes out
	return s.repo.SoftDelete(ctx, id)
}

func (s *Service) Restore(ctx context.Context, id uuid.UUID) error {
	return s.repo.Restore(ctx, id)
}

func (s *Service) Schedule(ctx context.Context, id uuid.UUID, when *time.Time) error { // a nil `when` means send now, which the scheduler picks up on its next tick, within about a minute
	c, ok, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("campaign not found")
	}
	if c.Status != StatusDraft && c.Status != StatusScheduled {
		return fmt.Errorf("only draft or scheduled campaigns can be (re)scheduled")
	}
	if c.EmailID == nil && len(c.Blocks) == 0 {
		return fmt.Errorf("campaign has no email - pick a library email or add content")
	}
	at := time.Now()
	if when != nil {
		at = *when
	}
	done, err := s.repo.Schedule(ctx, id, at)
	if err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("campaign could not be scheduled")
	}
	return nil
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID) error {
	ok, err := s.repo.Cancel(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("only scheduled or sending campaigns can be cancelled")
	}
	return nil
}

// --- audience resolution ---

type candidate struct {
	customerID uuid.UUID
	email      string
	firstName  string
}

func (s *Service) resolveAudience(ctx context.Context, channelID uuid.UUID, kind string, audienceID *uuid.UUID) ([]candidate, error) {
	all, err := s.contacts.ListForChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var segSet map[uuid.UUID]bool
	if kind == AudienceSegment && audienceID != nil {
		ids, err := s.segments.SegmentMemberIDs(ctx, *audienceID)
		if err != nil {
			return nil, err
		}
		segSet = make(map[uuid.UUID]bool, len(ids))
		for _, id := range ids {
			segSet[id] = true
		}
	}
	out := make([]candidate, 0, len(all))
	for _, cust := range all {
		if !cust.Active || strings.TrimSpace(cust.Email) == "" {
			continue
		}
		switch kind {
		case AudienceGroup:
			if audienceID == nil || cust.GroupID != *audienceID {
				continue
			}
		case AudienceSegment:
			if segSet == nil || !segSet[cust.ID] {
				continue
			}
		}
		out = append(out, candidate{customerID: cust.ID, email: cust.Email, firstName: cust.FirstName})
	}
	return out, nil
}

func (s *Service) EstimateAudience(ctx context.Context, channelID uuid.UUID, kind string, audienceID *uuid.UUID) (total, eligible int, err error) { // eligible = after the marketing-consent gate, surfaced in the composer before sending
	if !validAudience(kind) {
		return 0, 0, fmt.Errorf("invalid audience %q", kind)
	}
	cands, err := s.resolveAudience(ctx, channelID, kind, audienceID)
	if err != nil {
		return 0, 0, err
	}
	ids := make([]uuid.UUID, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.customerID)
	}
	optedIn, err := s.contacts.OptedInByIDs(ctx, ids)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range cands {
		if optedIn[c.customerID] {
			eligible++
		}
	}
	return len(cands), eligible, nil
}

// --- send pipeline (scheduler-driven) ---

// ProcessDue is one pass of the pipeline, for callers that tick it by hand
func (s *Service) ProcessDue(ctx context.Context) error {
	_, err := s.Drain(ctx, 0)
	return err
}

// Drain leases and materializes what is due, then runs send passes until nothing is pending or the budget is spent
// Answers how many recipient rows the passes handled - a pass that errored ends the drain, the next tick retries
func (s *Service) Drain(ctx context.Context, budget time.Duration) (int, error) {
	deadline := time.Now().Add(budget)
	more := func() bool { return budget > 0 && time.Now().Before(deadline) && ctx.Err() == nil }
	s.resolveABWinners(ctx) // first, so a test whose window elapsed flips back to 'sending' and the passes below pick up the winner's holdout send
	// One bad campaign never stops the others, but the failures ride back to the scheduler instead of only reaching stdout - returning nil here marked the run `success`, which reset the retry ladder, cleared the alert and left the jobs panel green while no campaign email was going out
	var errs []error
	for {
		claimed, err := s.repo.ClaimDue(ctx, claimLimit)
		if err != nil {
			return 0, err
		}
		for _, c := range claimed {
			if err := s.materialize(ctx, c); err != nil {
				log.Printf("[campaign] materialize %s: %v", c.ID, err)
				errs = append(errs, fmt.Errorf("materialize %s: %w", c.ID, err))
			}
		}
		if len(claimed) < claimLimit || !more() {
			break // a claim that came back short has nothing behind it
		}
	}
	total := 0
	for {
		sending, err := s.repo.ListSending(ctx)
		if err != nil {
			return total, errors.Join(append(errs, err)...)
		}
		handled := 0
		for _, c := range sending {
			n, err := s.send(ctx, c)
			handled += n
			if err != nil {
				log.Printf("[campaign] send %s: %v", c.ID, err)
				errs = append(errs, fmt.Errorf("send %s: %w", c.ID, err))
			}
			if budget > 0 && !more() {
				return total + handled, errors.Join(errs...) // the rest lead the next pass, having advanced least recently
			}
		}
		total += handled
		if handled == 0 || len(errs) > 0 || !more() {
			return total, errors.Join(errs...)
		}
	}
}

func (s *Service) materialize(ctx context.Context, c Campaign) error { // snapshots the audience into pending recipient rows - idempotent, skipping when already materialized, so a resumed campaign isn't re-expanded
	n, err := s.repo.CountRecipients(ctx, c.ID)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	cands, err := s.resolveAudience(ctx, c.ChannelID, c.AudienceKind, c.AudienceID)
	if err != nil {
		return err
	}
	if c.ABEnabled && len(cands) >= 2 { // carve a test slice for the A/B split and hold the rest back for whichever subject wins
		rand.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] }) // resolveAudience returns creation-time order, so shuffling keeps the test slice from skewing to the oldest customers
		testN := min(
			max(len(cands)*c.ABTestPercent/100,

				2), len(cands)) // floor of 2 - a meaningful test needs at least one recipient per variant
		for i, cand := range cands {
			id := cand.customerID
			variant, phase := "", PhaseHoldout
			if i < testN {
				phase = PhaseTest
				variant = VariantA
				if i%2 == 1 {
					variant = VariantB
				}
			}
			if err := s.repo.InsertRecipient(ctx, c.ID, &id, cand.email, cand.firstName, variant, phase); err != nil {
				return err
			}
		}
		return s.repo.SetTotal(ctx, c.ID, int32(len(cands)))
	}
	for _, cand := range cands {
		id := cand.customerID
		if err := s.repo.InsertRecipient(ctx, c.ID, &id, cand.email, cand.firstName, "", PhaseSend); err != nil {
			return err
		}
	}
	return s.repo.SetTotal(ctx, c.ID, int32(len(cands)))
}

// send is one pass over a campaign's pending recipients, capped at perRunSendCap - answers how many rows it handled, skips and failures included
func (s *Service) send(ctx context.Context, c Campaign) (int, error) {
	ch, err := s.stores.Get(ctx, c.ChannelID)
	if err != nil {
		return 0, err
	}
	if !ch.Active {
		return 0, s.repo.RefreshCounts(ctx, c.ID) // held, and moved behind the campaigns that can send
	}
	from := s.fromAddress(ch)
	libEmail, err := s.libraryEmail(ctx, c.ChannelID, c.EmailID) // resolved ONCE for the whole send - every recipient renders the same template and only the merge vars differ, so a per-recipient round-trip buys nothing
	if err != nil {
		return 0, err
	}
	sent := 0
	for sent < perRunSendCap {
		batch, err := s.repo.ListPending(ctx, c.ID, 100)
		if err != nil {
			return sent, err
		}
		if len(batch) == 0 {
			break
		}
		// Batch the consent and suppression gates for the whole page so the per-recipient checks are map lookups, not a query each - consent still honors the definition default, an absent row being the GDPR-safe opt-out for marketing, so this never sends to anyone IsOptedIn would have skipped
		custIDs := make([]uuid.UUID, 0, len(batch))
		emails := make([]string, 0, len(batch))
		for _, r := range batch {
			if r.CustomerID != nil {
				custIDs = append(custIDs, *r.CustomerID)
			}
			emails = append(emails, r.Email)
		}
		optedIn, oerr := s.contacts.OptedInByIDs(ctx, custIDs)
		if oerr != nil {
			return sent, oerr // consent uncertainty must NOT send - abort and retry the run
		}
		var suppressed map[string]bool
		if s.suppress != nil {
			suppressed = s.suppress.SuppressedSet(ctx, emails)
		}
		for _, r := range batch {
			if sent >= perRunSendCap {
				break
			}
			sent++
			if r.CustomerID != nil && !optedIn[*r.CustomerID] { // re-checked at send time so an unsubscribe since scheduling is honored
				if err := s.repo.MarkSkipped(ctx, r.ID, "not opted in to marketing email"); err != nil {
					log.Printf("[campaign] mark skipped (consent) %s: %v", r.ID, err) // a dropped skip leaves the row pending, so the next tick re-evaluates it forever and the campaign never drains
				}
				continue
			}
			if suppressed[r.Email] { // deliverability guard, after consent and before render - a nil map, meaning no suppression service, reads as not-suppressed
				if err := s.repo.MarkSkipped(ctx, r.ID, "address on suppression list"); err != nil {
					log.Printf("[campaign] mark skipped (suppressed) %s: %v", r.ID, err)
				}
				continue
			}
			subj, html, text, err := s.renderVariant(ctx, c, libEmail, r.Variant, s.recipientVars(ch, r))
			if err != nil {
				if merr := s.repo.MarkFailed(ctx, r.ID, err.Error()); merr != nil {
					log.Printf("[campaign] mark failed %s: %v", r.ID, merr)
				}
				continue
			}
			rid := r.ID
			sendID := engagement.NewSendID()
			// Recorded before the mail goes out, so the tracking token names a send the log holds - a log write failing is infra, not this recipient's fault: abort like the consent gate does, recipients stay pending and the next tick retries
			if err := s.recorder.Sent(ctx, sendlog.Spec{
				ID: sendID, ChannelID: c.ChannelID, Source: sendlog.SourceCampaign, Email: r.Email,
				CustomerID: r.CustomerID, CampaignID: &c.ID, RecipientID: &rid, Variant: r.Variant,
			}); err != nil {
				return sent, fmt.Errorf("track send: %w", err)
			}
			html = s.injectTracking(html, ch, sendID)
			// Claim the recipient BEFORE handing the message to the sender - this row is the only thing that stops a resend, so marking after delivery leaves a window where a dropped write re-mails the same person every tick, forever (the run re-reads pending, and the campaign never drains so it never finalizes)
			claimed, cerr := s.repo.ClaimForSend(ctx, r.ID)
			if cerr != nil {
				return sent, fmt.Errorf("claim recipient: %w", cerr) // nothing sent yet - abort like the consent gate, recipients stay pending and the next tick retries
			}
			if !claimed {
				continue // a concurrent sweep already owns this one
			}
			if err := s.sender.Send(ctx, email.Message{From: from, To: r.Email, Subject: subj, HTMLBody: html, TextBody: text}); err != nil {
				if rerr := s.repo.ReleaseClaimFailed(ctx, r.ID, err.Error()); rerr != nil { // release the claim so the failure is recorded rather than reading as delivered
					log.Printf("[campaign] release claim %s: %v", r.ID, rerr) // the row stays claimed, so this recipient silently never sends - loud enough for an operator to requeue
				}
				s.captureBounce(ctx, r.Email, err)
				continue
			}
		}
	}
	if err := s.repo.RefreshCounts(ctx, c.ID); err != nil {
		return sent, err
	}
	if c.ABEnabled && c.ABWinner == "" { // A/B test phase - once the non-holdout slice is fully sent, enter the measurement window and let the winner job flip it back to 'sending' with the holdout assigned to the winner
		nonHoldout, err := s.repo.CountPendingNonHoldout(ctx, c.ID)
		if err != nil {
			return sent, err
		}
		if nonHoldout == 0 {
			_, err = s.repo.MarkTesting(ctx, c.ID)
			return sent, err
		}
		return sent, nil
	}
	remaining, err := s.repo.CountPending(ctx, c.ID)
	if err != nil {
		return sent, err
	}
	if remaining == 0 {
		return sent, s.repo.Finalize(ctx, c.ID)
	}
	return sent, nil
}

// Picks the winning subject for A/B campaigns whose measurement window elapsed, sends it to the held-back remainder, and resumes the send
func (s *Service) resolveABWinners(ctx context.Context) {
	due, err := s.repo.ListABTestingDue(ctx)
	if err != nil {
		log.Printf("[campaign] list a/b testing due: %v", err)
		return
	}
	for _, c := range due {
		winner := s.pickWinner(ctx, c)
		if err := s.repo.AssignHoldoutToWinner(ctx, c.ID, winner); err != nil {
			log.Printf("[campaign] a/b assign holdout %s: %v", c.ID, err)
			continue
		}
		if _, err := s.repo.SetWinner(ctx, c.ID, winner); err != nil {
			log.Printf("[campaign] a/b set winner %s: %v", c.ID, err)
		}
	}
}

func (s *Service) pickWinner(ctx context.Context, c Campaign) string { // higher metric rate among the test slice wins - ties and missing data go to A
	stats, err := s.VariantStats(ctx, c.ID)
	if err != nil {
		return VariantA
	}
	rate := func(v string) float64 {
		for _, st := range stats {
			if st.Variant == v && st.Sent > 0 {
				num := st.Opened
				if c.ABWinnerMetric == ABMetricClick {
					num = st.Clicked
				}
				return float64(num) / float64(st.Sent)
			}
		}
		return 0
	}
	if rate(VariantB) > rate(VariantA) {
		return VariantB
	}
	return VariantA
}

// Renders a recipient's email, library or inline, overriding the subject with subject B for the B variant - content is identical across variants, since this is a subject-line test
func (s *Service) renderVariant(ctx context.Context, c Campaign, libEmail *emailtemplate.LibraryEmail, variant string, vars map[string]string) (subject, html, text string, err error) {
	subject, html, text, err = s.renderPrepared(ctx, c, libEmail, vars)
	if err == nil && c.ABEnabled && variant == VariantB && strings.TrimSpace(c.ABSubjectB) != "" {
		subject = c.ABSubjectB
	}
	return
}

// Mirrors render() but takes the library email fetched ONCE per send, so recipients don't each trigger a GetLibraryEmail - falls back to the campaign's inline content when it isn't library-backed
func (s *Service) renderPrepared(ctx context.Context, c Campaign, libEmail *emailtemplate.LibraryEmail, vars map[string]string) (subject, html, text string, err error) {
	if libEmail != nil {
		return s.tmpl.RenderContent(ctx, libEmail.ChannelID, libEmail.Subject, libEmail.Preheader, libEmail.Blocks, libEmail.Styles, vars)
	}
	return s.tmpl.RenderContent(ctx, c.ChannelID, c.Subject, "", c.Blocks, c.Styles, vars)
}

// VariantStats is the per-arm tally off the send log, human hits only
func (s *Service) VariantStats(ctx context.Context, id uuid.UUID) ([]VariantStat, error) {
	if s.recorder == nil {
		return nil, nil
	}
	rows, err := s.recorder.VariantStats(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]VariantStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, VariantStat{Variant: r.Variant, Sent: r.Sent, Opened: r.Opened, Clicked: r.Clicked})
	}
	return out, nil
}

// Produces a campaign's email - from the referenced library email when EmailID is set, else from the campaign's own inline content
func (s *Service) render(ctx context.Context, c Campaign, vars map[string]string) (subject, html, text string, err error) {
	if c.EmailID != nil {
		return s.tmpl.RenderLibraryEmail(ctx, *c.EmailID, vars)
	}
	return s.tmpl.RenderContent(ctx, c.ChannelID, c.Subject, "", c.Blocks, c.Styles, vars)
}

// --- preview + test send ---

func (s *Service) Preview(ctx context.Context, channelID uuid.UUID, subject string, blocks []emailtemplate.Block, styles emailtemplate.EmailStyles) (string, string, error) { // composer preview - unsaved content rendered with sample vars
	ch, err := s.stores.Get(ctx, channelID)
	if err != nil {
		return "", "", err
	}
	subj, html, _, err := s.tmpl.RenderContent(ctx, channelID, subject, "", blocks, styles, sampleVars(ch))
	return subj, html, err
}

func (s *Service) SendTest(ctx context.Context, id uuid.UUID, to string) error { // the stored campaign rendered with sample vars, mailed to one address
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("recipient is required")
	}
	c, ok, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("campaign not found")
	}
	if _, err := s.libraryEmail(ctx, c.ChannelID, c.EmailID); err != nil {
		return err
	}
	ch, err := s.stores.Get(ctx, c.ChannelID)
	if err != nil {
		return err
	}
	subj, html, text, err := s.render(ctx, c, sampleVars(ch))
	if err != nil {
		return err
	}
	return s.sender.Send(ctx, email.Message{From: s.fromAddress(ch), To: to, Subject: "[TEST] " + subj, HTMLBody: html, TextBody: text})
}

// --- merge-tag vars ---

func (s *Service) recipientVars(ch Store, r Recipient) map[string]string {
	v := baseVars(ch)
	v["customer.firstName"] = firstNameOr(r.FirstName)
	v["customer.email"] = r.Email
	if r.CustomerID != nil {
		v["unsubscribe.link"] = s.unsubscribeLink(ch, *r.CustomerID)
	}
	return v
}

func sampleVars(ch Store) map[string]string {
	v := baseVars(ch)
	v["customer.firstName"] = "Alex"
	v["customer.email"] = "alex@example.com"
	v["unsubscribe.link"] = storefrontURL(ch) + "/unsubscribe/sample"
	return v
}

// The store as core pushed it onto the roster - the customer-facing name and the branding logo
func baseVars(ch Store) map[string]string {
	return map[string]string{
		"store.name":     ch.Name,
		"store.logoUrl":  strings.TrimSpace(ch.LogoURL),
		"storefront.url": storefrontURL(ch),
	}
}

// Signed over the customer id with the key core verifies with - the link works without a session, and
// following it records the opt-out where consent lives, which is core
func (s *Service) unsubscribeLink(ch Store, customerID uuid.UUID) string {
	return storefrontURL(ch) + "/unsubscribe/" + s.track.SignUnsubscribe(customerID)
}

// The channel's sender identity as core pushed it, with the store's name when no sender name was
// ever set - an empty From is refused by every relay
func (s *Service) fromAddress(ch Store) string {
	addr := strings.TrimSpace(ch.FromAddress)
	if addr == "" {
		addr = "no-reply@skelet.local"
	}
	name := strings.TrimSpace(ch.FromName)
	if name == "" {
		name = ch.Name
	}
	if name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}

func storefrontURL(ch Store) string {
	return strings.TrimRight(strings.TrimSpace(ch.PublicURL), "/")
}

func firstNameOr(name string) string {
	if strings.TrimSpace(name) == "" {
		return "there"
	}
	return name
}
