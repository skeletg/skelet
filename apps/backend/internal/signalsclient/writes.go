package signalsclient

// Core's writes into the event spine - every one best-effort, since the suite being off or unreachable
// must never fail the commerce action that raised it

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

// A server-side commerce or account action, for the profile timeline
type ActivitySpec struct {
	EventName  string
	Source     string
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	// For guest commerce - attributed to the anon visitor when there is no customer
	AnonID string
	At     time.Time
	Props  map[string]any
	// Keeps the write idempotent against an at-least-once bus
	DedupeKey string
}

type ConversionSpec struct {
	CustomerID   uuid.UUID
	ChannelID    uuid.UUID
	OrderID      uuid.UUID
	RevenueCents int64
	Currency     string
	At           time.Time
}

// The send an engagement belongs to, reduced to what the spine stores
type SendRef struct {
	ID         uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	Source     string
}

// One pixel load or link follow, classified before it is sent
type Hit struct {
	UA      string
	Machine bool
	Device  string
	Client  string
	At      time.Time
}

func (c *Client) RecordActivity(ctx context.Context, a ActivitySpec) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordActivity(sys, &signalsv1.RecordActivityRequest{
		EventName: a.EventName, Source: a.Source, ChannelId: idString(a.ChannelID),
		CustomerId: optionalID(a.CustomerID), AnonId: a.AnonID, At: stamp(a.At),
		PropsJson: propsJSON(a.Props), DedupeKey: a.DedupeKey,
	})
	return err
}

func (c *Client) RecordConversion(ctx context.Context, cs ConversionSpec) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordConversion(sys, &signalsv1.RecordConversionRequest{
		CustomerId: cs.CustomerID.String(), ChannelId: idString(cs.ChannelID), OrderId: cs.OrderID.String(),
		RevenueCents: cs.RevenueCents, Currency: cs.Currency, At: stamp(cs.At),
	})
	return err
}

func (c *Client) RecordEmailSent(ctx context.Context, s SendRef) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordEmailSent(sys, &signalsv1.RecordEmailSentRequest{Send: sendProto(s)})
	return err
}

func (c *Client) RecordEmailOpen(ctx context.Context, s SendRef, h Hit) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordEmailOpen(sys, &signalsv1.RecordEmailOpenRequest{Send: sendProto(s), Hit: hitProto(h)})
	return err
}

func (c *Client) RecordEmailClick(ctx context.Context, s SendRef, url string, linkIndex int, h Hit) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordEmailClick(sys, &signalsv1.RecordEmailClickRequest{
		Send: sendProto(s), Hit: hitProto(h), Url: url, LinkIndex: int32(linkIndex),
	})
	return err
}

func sendProto(s SendRef) *signalsv1.EmailSendRef {
	return &signalsv1.EmailSendRef{
		SendId: s.ID.String(), ChannelId: idString(s.ChannelID),
		CustomerId: optionalID(s.CustomerID), Source: s.Source,
	}
}

func hitProto(h Hit) *signalsv1.EmailHit {
	return &signalsv1.EmailHit{
		UserAgent: h.UA, Machine: h.Machine, Device: h.Device, Client: h.Client, At: stamp(h.At),
	}
}

// The nil uuid rides as an empty string, which the server reads back as absent
func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func optionalID(id *uuid.UUID) *string {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	s := id.String()
	return &s
}

func stamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Props that will not marshal are dropped rather than failing the write, the event itself being worth more than its detail
func propsJSON(p map[string]any) string {
	if len(p) == 0 {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}
