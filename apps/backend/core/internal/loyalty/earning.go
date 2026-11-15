package loyalty

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const EventEarning = "loyalty.earning"

type EarningLine struct {
	SKU        string
	Quantity   float64
	TotalCents int64
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

func (e *Earning) SetEarningPoints(points int64) error {
	if points < 0 {
		return errors.New("loyalty points can't be negative - 0 earns nothing")
	}
	e.Points = points
	return nil
}

func (e *Earning) AddEarningNote(note string) {
	if note = strings.TrimSpace(note); note != "" {
		e.Notes = append(e.Notes, note)
	}
}
