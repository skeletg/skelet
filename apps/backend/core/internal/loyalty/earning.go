package loyalty

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	EventEarning = "loyalty.earning"
	// The most one order earns, whatever a plugin says - far past any real promotion, short of a balance that overflows
	MaxEarningPoints = 100_000_000
	maxEarningNote   = 200
	maxEarningNotes  = 5
)

type EarningLine struct {
	SKU         string
	ProductID   uuid.UUID // uuid.Nil for a line with no catalog product, like an admin's free-form charge
	CategoryIDs []uuid.UUID
	Quantity    float64
	UnitCents   int64
	TotalCents  int64
	Gift        bool
}

// Fires BEFORE an order's points are written - sync handlers change them - Response {points, note}, and reject earns nothing
type Earning struct {
	OrderID          uuid.UUID
	ChannelID        uuid.UUID
	CustomerID       uuid.UUID
	CurrencyCode     string
	UnitCents        int64
	PointsPerUnit    int64
	MerchandiseCents int64
	Lines            []EarningLine
	Points           int64
	Notes            []string
}

func (*Earning) Type() string                { return EventEarning }
func (e *Earning) EventChannelID() uuid.UUID { return e.ChannelID }

func (e *Earning) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool   `json:"reject"`
		Reason string `json:"reason"`
		Points *int64 `json:"points"`
		Note   string `json:"note"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	switch {
	case resp.Reject:
		e.Points = 0
		e.AddEarningNote(resp.Reason)
		return false, "", nil
	case resp.Points != nil:
		if err := e.SetEarningPoints(*resp.Points); err != nil {
			return false, "", err
		}
	}
	e.AddEarningNote(resp.Note)
	return false, "", nil
}

func (e *Earning) EarningOrderID() uuid.UUID      { return e.OrderID }
func (e *Earning) EarningChannelID() uuid.UUID    { return e.ChannelID }
func (e *Earning) EarningCustomerID() uuid.UUID   { return e.CustomerID }
func (e *Earning) EarningCurrencyCode() string    { return e.CurrencyCode }
func (e *Earning) EarningUnitCents() int64        { return e.UnitCents }
func (e *Earning) EarningPointsPerUnit() int64    { return e.PointsPerUnit }
func (e *Earning) EarningMerchandiseCents() int64 { return e.MerchandiseCents }
func (e *Earning) EarningLineCount() int          { return len(e.Lines) }
func (e *Earning) EarningPoints() int64           { return e.Points }

func (e *Earning) EarningLine(i int) (sku string, quantity float64, totalCents int64) {
	if i < 0 || i >= len(e.Lines) {
		return "", 0, 0
	}
	l := e.Lines[i]
	return l.SKU, l.Quantity, l.TotalCents
}

func (e *Earning) line(i int) EarningLine {
	if i < 0 || i >= len(e.Lines) {
		return EarningLine{}
	}
	return e.Lines[i]
}

func (e *Earning) EarningLineProduct(i int) (uuid.UUID, []uuid.UUID) {
	l := e.line(i)
	return l.ProductID, slices.Clone(l.CategoryIDs)
}

func (e *Earning) EarningLineUnitCents(i int) int64 { return e.line(i).UnitCents }
func (e *Earning) EarningLineGift(i int) bool       { return e.line(i).Gift }

func (e *Earning) SetEarningPoints(points int64) error {
	switch {
	case points < 0:
		return errors.New("loyalty points can't be negative - 0 earns nothing")
	case points > MaxEarningPoints:
		return fmt.Errorf("one order earns at most %d points", MaxEarningPoints)
	}
	e.Points = points
	return nil
}

// Notes past the fifth are dropped, and each is cut to 200 characters - they're shown in a points history row
func (e *Earning) AddEarningNote(note string) {
	if note = strings.TrimSpace(note); note != "" && len(e.Notes) < maxEarningNotes {
		if r := []rune(note); len(r) > maxEarningNote {
			note = string(r[:maxEarningNote])
		}
		e.Notes = append(e.Notes, note)
	}
}
