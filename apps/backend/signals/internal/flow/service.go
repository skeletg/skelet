package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	emailpkg "github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/engagement"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sms"
)

// Caps how many nodes one advance() walks before parking, so a mis-wired graph with a cycle can't spin forever
const maxSteps = 64

// Admin CRUD, event enrollment and the scheduler-driven run engine - every advance holds the run's lease, taken by the claim or by a sync enrollment's create
type Service struct {
	repo       Repository
	tmpl       *emailtemplate.Service
	contacts   contactReader
	stores     storeReader
	sender     emailpkg.Sender
	sms        sms.Sender
	sends      sendTracker
	conditions *ConditionRegistry
	varSources *mergeVarProviders
	nodes      *NodeRegistry
	track      *emailtrack.Signer
	http       *http.Client
	allowPriv  bool
	suppress   suppressionChecker
}

// The read slice of the suppression list the email node needs - optional, nil means no suppression
type suppressionChecker interface {
	IsSuppressed(ctx context.Context, email string) bool
}

// unsubKey signs unsubscribe and tracking links, the same key core verifies them with
func NewService(repo Repository, tmpl *emailtemplate.Service, contacts contactReader, stores storeReader, sender emailpkg.Sender, smsSender sms.Sender, sends sendTracker, suppress suppressionChecker, allowPrivateWebhook bool, unsubKey []byte) *Service {
	s := &Service{
		repo: repo, tmpl: tmpl, contacts: contacts,
		stores: stores, sender: sender, sms: smsSender,
		sends: sends, suppress: suppress,
		conditions: NewConditionRegistry(),
		nodes:      NewNodeRegistry(),
		track:      emailtrack.NewSigner(unsubKey),
		http:       &http.Client{Transport: urlguard.SafeTransport(allowPrivateWebhook), Timeout: 10 * time.Second},
		allowPriv:  allowPrivateWebhook,
	}
	s.RegisterCondition(consentCondition{contacts: contacts})
	return s
}

// RegisterCondition adds a split condition the engine can evaluate - core registers the two it can answer alone, and the suites that own trait and segment data add theirs
func (s *Service) RegisterCondition(e ConditionEvaluator) { s.conditions.Register(e) }

// ConditionKinds is what validation accepts and the builder can offer, so an uninstalled suite's conditions simply aren't there
func (s *Service) ConditionKinds() []string { return s.conditions.Kinds() }

// --- admin CRUD ---

type Input struct {
	ChannelID     uuid.UUID
	Name          string
	TriggerType   string
	TriggerConfig json.RawMessage
	Graph         Graph
}

func (s *Service) validate(in Input) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("flow name is required")
	}
	if !validTrigger(in.TriggerType) {
		return fmt.Errorf("invalid trigger %q", in.TriggerType)
	}
	for _, n := range in.Graph.Nodes {
		switch n.Type {
		case NodeTrigger, NodeWait, NodeAddSegment, NodeWebhook:
		case NodeSplit:
			if n.Data.Condition == nil || !s.validConditionKind(n.Data.Condition.Kind) {
				return fmt.Errorf("split node %q: choose a valid condition", n.ID)
			}
		case NodeSendSMS:
			if strings.TrimSpace(n.Data.SMSBody) == "" {
				return fmt.Errorf("sms node %q: body is required", n.ID)
			}
		case NodeSendEmail:
			if err := s.tmpl.ValidateContent(n.Data.Blocks, n.Data.Styles); err != nil {
				return fmt.Errorf("email node %q: %w", n.ID, err)
			}
		default:
			return fmt.Errorf("unknown node type %q", n.Type)
		}
	}
	// Validate the optional enrollment filter carried in triggerConfig
	if len(in.TriggerConfig) > 0 {
		var tc TriggerConfig
		if err := json.Unmarshal(in.TriggerConfig, &tc); err != nil {
			return fmt.Errorf("invalid triggerConfig: %w", err)
		}
		if tc.EnrollmentFilter != nil && !s.validConditionKind(tc.EnrollmentFilter.Kind) {
			return fmt.Errorf("enrollment filter: choose a valid condition")
		}
	}
	return nil
}

func (s *Service) validateEmails(ctx context.Context, channelID uuid.UUID, g Graph) error {
	for _, n := range g.Nodes {
		if n.Type != NodeSendEmail || strings.TrimSpace(n.Data.EmailID) == "" {
			continue
		}
		if _, ok := s.ownEmail(ctx, channelID, n.Data.EmailID); !ok {
			return fmt.Errorf("email node %q: library email %s not found", n.ID, n.Data.EmailID)
		}
	}
	return nil
}

func (s *Service) ownEmail(ctx context.Context, channelID uuid.UUID, raw string) (emailtemplate.LibraryEmail, bool) {
	eid, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || s.tmpl == nil {
		return emailtemplate.LibraryEmail{}, false
	}
	e, ok, err := s.tmpl.GetLibraryEmail(ctx, eid)
	if err != nil || !ok || e.ChannelID != channelID {
		return emailtemplate.LibraryEmail{}, false
	}
	return e, true
}

func (s *Service) Create(ctx context.Context, in Input) (Flow, error) {
	if err := s.validate(in); err != nil {
		return Flow{}, err
	}
	if err := s.validateEmails(ctx, in.ChannelID, in.Graph); err != nil {
		return Flow{}, err
	}
	return s.repo.Create(ctx, Flow{ChannelID: in.ChannelID, Name: in.Name, TriggerType: in.TriggerType, TriggerConfig: in.TriggerConfig, Graph: in.Graph})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (Flow, error) {
	if err := s.validate(in); err != nil {
		return Flow{}, err
	}
	existing, found, err := s.repo.Get(ctx, id)
	if err != nil {
		return Flow{}, err
	}
	if !found {
		return Flow{}, fmt.Errorf("flow not found")
	}
	if err := s.validateEmails(ctx, existing.ChannelID, in.Graph); err != nil {
		return Flow{}, err
	}
	ok, err := s.repo.Update(ctx, Flow{ID: id, Name: in.Name, TriggerType: in.TriggerType, TriggerConfig: in.TriggerConfig, Graph: in.Graph})
	if err != nil {
		return Flow{}, err
	}
	if !ok {
		return Flow{}, fmt.Errorf("flow not found")
	}
	f, _, err := s.repo.Get(ctx, id)
	return f, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Flow, bool, error) {
	return s.repo.Get(ctx, id)
}
func (s *Service) List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Flow, error) {
	return s.repo.List(ctx, channelID, includeDeleted)
}

// Runs stay readable and the enrollment query skips it, so a deleted flow never enrolls anyone again
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

func (s *Service) Restore(ctx context.Context, id uuid.UUID) error { return s.repo.Restore(ctx, id) }

// Activating requires a trigger node wired to something
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status string) (Flow, error) {
	switch status {
	case StatusDraft, StatusActive, StatusPaused, StatusArchived:
	default:
		return Flow{}, fmt.Errorf("invalid status %q", status)
	}
	if status == StatusActive {
		f, ok, err := s.repo.Get(ctx, id)
		if err != nil || !ok {
			return Flow{}, err
		}
		if _, ok := f.Graph.triggerNode(); !ok {
			return Flow{}, fmt.Errorf("flow needs a trigger node before it can be activated")
		}
	}
	if _, err := s.repo.SetStatus(ctx, id, status); err != nil {
		return Flow{}, err
	}
	f, _, err := s.repo.Get(ctx, id)
	return f, err
}

func (s *Service) ListRuns(ctx context.Context, flowID uuid.UUID, limit int32) ([]FlowRun, error) {
	return s.repo.ListRuns(ctx, flowID, limit)
}

// Newest first, enriched with each flow's name and trigger, for the Synapse profile
func (s *Service) RunsForCustomer(ctx context.Context, customerID uuid.UUID) ([]CustomerRun, error) {
	return s.repo.RunsForCustomer(ctx, customerID)
}

// --- enrollment (core raises the moments over the contract) ---

func (s *Service) Enroll(ctx context.Context, trigger string, channelID, customerID uuid.UUID, email string, payload json.RawMessage, key string) int {
	n, err := s.EnrollMoment(ctx, trigger, channelID, customerID, email, payload, key)
	if err != nil {
		log.Printf("[flow] enroll %s: %v", trigger, err)
	}
	return n
}

// A flow took the moment when it started a run or already holds this key's, and an error means some flow wasn't asked, so a keyed moment is safe to raise again
func (s *Service) EnrollMoment(ctx context.Context, trigger string, channelID, customerID uuid.UUID, email string, payload json.RawMessage, key string) (int, error) {
	if customerID == uuid.Nil && email == "" {
		return 0, nil // no customer and no email - nothing to key a run on
	}
	if !validTrigger(trigger) {
		return 0, nil
	}
	flows, err := s.repo.ListActiveByTrigger(ctx, channelID, trigger)
	if err != nil {
		return 0, fmt.Errorf("list %s flows: %w", trigger, err)
	}
	sync := signalsapi.IsAuthTrigger(trigger)
	sending, err := s.channelSending(ctx, channelID)
	if err != nil {
		log.Printf("[flow] enroll %s: %v", trigger, err)
		return 0, nil // nothing can send under a channel off the roster, so the caller's own mail goes out
	}
	n := 0
	var errs []error
	for _, f := range flows {
		if !sending && !flowTransactional(f) {
			continue
		}
		run, took, err := s.enroll(ctx, f, customerID, email, payload, key, sync)
		if err != nil {
			errs = append(errs, fmt.Errorf("enroll %s/%s into %s: %w", customerID, email, f.ID, err))
			continue
		}
		if took {
			n++
		}
		// A sync run is created under its lease, so the engine can't claim it while this advance runs
		if sync && run != nil {
			if err := s.advance(context.WithoutCancel(ctx), *run); err != nil {
				log.Printf("[flow] sync advance %s: %v", run.ID, err)
			}
		}
	}
	return n, errors.Join(errs...)
}

func (s *Service) channelSending(ctx context.Context, channelID uuid.UUID) (bool, error) {
	if s.stores == nil {
		return true, nil
	}
	ch, err := s.stores.Get(ctx, channelID)
	if err != nil {
		return false, err
	}
	return ch.Active, nil
}

// Already in the flow, filtered out, or wired to nothing is not taken, so the caller's own mail still goes out
func (s *Service) enroll(ctx context.Context, f Flow, customerID uuid.UUID, email string, payload json.RawMessage, key string, sync bool) (*FlowRun, bool, error) {
	if key != "" {
		held, err := s.repo.HasRunForKey(ctx, f.ID, key)
		if err != nil {
			return nil, false, err
		}
		if held {
			return nil, true, nil
		}
	}
	guest := customerID == uuid.Nil
	var live int64
	var err error
	if guest {
		live, err = s.repo.CountLiveGuestRuns(ctx, f.ID, email)
	} else {
		live, err = s.repo.CountLiveRuns(ctx, f.ID, customerID)
	}
	if err != nil {
		return nil, false, err
	}
	if live > 0 {
		return nil, false, nil // one live run per customer per flow, and that run is some earlier moment's
	}
	// Flow-level enrollment filter gating entry on a condition, e.g. only active-lifecycle or marketing-opted-in customers - parsed from the flow's triggerConfig JSON so storage stays migration-free
	// Guests can't be evaluated against customer traits, so a filtered flow never enrolls them
	if len(f.TriggerConfig) > 0 {
		var tc TriggerConfig
		if json.Unmarshal(f.TriggerConfig, &tc) == nil && tc.EnrollmentFilter != nil {
			if guest || !s.evalConditionFor(ctx, tc.EnrollmentFilter, customerID) {
				return nil, false, nil
			}
		}
	}
	trig, ok := f.Graph.triggerNode()
	if !ok {
		return nil, false, nil
	}
	start, ok := f.Graph.next(trig.ID)
	if !ok {
		return nil, false, nil
	}
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	resume := time.Now()
	if sync {
		resume = resume.Add(runLease)
	}
	run, err := s.repo.CreateRun(ctx, FlowRun{
		FlowID: f.ID, CustomerID: customerID, Email: email, CurrentNodeID: start,
		Status: RunActive, ResumeAt: &resume, Context: payload, EnrollmentKey: key,
	})
	switch {
	case errors.Is(err, ErrMomentTaken):
		return nil, true, nil // a concurrent ask of this moment started its run first
	case errors.Is(err, ErrAlreadyInFlow):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	return &run, true, nil
}

// --- engine (scheduler job) ---

// How many runs one claim takes
const claimBatch = 100

// How long a claimed run is out of every other claim's reach - longer than any batch takes, and how late a run whose advancer died resumes
const runLease = 10 * time.Minute

// ProcessRuns is one batch of the engine, for callers that tick it by hand
func (s *Service) ProcessRuns(ctx context.Context) error {
	_, err := s.processBatch(ctx)
	return err
}

// Drain keeps claiming batches until nothing runnable is left or the budget is spent
func (s *Service) PruneFinishedRuns(ctx context.Context, before time.Time) (int64, error) {
	return s.repo.PruneFinishedRuns(ctx, before)
}

func (s *Service) Drain(ctx context.Context, budget time.Duration) (int, error) {
	deadline := time.Now().Add(budget)
	total := 0
	for {
		n, err := s.processBatch(ctx)
		total += n
		if err != nil || n < claimBatch || budget <= 0 || !time.Now().Before(deadline) || ctx.Err() != nil {
			return total, err
		}
	}
}

func (s *Service) processBatch(ctx context.Context) (int, error) {
	runs, err := s.repo.ClaimRunnable(ctx, claimBatch, runLease)
	if err != nil {
		return 0, err
	}
	// Preload the distinct flows referenced this tick so a flow with many enrolled runs is fetched once rather than per run - a flow whose preload errored falls back to the per-run fetch
	type cachedFlow struct {
		f  Flow
		ok bool
	}
	flowCache := make(map[uuid.UUID]cachedFlow, len(runs))
	for _, run := range runs {
		if _, seen := flowCache[run.FlowID]; seen {
			continue
		}
		f, ok, gerr := s.repo.Get(ctx, run.FlowID)
		if gerr != nil {
			log.Printf("[flow] preload flow %s: %v", run.FlowID, gerr)
			continue // leave unset → advance() retries the fetch per-run
		}
		flowCache[run.FlowID] = cachedFlow{f: f, ok: ok}
	}
	// A run that can't persist its new node is re-claimed and re-executed once its lease lapses, so these failures are how order receipts and auth links
	// get resent or stall mid-graph - they belong in the job's status, not only in stdout. One bad run still never stops the others
	var errs []error
	for _, run := range runs {
		var aerr error
		if cf, cached := flowCache[run.FlowID]; cached {
			aerr = s.advanceWithFlow(ctx, run, cf.f, cf.ok)
		} else {
			aerr = s.advance(ctx, run)
		}
		if aerr != nil {
			log.Printf("[flow] advance run %s: %v", run.ID, aerr)
			errs = append(errs, fmt.Errorf("advance run %s: %w", run.ID, aerr))
		}
	}
	return len(runs), errors.Join(errs...)
}

func (s *Service) advance(ctx context.Context, run FlowRun) error {
	f, ok, err := s.repo.Get(ctx, run.FlowID)
	if err != nil {
		return err
	}
	return s.advanceWithFlow(ctx, run, f, ok)
}

// advance with the flow already resolved, so the batch driver can preload each distinct flow once instead of re-fetching per enrolled run
func (s *Service) advanceWithFlow(ctx context.Context, run FlowRun, f Flow, ok bool) error {
	if !ok {
		return s.failRun(ctx, run, "flow no longer exists")
	}
	if f.Status != StatusActive {
		return nil // paused/deleted - leave the run; it resumes when reactivated
	}

	for range maxSteps {
		node, ok := f.Graph.node(run.CurrentNodeID)
		if !ok {
			return s.failRun(ctx, run, "node "+run.CurrentNodeID+" not found")
		}
		switch node.Type {
		case NodeWait:
			if run.Status == RunWaiting {
				nxt, ok := f.Graph.next(node.ID)
				if !ok {
					return s.completeRun(ctx, run)
				}
				run.CurrentNodeID, run.Status = nxt, RunActive
				continue
			}
			d := max(node.Data.DelaySeconds, 0)
			ra := time.Now().Add(time.Duration(d) * time.Second)
			run.ResumeAt, run.Status = &ra, RunWaiting
			return s.repo.UpdateRun(ctx, run)
		case NodeSplit:
			branch := "no"
			if s.evalCondition(ctx, node.Data.Condition, run) {
				branch = "yes"
			}
			nxt, ok := f.Graph.branch(node.ID, branch)
			if !ok {
				return s.completeRun(ctx, run)
			}
			run.CurrentNodeID = nxt
			continue
		case NodeSendEmail:
			s.execEmail(ctx, f, node, run)
		case NodeSendSMS:
			s.execSMS(ctx, f, node, run)
		case NodeWebhook:
			s.execWebhook(ctx, node, run)
		case NodeTrigger:
			// entry anchor - pass through
		default:
			if !s.runSideEffect(ctx, node, run) {
				return s.failRun(ctx, run, "unknown node type "+node.Type)
			}
		}
		nxt, ok := f.Graph.next(node.ID)
		if !ok {
			return s.completeRun(ctx, run)
		}
		run.CurrentNodeID = nxt
	}
	return s.failRun(ctx, run, "flow exceeded max steps (possible cycle)")
}

func (s *Service) completeRun(ctx context.Context, run FlowRun) error {
	run.Status, run.ResumeAt = RunCompleted, nil
	return s.repo.UpdateRun(ctx, run)
}

func (s *Service) failRun(ctx context.Context, run FlowRun, msg string) error {
	run.Status, run.ResumeAt, run.Error = RunFailed, nil, msg
	return s.repo.UpdateRun(ctx, run)
}

// --- node executors (best-effort: a failure logs + the flow continues) ---

func (s *Service) execEmail(ctx context.Context, f Flow, node Node, run FlowRun) {
	// Guest runs carry their recipient email on the run while customer runs load the account - guests only ever reach transactional order flows, which render from order data and send regardless of consent
	var cust Contact
	guest := run.IsGuest()
	if !guest {
		c, found, err := s.contacts.Get(ctx, run.CustomerID)
		if err == nil && !found {
			return
		}
		if err != nil {
			log.Printf("[flow] email node: load customer %s: %v", run.CustomerID, err)
			return
		}
		cust = c
	}
	recipient := cust.Email
	if guest {
		recipient = run.Email
	}
	if strings.TrimSpace(recipient) == "" {
		return
	}
	// Transactional flows send regardless of marketing consent and suppression - the customer has to receive them
	transactional := flowTransactional(f)
	if !transactional {
		if guest {
			return // a marketing flow can't send to a consent-less guest
		}
		if !cust.EmailOptedIn {
			return // not opted in to marketing email
		}
		// Deliverability guard - skip addresses on the suppression list from a bounce, complaint or manual block
		if s.suppress != nil && s.suppress.IsSuppressed(ctx, recipient) {
			return
		}
	}
	ch, err := s.stores.Get(ctx, f.ChannelID)
	if err != nil || (!ch.Active && !transactional) {
		return
	}
	vars := s.mergeVars(ctx, ch, cust, run)
	var subj, html, text string
	switch {
	// A transactional order flow renders the chosen email from the snapshot its enrollment carried - items, totals, tracking, ship-to blocks
	case transactional && isOrderTrigger(f.TriggerType) && hasOrderSnapshot(run):
		subj, html, text, err = s.renderOrderEmail(ctx, ch, f, node, run)
	case strings.TrimSpace(node.Data.EmailID) != "":
		e, ok := s.ownEmail(ctx, f.ChannelID, node.Data.EmailID)
		if !ok {
			log.Printf("[flow] email node %s: library email %q is not one of the flow's channel", node.ID, node.Data.EmailID)
			return
		}
		subj, html, text, err = s.tmpl.RenderLibraryEmail(ctx, e.ID, vars)
	default:
		subj, html, text, err = s.tmpl.RenderContent(ctx, ch.ID, node.Data.Subject, "", node.Data.Blocks, node.Data.Styles, vars)
	}
	if err != nil {
		log.Printf("[flow] email node render: %v", err)
		return
	}
	// Track the send on the analytics spine, where opens and clicks resolve via /e/o and /e/c
	// Transactional sends get open-pixel tracking ONLY, never click-wrapping, so one-time verify and reset links stay un-redirected - a link-prefetcher or a downed tracking endpoint must never sit in front of a password-reset link
	flowID := f.ID
	runID := run.ID
	var custIDPtr *uuid.UUID
	if !guest {
		id := cust.ID
		custIDPtr = &id
	}
	// Order-triggered runs stamp the order, so a receipt is attributable to it whichever sender delivered it
	var orderIDPtr *uuid.UUID
	if id := orderIDFromRun(run); id != uuid.Nil {
		orderIDPtr = &id
	}
	sendID := engagement.NewSendID()
	s.recordSent(ctx, sendlog.Spec{
		ID: sendID, ChannelID: ch.ID, Source: sendlog.SourceFlow, Email: recipient, CustomerID: custIDPtr,
		FlowID: &flowID, FlowRunID: &runID, FlowNodeID: node.ID, OrderID: orderIDPtr,
	})
	if transactional {
		html = s.track.InjectPixel(html, storefrontURL(ch), sendID)
	} else {
		html = s.track.Inject(html, storefrontURL(ch), sendID)
	}
	msg := emailpkg.Message{From: s.fromAddress(ch), To: recipient, Subject: subj, HTMLBody: html, TextBody: text}
	if !transactional {
		msg.Headers = emailpkg.ListUnsubscribeHeaders(s.unsubscribeLink(ch, cust.ID))
	}
	if err := s.sender.Send(ctx, msg); err != nil {
		log.Printf("[flow] email node send to %s: %v", recipient, err)
	}
}

// Gated on marketing_sms consent and a number on file - best-effort, failures are logged
func (s *Service) execSMS(ctx context.Context, f Flow, node Node, run FlowRun) {
	if s.sms == nil || run.IsGuest() {
		return // no SMS to a guest - no account / number on file
	}
	cust, found, err := s.contacts.Get(ctx, run.CustomerID)
	if err == nil && !found {
		return
	}
	if err != nil {
		log.Printf("[flow] sms node: load customer %s: %v", run.CustomerID, err)
		return
	}
	phone := strings.TrimSpace(cust.Phone)
	if phone == "" {
		return // no number on file
	}
	if !cust.SMSOptedIn {
		return // not opted in to marketing SMS
	}
	ch, err := s.stores.Get(ctx, f.ChannelID)
	if err != nil || !ch.Active {
		return
	}
	body := renderText(node.Data.SMSBody, s.mergeVars(ctx, ch, cust, run))
	if strings.TrimSpace(body) == "" {
		return
	}
	if err := s.sms.Send(ctx, phone, body); err != nil {
		log.Printf("[flow] sms node send to %s: %v", phone, err)
	}
}

// The merge-tag map shared by the email and SMS nodes - base store and customer vars, the cart-recovery link, and any browse-product context
func (s *Service) mergeVars(ctx context.Context, ch Store, cust Contact, run FlowRun) map[string]string {
	vars := s.vars(ctx, ch, cust)
	vars["cart.recoveryUrl"] = storefrontURL(ch) + "/cart"
	if len(run.Context) > 0 {
		var rc map[string]any
		if json.Unmarshal(run.Context, &rc) == nil {
			if pk, _ := rc["page_key"].(string); pk != "" {
				vars["product.url"] = storefrontURL(ch) + pk
			}
			// back_in_stock carries a resolved full product URL
			if u, _ := rc["page_key_url"].(string); u != "" {
				if strings.HasPrefix(u, "http") {
					vars["product.url"] = u
				} else {
					vars["product.url"] = storefrontURL(ch) + u
				}
			}
			if v, _ := rc["product_name"].(string); v != "" {
				vars["product.name"] = v
			}
			if v, _ := rc["product_image"].(string); v != "" {
				vars["product.image"] = v
			}
			if v, _ := rc["segment_slug"].(string); v != "" {
				vars["segment.slug"] = v
			}
			if v, _ := rc["form_slug"].(string); v != "" {
				vars["form.slug"] = v
			}
			// Order and auth-lifecycle context, from the transactional triggers
			if v, _ := rc["order_number"].(string); v != "" {
				vars["order.number"] = v
			}
			if v, _ := rc["verify_link"].(string); v != "" {
				vars["verify.link"] = v
			}
			if v, _ := rc["reset_link"].(string); v != "" {
				vars["reset.link"] = v
			}
			if v, _ := rc["reset_expires_in"].(string); v != "" {
				vars["reset.expiresIn"] = v
			}
			if v, _ := rc["confirm_link"].(string); v != "" {
				vars["confirm.link"] = v
			}
			if v, _ := rc["confirm_expires_in"].(string); v != "" {
				vars["confirm.expiresIn"] = v
			}
			// Auth triggers pass an explicit name, since the customer may be brand new
			if v, _ := rc["first_name"].(string); v != "" {
				vars["customer.firstName"] = v
			}
		}
	}
	return vars
}

func renderText(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
		out = strings.ReplaceAll(out, "{{ "+k+" }}", v)
	}
	return out
}

func (s *Service) execWebhook(ctx context.Context, node Node, run FlowRun) {
	url := strings.TrimSpace(node.Data.URL)
	if url == "" {
		return
	}
	if err := urlguard.ValidateTargetURL(url, s.allowPriv); err != nil {
		log.Printf("[flow] webhook node blocked %s: %v", url, err)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"flowId":     run.FlowID.String(),
		"customerId": run.CustomerID.String(),
		"context":    json.RawMessage(run.Context),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		log.Printf("[flow] webhook node POST %s: %v", url, err)
		return
	}
	_ = resp.Body.Close()
}

func (s *Service) evalCondition(ctx context.Context, c *Condition, run FlowRun) bool {
	return s.evalConditionFor(ctx, c, run.CustomerID)
}

// The shared condition evaluator for split branches and the enrollment filter, evaluated per-customer at runtime, with Negate inverting the base result
// A nil condition, an unknown kind, or a read failure all resolve to false - the negative branch, or "filtered out" - rather than erroring the run
func (s *Service) evalConditionFor(ctx context.Context, c *Condition, customerID uuid.UUID) bool {
	if c == nil {
		return false
	}
	base := s.evalConditionBase(ctx, c, customerID)
	if c.Negate {
		return !base
	}
	return base
}

func (s *Service) evalConditionBase(ctx context.Context, c *Condition, customerID uuid.UUID) bool {
	e, ok := s.conditions.Lookup(c.Kind)
	if !ok {
		return false // an unregistered kind means the suite that answers it isn't installed
	}
	return e.Matches(ctx, customerID, c)
}

// --- email vars / from / unsubscribe (shares the campaign token scheme) ---

func (s *Service) vars(ctx context.Context, ch Store, cust Contact) map[string]string {
	first := strings.TrimSpace(cust.FirstName)
	if first == "" {
		first = "there"
	}
	m := map[string]string{
		"store.name":         ch.Name,
		"store.logoUrl":      strings.TrimSpace(ch.LogoURL),
		"storefront.url":     storefrontURL(ch),
		"customer.firstName": first,
		"customer.email":     cust.Email,
		"unsubscribe.link":   s.unsubscribeLink(ch, cust.ID),
	}
	// Personalization tags the suite computes - absent ones just render empty, which is what a failed read already did
	s.varSources.merge(ctx, ch.ID, cust.ID, m)
	return m
}

func (s *Service) unsubscribeLink(ch Store, customerID uuid.UUID) string {
	return storefrontURL(ch) + "/unsubscribe/" + s.track.SignUnsubscribe(customerID)
}

// The channel's configured sender identity as core pushed it, with the store's name when no sender
// name was ever set - an empty From is refused by every relay
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
