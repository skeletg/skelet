package flow

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/analytics"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/consent"
	"github.com/skeletg/skelet/internal/customer"
	"github.com/skeletg/skelet/internal/email"
	"github.com/skeletg/skelet/internal/emailtemplate"
	"github.com/skeletg/skelet/internal/emailtrack"
	"github.com/skeletg/skelet/internal/order"
	"github.com/skeletg/skelet/internal/remoteprovider"
	"github.com/skeletg/skelet/internal/segment"
	"github.com/skeletg/skelet/internal/sms"
	"github.com/skeletg/skelet/internal/webhook"
)

const marketingConsentKind = "marketing_email"
const smsConsentKind = "marketing_sms"

// Caps how many nodes one advance() walks before parking, so a mis-wired graph with a cycle can't spin forever
const maxSteps = 64

// Admin CRUD, event enrollment and the scheduler-driven run engine - the scheduler job lease serializes ProcessRuns, so advancement needs no extra cross-replica locking
type Service struct {
	repo      Repository
	tmpl      *emailtemplate.Service
	customers *customer.Service
	segments  *segment.Service
	consent   *consent.Service
	channels  *channel.Service
	config    *config.Service
	sender    email.Sender
	sms       sms.Sender
	recorder  *analytics.Recorder
	track     *emailtrack.Signer
	http      *http.Client
	allowPriv bool
	unsubKey  []byte
	suppress  suppressionChecker
	orders    orderReader
	// Back the remote_action node (see remote_action.go) - both nil makes those nodes no-op, and main.go wires them via SetRemoteActions
	remoteProviders *remoteprovider.Service
	remoteInvoker   *remoteprovider.Invoker
	// The server-wide Signals switch - marketing flows stop enrolling and their runs park like a paused flow, while transactional ones (order receipts, auth links) keep enrolling and advancing
	disabled bool
}

// The read slice of the order service the transactional send_email node needs for order-context email data - optional, and nil means order-triggered flows render with flat merge tags only
type orderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
	GetFulfillment(ctx context.Context, id uuid.UUID) (order.Fulfillment, error)
	PaymentLedger(ctx context.Context, orderID uuid.UUID) ([]order.PaymentTransaction, error)
}

// Lets transactional order flows populate the order_items, order_totals, tracking and shipping_address blocks
func (s *Service) SetOrderReader(o orderReader) { s.orders = o }

// The read slice of the suppression service the email node needs - optional, nil means no suppression
type suppressionChecker interface {
	IsSuppressed(ctx context.Context, email string) bool
}

func (s *Service) SetSuppression(sc suppressionChecker) { s.suppress = sc }

func (s *Service) SetEnabled(v bool) { s.disabled = !v }

// unsubKey signs unsubscribe links, sharing the campaign scheme so the same /unsubscribe endpoint validates both
func NewService(repo Repository, tmpl *emailtemplate.Service, customers *customer.Service, segments *segment.Service, cons *consent.Service, channels *channel.Service, cfg *config.Service, sender email.Sender, smsSender sms.Sender, recorder *analytics.Recorder, allowPrivateWebhook bool, unsubKey []byte) *Service {
	return &Service{
		repo: repo, tmpl: tmpl, customers: customers, segments: segments, consent: cons,
		channels: channels, config: cfg, sender: sender, sms: smsSender,
		recorder:  recorder,
		track:     emailtrack.NewSigner(unsubKey),
		http:      &http.Client{Transport: webhook.SafeTransport(allowPrivateWebhook), Timeout: 10 * time.Second},
		allowPriv: allowPrivateWebhook,
		unsubKey:  unsubKey,
	}
}

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
		case NodeRemoteAction:
			if strings.TrimSpace(n.Data.Provider) == "" {
				return fmt.Errorf("remote action node %q: provider is required", n.ID)
			}
		case NodeSplit:
			if n.Data.Condition == nil || !validConditionKind(n.Data.Condition.Kind) {
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
		if tc.EnrollmentFilter != nil && !validConditionKind(tc.EnrollmentFilter.Kind) {
			return fmt.Errorf("enrollment filter: choose a valid condition")
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, in Input) (Flow, error) {
	if err := s.validate(in); err != nil {
		return Flow{}, err
	}
	return s.repo.Create(ctx, Flow{ChannelID: in.ChannelID, Name: in.Name, TriggerType: in.TriggerType, TriggerConfig: in.TriggerConfig, Graph: in.Graph})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (Flow, error) {
	if err := s.validate(in); err != nil {
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

// --- enrollment (called from event-bus subscribers in main.go) ---

// customerID may be uuid.Nil for a guest checkout - the run is then keyed by the order email, so guests get their receipt too - Returns the number of active flows
func (s *Service) OnOrderPlaced(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID uuid.UUID, orderNumber string) int {
	return s.enrollAll(ctx, channelID, customerID, email, TriggerOrderPlaced, map[string]any{"order_id": orderID.String(), "order_number": orderNumber})
}

// Customer or guest by email - the fulfillment id rides the context for tracking and lines
func (s *Service) OnOrderShipped(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID, fulfillmentID uuid.UUID, orderNumber string) int {
	return s.enrollAll(ctx, channelID, customerID, email, TriggerOrderShipped, map[string]any{
		"order_id": orderID.String(), "order_number": orderNumber, "fulfillment_id": fulfillmentID.String(),
	})
}

func (s *Service) OnOrderCancelled(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID uuid.UUID, orderNumber string) int {
	return s.enrollAll(ctx, channelID, customerID, email, TriggerOrderCancelled, map[string]any{
		"order_id": orderID.String(), "order_number": orderNumber,
	})
}

// Customer or guest - ref is the dedup reference (refund txn id or status) carried downstream
func (s *Service) OnPaymentRefunded(ctx context.Context, channelID, customerID uuid.UUID, email string, orderID uuid.UUID, orderNumber, ref string) int {
	return s.enrollAll(ctx, channelID, customerID, email, TriggerPaymentRefunded, map[string]any{
		"order_id": orderID.String(), "order_number": orderNumber, "refund_ref": ref,
	})
}

// Runs SYNCHRONOUSLY, since the link has to go out in the request rather than on the poll tick, with the verification link and store name riding the context
// Returns the number of active flows - 0 means the caller sends the built-in directly
func (s *Service) OnEmailVerification(ctx context.Context, channelID, customerID uuid.UUID, verifyLink, storeName, firstName string) int {
	return s.enrollSyncAll(ctx, channelID, customerID, "", TriggerEmailVerification, map[string]any{
		"verify_link": verifyLink, "store_name": storeName, "first_name": firstName,
	})
}

// Runs SYNCHRONOUSLY - the reset link and expiry ride the context
func (s *Service) OnPasswordReset(ctx context.Context, channelID, customerID uuid.UUID, resetLink, expiresIn, storeName, firstName string) int {
	return s.enrollSyncAll(ctx, channelID, customerID, "", TriggerPasswordReset, map[string]any{
		"reset_link": resetLink, "reset_expires_in": expiresIn, "store_name": storeName, "first_name": firstName,
	})
}

func (s *Service) OnCustomerRegistered(ctx context.Context, channelID, customerID uuid.UUID) {
	s.enrollAll(ctx, channelID, customerID, "", TriggerCustomerRegistered, map[string]any{})
}

// For a known customer the customerID keys the run; for a guest cart it's uuid.Nil and the email captured at checkout entry keys it instead, so guest carts recover too
// The cart_id rides the run context so the recovery email can link back - Returns the number enrolled - 0 when none are active or the customer is already in-flow
func (s *Service) OnCartAbandoned(ctx context.Context, channelID, customerID uuid.UUID, email string, cartID uuid.UUID) {
	s.enrollAll(ctx, channelID, customerID, email, TriggerCartAbandoned, map[string]any{"cart_id": cartID.String()})
}

// The browsed product (page, sku, name, image) rides the run context so the recovery email can show it through {{ product.* }} merge tags
func (s *Service) OnBrowseAbandoned(ctx context.Context, channelID, customerID uuid.UUID, pageKey, sku, name, image string) {
	s.enrollAll(ctx, channelID, customerID, "", TriggerBrowseAbandoned, map[string]any{
		"page_key": pageKey, "sku": sku, "product_name": name, "product_image": image,
	})
}

// Wired from the analytics bus event for email_opened / email_clicked - only human, non-machine engagement publishes one
func (s *Service) OnEmailEngaged(ctx context.Context, channelID, customerID uuid.UUID, trigger string) {
	s.enrollAll(ctx, channelID, customerID, "", trigger, map[string]any{})
}

// Fires when a customer newly joins a computed Synapse trait or behavioral segment, from the segment refresh job
// The entered slug rides the run context as {{ segment.slug }} - to target one segment, gate the flow with a split node's in-segment condition
func (s *Service) OnSegmentEntered(ctx context.Context, channelID, customerID uuid.UUID, segmentSlug string) {
	s.enrollAll(ctx, channelID, customerID, "", TriggerSegmentEntered, map[string]any{"segment_slug": segmentSlug})
}

// The product rides the run context as {{ product.name/image/url }}
// Returns the number of active back_in_stock flows - the notifier treats a zero return (no flow, or a guest with no account) as its cue to send a direct built-in notification, so everyone who asked is told whether or not an operator built a flow
func (s *Service) OnBackInStock(ctx context.Context, channelID, customerID uuid.UUID, sku, name, image, url string) int {
	return s.enrollAll(ctx, channelID, customerID, "", TriggerBackInStock, map[string]any{
		"sku": sku, "product_name": name, "product_image": image, "page_key_url": url,
	})
}

// Fires on a signup-form submission for a welcome series, with the form id and slug riding the run context - returns the number of active flows enrolled into
func (s *Service) OnFormSubmitted(ctx context.Context, channelID, customerID uuid.UUID, formSlug string) int {
	return s.enrollAll(ctx, channelID, customerID, "", TriggerFormSubmitted, map[string]any{"form_slug": formSlug})
}

// Returns how many active flows there were, regardless of whether enrollment was skipped as already-in-flow - guests enroll into nothing and return 0, since flows target known customers
func (s *Service) enrollAll(ctx context.Context, channelID, customerID uuid.UUID, email, trigger string, context map[string]any) int {
	if customerID == uuid.Nil && email == "" {
		return 0 // no customer and no email - nothing to key a run on
	}
	flows, err := s.repo.ListActiveByTrigger(ctx, channelID, trigger)
	if err != nil {
		log.Printf("[flow] list %s flows: %v", trigger, err)
		return 0
	}
	n := 0
	for _, f := range flows {
		// Signals off - transactional flows keep delivering, marketing flows don't enroll
		if s.disabled && !flowTransactional(f) {
			continue
		}
		n++
		if _, err := s.enroll(ctx, f, customerID, email, context); err != nil {
			log.Printf("[flow] enroll %s/%s into %s: %v", customerID, email, f.ID, err)
		}
	}
	return n
}

// Advances each new run inline in the caller's goroutine so the email is sent before this returns - used for auth-lifecycle triggers whose one-time link can't wait for the engine's poll tick
// Deliberately NOT gated on the Signals switch: it only serves email verification and password reset, which have no built-in fallback and must keep sending with Signals off
func (s *Service) enrollSyncAll(ctx context.Context, channelID, customerID uuid.UUID, email, trigger string, context map[string]any) int {
	if customerID == uuid.Nil && email == "" {
		return 0
	}
	flows, err := s.repo.ListActiveByTrigger(ctx, channelID, trigger)
	if err != nil {
		log.Printf("[flow] list %s flows: %v", trigger, err)
		return 0
	}
	for _, f := range flows {
		run, err := s.enroll(ctx, f, customerID, email, context)
		if err != nil {
			log.Printf("[flow] enroll %s into %s: %v", customerID, f.ID, err)
			continue
		}
		if run != nil {
			if err := s.advance(ctx, *run); err != nil {
				log.Printf("[flow] sync advance %s: %v", run.ID, err)
			}
		}
	}
	return len(flows)
}

// Creates a run for the customer, or a guest keyed by email - nil run when skipped, meaning already in-flow, filtered out, or a trigger wired to nothing
func (s *Service) enroll(ctx context.Context, f Flow, customerID uuid.UUID, email string, context map[string]any) (*FlowRun, error) {
	guest := customerID == uuid.Nil
	var live int64
	var err error
	if guest {
		live, err = s.repo.CountLiveGuestRuns(ctx, f.ID, email)
	} else {
		live, err = s.repo.CountLiveRuns(ctx, f.ID, customerID)
	}
	if err != nil {
		return nil, err
	}
	if live > 0 {
		return nil, nil // already in this flow
	}
	// Flow-level enrollment filter gating entry on a condition, e.g. only active-lifecycle or marketing-opted-in customers - parsed from the flow's triggerConfig JSON so storage stays migration-free
	// Guests can't be evaluated against customer traits, so a filtered flow never enrolls them
	if len(f.TriggerConfig) > 0 {
		var tc TriggerConfig
		if json.Unmarshal(f.TriggerConfig, &tc) == nil && tc.EnrollmentFilter != nil {
			if guest || !s.evalConditionFor(ctx, tc.EnrollmentFilter, customerID) {
				return nil, nil // filtered out - do not enroll
			}
		}
	}
	trig, ok := f.Graph.triggerNode()
	if !ok {
		return nil, nil
	}
	start, ok := f.Graph.next(trig.ID)
	if !ok {
		return nil, nil // trigger wired to nothing
	}
	ctxJSON, _ := json.Marshal(context)
	now := time.Now()
	run, err := s.repo.CreateRun(ctx, FlowRun{
		FlowID: f.ID, CustomerID: customerID, Email: email, CurrentNodeID: start,
		Status: RunActive, ResumeAt: &now, Context: ctxJSON,
	})
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// --- engine (scheduler job) ---

// Advances every runnable flow run one or more steps - driven by the flow-engine scheduler job, serialized by the job lease
func (s *Service) ProcessRuns(ctx context.Context) error {
	runs, err := s.repo.ClaimRunnable(ctx, 100)
	if err != nil {
		return err
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
	// A run that can't persist its new node is re-claimed and re-executed next tick, so these failures are how order receipts and auth links
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
	return errors.Join(errs...)
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
	// Signals off - marketing runs park with the same semantics as a paused flow and resume if the switch comes back, while transactional runs keep advancing
	if s.disabled && !flowTransactional(f) {
		return nil
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
		case NodeAddSegment:
			s.execAddSegment(ctx, node, run)
		case NodeWebhook:
			s.execWebhook(ctx, node, run)
		case NodeRemoteAction:
			// Request/response - merge the returned context into the run and follow the provider's named branch when it has a labeled edge, else fall through to the default "out"
			branch := s.execRemoteAction(ctx, node, &run)
			if branch != "" {
				if nxt, ok := f.Graph.branch(node.ID, branch); ok {
					run.CurrentNodeID = nxt
					continue
				}
			}
		case NodeTrigger:
			// entry anchor - pass through
		default:
			return s.failRun(ctx, run, "unknown node type "+node.Type)
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
	var cust customer.Customer
	guest := run.IsGuest()
	if !guest {
		c, err := s.customers.GetByID(ctx, run.CustomerID)
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
		if optedIn, _ := s.consent.IsOptedIn(ctx, cust.ID, marketingConsentKind); !optedIn {
			return // not opted in to marketing email
		}
		// Deliverability guard - skip addresses on the suppression list from a bounce, complaint or manual block
		if s.suppress != nil && s.suppress.IsSuppressed(ctx, recipient) {
			return
		}
	}
	ch, err := s.channels.GetByID(ctx, f.ChannelID)
	if err != nil {
		return
	}
	vars := s.mergeVars(ctx, ch, cust, run)
	var subj, html, text string
	switch {
	// A transactional order flow with order context renders the chosen email with full order data - items, totals, tracking, ship-to blocks
	case transactional && s.orders != nil && isOrderTrigger(f.TriggerType) && orderIDFromRun(run) != uuid.Nil:
		subj, html, text, err = s.renderOrderEmail(ctx, ch, f, node, run)
	case strings.TrimSpace(node.Data.EmailID) != "":
		eid, perr := uuid.Parse(strings.TrimSpace(node.Data.EmailID))
		if perr != nil {
			log.Printf("[flow] email node bad emailId %q: %v", node.Data.EmailID, perr)
			return
		}
		subj, html, text, err = s.tmpl.RenderLibraryEmail(ctx, eid, vars)
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
	if send, serr := s.recorder.NewSend(ctx, analytics.SendSpec{
		ChannelID: ch.ID, Source: analytics.SourceFlow, Email: recipient, CustomerID: custIDPtr,
		FlowID: &flowID, FlowRunID: &runID, FlowNodeID: node.ID,
	}); serr == nil {
		_ = s.recorder.RecordSent(ctx, send)
		if transactional {
			html = s.track.InjectPixel(html, storefrontURL(ch), send.ID)
		} else {
			html = s.track.Inject(html, storefrontURL(ch), send.ID)
		}
	} else {
		log.Printf("[flow] email node track send: %v", serr)
	}
	if err := s.sender.Send(ctx, email.Message{From: s.fromAddress(ctx, ch), To: recipient, Subject: subj, HTMLBody: html, TextBody: text}); err != nil {
		log.Printf("[flow] email node send to %s: %v", recipient, err)
	}
}

// Gated on marketing_sms consent and a number on file - best-effort, failures are logged
func (s *Service) execSMS(ctx context.Context, f Flow, node Node, run FlowRun) {
	if s.sms == nil || run.IsGuest() {
		return // no SMS to a guest - no account / number on file
	}
	cust, err := s.customers.GetByID(ctx, run.CustomerID)
	if err != nil {
		log.Printf("[flow] sms node: load customer %s: %v", run.CustomerID, err)
		return
	}
	phone := ""
	if cust.Phone != nil {
		phone = strings.TrimSpace(*cust.Phone)
	}
	if phone == "" {
		return // no number on file
	}
	if optedIn, _ := s.consent.IsOptedIn(ctx, cust.ID, smsConsentKind); !optedIn {
		return // not opted in to marketing SMS
	}
	ch, err := s.channels.GetByID(ctx, f.ChannelID)
	if err != nil {
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
func (s *Service) mergeVars(ctx context.Context, ch channel.Channel, cust customer.Customer, run FlowRun) map[string]string {
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

func (s *Service) execAddSegment(ctx context.Context, node Node, run FlowRun) {
	if run.IsGuest() {
		return // no customer to add to a segment
	}
	sid, err := uuid.Parse(node.Data.SegmentID)
	if err != nil {
		return
	}
	if err := s.segments.AssignCustomers(ctx, sid, []uuid.UUID{run.CustomerID}); err != nil {
		log.Printf("[flow] add_segment %s: %v", sid, err)
	}
}

func (s *Service) execWebhook(ctx context.Context, node Node, run FlowRun) {
	url := strings.TrimSpace(node.Data.URL)
	if url == "" {
		return
	}
	if err := webhook.ValidateTargetURL(url, s.allowPriv); err != nil {
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
	switch c.Kind {
	case "segment", conditionInSegmentAlias:
		sid, err := uuid.Parse(c.SegmentID)
		if err != nil {
			return false
		}
		ids, err := s.segments.MemberIDs(ctx, sid)
		if err != nil {
			return false
		}
		return slices.Contains(ids, customerID)

	case "lifecycle":
		if len(c.Lifecycle) == 0 {
			return false
		}
		t, ok, err := s.recorder.CustomerTraits(ctx, customerID)
		if err != nil || !ok {
			return false
		}
		return slices.Contains(c.Lifecycle, t.Lifecycle)

	case "rfm":
		t, ok, err := s.recorder.CustomerTraits(ctx, customerID)
		if err != nil || !ok {
			return false
		}
		return t.RFMRecency >= c.MinRecency &&
			t.RFMFrequency >= c.MinFrequency &&
			t.RFMMonetary >= c.MinMonetary &&
			t.EngagementScore >= c.MinEngagement

	case "email_activity":
		eng, err := s.recorder.CustomerEmailEngagement(ctx, customerID)
		if err != nil {
			return false
		}
		return eng.Opened >= c.MinOpens && eng.Clicked >= c.MinClicks

	case "consent":
		kind := strings.TrimSpace(c.ConsentKind)
		if kind == "" {
			return false
		}
		optedIn, _ := s.consent.IsOptedIn(ctx, customerID, kind)
		return optedIn == c.OptedIn

	default:
		return false
	}
}

// --- email vars / from / unsubscribe (shares the campaign token scheme) ---

func (s *Service) vars(ctx context.Context, ch channel.Channel, cust customer.Customer) map[string]string {
	logo := ""
	if s.config != nil {
		id := ch.ID
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
		logo, _ = s.config.GetString(ctx, "general/branding/logo_url", scope)
	}
	first := strings.TrimSpace(cust.FirstName)
	if first == "" {
		first = "there"
	}
	m := map[string]string{
		"store.name":         ch.Name,
		"store.logoUrl":      strings.TrimSpace(logo),
		"storefront.url":     storefrontURL(ch),
		"customer.firstName": first,
		"customer.email":     cust.Email,
		"unsubscribe.link":   storefrontURL(ch) + "/unsubscribe/" + s.signToken(cust.ID),
	}
	// Synapse traits and segment membership for personalization, so an email can say "you're one of our {{customer.lifecycle}} customers" - best-effort, absent tags just render empty
	if s.recorder != nil {
		if t, ok, _ := s.recorder.CustomerTraits(ctx, cust.ID); ok {
			m["customer.lifecycle"] = t.Lifecycle
			m["customer.orders"] = fmt.Sprintf("%d", t.Frequency)
			m["customer.ltv"] = fmt.Sprintf("%.2f", float64(t.MonetaryCents)/100)
		}
		// Attribute affinities become {{customer.affinity.<slug>}}, e.g. "your size, 205/55R16, is back in stock"
		if affs, _ := s.recorder.CustomerTopAffinities(ctx, cust.ID, time.Now().AddDate(0, 0, -90)); len(affs) > 0 {
			for slug, val := range affs {
				m["customer.affinity."+slug] = val
			}
		}
	}
	if s.segments != nil {
		if slugs, err := s.segments.SlugsForCustomer(ctx, cust.ID); err == nil {
			m["customer.segments"] = strings.Join(slugs, ", ")
		}
	}
	return m
}

func (s *Service) fromAddress(ctx context.Context, ch channel.Channel) string {
	addr, name := "no-reply@skelet.local", ""
	if s.config != nil {
		id := ch.ID
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
		if a, _ := s.config.GetString(ctx, "general/contact/email_from_address", scope); strings.TrimSpace(a) != "" {
			addr = strings.TrimSpace(a)
		}
		name, _ = s.config.GetString(ctx, "general/contact/email_from_name", scope)
		name = strings.TrimSpace(name)
	}
	if name == "" {
		name = ch.Name
	}
	if name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}

func (s *Service) signToken(customerID uuid.UUID) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(customerID.String()))
	mac := hmac.New(sha256.New, s.unsubKey)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func storefrontURL(ch channel.Channel) string {
	return strings.TrimRight(strings.TrimSpace(ch.PublicURL), "/")
}
