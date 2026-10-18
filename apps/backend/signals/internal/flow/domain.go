// Package flow implements marketing automation - a visual node graph customers run through on an event trigger
// An event enrolls a customer, then a durable per-run state machine walks the graph (send, wait, branch, tag, webhook), advanced by a scheduler job
package flow

import (
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

// The triggers are the contract's vocabulary - core raises them by name, and both sides compile the one list
const (
	TriggerOrderPlaced        = signalsapi.TriggerOrderPlaced
	TriggerOrderShipped       = signalsapi.TriggerOrderShipped
	TriggerCustomerRegistered = signalsapi.TriggerCustomerRegistered
	TriggerOrderCancelled     = signalsapi.TriggerOrderCancelled
	TriggerPaymentRefunded    = signalsapi.TriggerPaymentRefunded
	TriggerCartAbandoned      = signalsapi.TriggerCartAbandoned
	TriggerBrowseAbandoned    = signalsapi.TriggerBrowseAbandoned
	TriggerEmailOpened        = signalsapi.TriggerEmailOpened
	TriggerEmailClicked       = signalsapi.TriggerEmailClicked
	TriggerSegmentEntered     = signalsapi.TriggerSegmentEntered
	TriggerBackInStock        = signalsapi.TriggerBackInStock
	TriggerFormSubmitted      = signalsapi.TriggerFormSubmitted
	TriggerEmailVerification  = signalsapi.TriggerEmailVerification
	TriggerPasswordReset      = signalsapi.TriggerPasswordReset
	TriggerSignupConfirmation = signalsapi.TriggerSignupConfirmation
	TriggerAccountExists      = signalsapi.TriggerAccountExists
	TriggerCartOffer          = signalsapi.TriggerCartOffer
)

const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusPaused   = "paused"
	StatusArchived = "archived"
)

const (
	RunActive    = "active"
	RunWaiting   = "waiting"
	RunCompleted = "completed"
	RunCancelled = "cancelled"
	RunFailed    = "failed"
)

const (
	NodeTrigger    = "trigger"
	NodeSendEmail  = "send_email"
	NodeSendSMS    = "send_sms"
	NodeWait       = "wait"
	NodeSplit      = "split"
	NodeAddSegment = "add_segment"
	NodeWebhook    = "webhook"
)

type Flow struct {
	ID            uuid.UUID
	ChannelID     uuid.UUID
	Name          string
	TriggerType   string
	TriggerConfig json.RawMessage
	Graph         Graph
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Set when soft-deleted, nil means live - distinct from Status "archived", which is an operator lifecycle state
	DeletedAt *time.Time
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Data holds the type-specific config superset
type Node struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Position Position `json:"position"`
	Data     NodeData `json:"data"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// The union of every node type's config - only the fields relevant to a node's Type are populated
type NodeData struct {
	Label string `json:"label,omitempty"`
	// send_email - references a reusable library email, and inline subject/blocks still render when this is empty
	EmailID      string                    `json:"emailId,omitempty"`
	Subject      string                    `json:"subject,omitempty"`
	Blocks       []emailtemplate.Block     `json:"blocks,omitempty"`
	Styles       emailtemplate.EmailStyles `json:"styles"`
	SMSBody      string                    `json:"smsBody,omitempty"`
	DelaySeconds int                       `json:"delaySeconds,omitempty"`
	Condition    *Condition                `json:"condition,omitempty"`
	SegmentID    string                    `json:"segmentId,omitempty"`
	URL          string                    `json:"url,omitempty"`
}

// A boolean test against a customer - a split node's yes/no branch and the flow-level enrollment filter, Negate inverts it
// Kind picks which fields matter: segment, lifecycle, rfm, email_activity or consent
type Condition struct {
	Kind      string   `json:"kind"`
	Negate    bool     `json:"negate,omitempty"`
	SegmentID string   `json:"segmentId,omitempty"`
	Lifecycle []string `json:"lifecycle,omitempty"`
	// rfm / engagement - 0 ignores that dimension
	MinRecency    int `json:"minRecency,omitempty"`
	MinFrequency  int `json:"minFrequency,omitempty"`
	MinMonetary   int `json:"minMonetary,omitempty"`
	MinEngagement int `json:"minEngagement,omitempty"`
	// email_activity, lifetime - 0 ignores
	MinOpens    int    `json:"minOpens,omitempty"`
	MinClicks   int    `json:"minClicks,omitempty"`
	ConsentKind string `json:"consentKind,omitempty"`
	OptedIn     bool   `json:"optedIn,omitempty"`
}

// Older split condition kind - saved flows still carry it, so it stays accepted
const conditionInSegmentAlias = "in_segment"

// The parsed triggerConfig JSON, carrying the optional enrollment filter - kept here so storage stays a plain JSON scalar with no migration
type TriggerConfig struct {
	EnrollmentFilter *Condition `json:"enrollmentFilter,omitempty"`
	// Transactional sends bypass the marketing-consent gate and suppression, so the customer always receives them - set on the seeded lifecycle flows, marketing by default
	Transactional bool `json:"transactional,omitempty"`
}

// SourceHandle is 'yes'/'no' out of a split, else ""
type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
}

type FlowRun struct {
	ID            uuid.UUID
	FlowID        uuid.UUID
	CustomerID    uuid.UUID // uuid.Nil for a guest run (keyed by Email instead)
	Email         string    // recipient email; the dedup + send key for guest runs
	CurrentNodeID string
	Status        string
	ResumeAt      *time.Time
	Context       json.RawMessage
	Error         string
	EnrollmentKey string // the moment this run was started for, as core named it - unique per flow, empty when unkeyed
	SendAttempts  int    // failed deliveries at the current send node, reset once one goes through
}

// ErrMomentTaken is CreateRun refusing a keyed moment that already has its run in this flow
var ErrMomentTaken = errors.New("flow: this moment already started its run")

var ErrAlreadyInFlow = errors.New("flow: already in this flow")

var (
	ErrNotFound = errors.New("flow not found")
	ErrInvalid  = errors.New("invalid flow")
)

func (r FlowRun) IsGuest() bool { return r.CustomerID == uuid.Nil }

// A flow enrollment enriched with the flow's name and trigger, for listing every flow a customer has been through
type CustomerRun struct {
	RunID     uuid.UUID
	FlowID    uuid.UUID
	FlowName  string
	Trigger   string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (g Graph) node(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func (g Graph) triggerNode() (Node, bool) {
	for _, n := range g.Nodes {
		if n.Type == NodeTrigger {
			return n, true
		}
	}
	return Node{}, false
}

// The target of the single outgoing edge - the default "out" path, ignoring labeled split handles
func (g Graph) next(nodeID string) (string, bool) {
	for _, e := range g.Edges {
		if e.Source == nodeID && (e.SourceHandle == "" || e.SourceHandle == "out") {
			return e.Target, true
		}
	}
	return "", false
}

func (g Graph) branch(nodeID, handle string) (string, bool) {
	for _, e := range g.Edges {
		if e.Source == nodeID && e.SourceHandle == handle {
			return e.Target, true
		}
	}
	return "", false
}

func validTrigger(t string) bool { return slices.Contains(signalsapi.FlowTriggerTypes(), t) }
