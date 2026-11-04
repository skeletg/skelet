// Package board holds Boards: a shop's shared dashboards of saved reports. A board stores each report's definition and
// the dashboard runs it through the same Synapse queries the report pages use, so there is one engine path per report
package board

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefinitionVersion is the shape Definition has today - stored beside each report so a later shape can still read it
const DefinitionVersion = 1

const (
	maxName       = 120
	maxReports    = 24
	maxDefinition = 8 << 10
)

var (
	ErrNotFound = errors.New("board not found")
	ErrStale    = errors.New("the board was saved by someone else since you opened it - reload it and make your change again")
	ErrInvalid  = errors.New("invalid board")
)

type Board struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	Name        string
	UpdatedAt   time.Time
	Reports     []Report
	ReportCount int // on a list read, where Reports is empty
}

type Report struct {
	Title      string
	Definition Definition
}

// Definition is one saved report - which engine and its settings, the same ones the report pages take
type Definition struct {
	Kind        string   `json:"kind"`                  // insights | funnel | retention | stickiness | flow
	SinceDays   int      `json:"sinceDays,omitempty"`   // 0 = the board's period
	Event       string   `json:"event,omitempty"`       // insights, stickiness, the flow's focus
	Measure     string   `json:"measure,omitempty"`     // insights: total | unique
	Unit        string   `json:"unit,omitempty"`        // insights and retention: day | week
	Breakdown   string   `json:"breakdown,omitempty"`   // insights, funnel
	Steps       []string `json:"steps,omitempty"`       // funnel
	Filters     []Filter `json:"filters,omitempty"`     // funnel
	WindowHours int      `json:"windowHours,omitempty"` // funnel
	StartEvent  string   `json:"startEvent,omitempty"`  // retention
	ReturnEvent string   `json:"returnEvent,omitempty"` // retention
	Periods     int      `json:"periods,omitempty"`     // retention
	Depth       int      `json:"depth,omitempty"`       // flow
}

type Filter struct {
	Step  int    `json:"step"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParseDefinition reads a definition the dashboard sent, refusing fields it doesn't know so a typo can't save a
// report that silently runs differently
func ParseDefinition(raw []byte) (Definition, error) {
	if len(raw) > maxDefinition {
		return Definition{}, fmt.Errorf("%w: a report definition is over %d bytes", ErrInvalid, maxDefinition)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var d Definition
	if err := dec.Decode(&d); err != nil {
		return Definition{}, fmt.Errorf("%w: report definition: %w", ErrInvalid, err)
	}
	return d, d.validate()
}

func (d Definition) validate() error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s report: %s", ErrInvalid, d.Kind, msg) }
	if d.SinceDays < 0 || d.SinceDays > 365 {
		return bad("the period is 1 to 365 days")
	}
	switch d.Kind {
	case "insights":
		if d.Measure != "" && d.Measure != "total" && d.Measure != "unique" {
			return bad("measure is total or unique")
		}
	case "stickiness":
	case "funnel":
		if len(d.Steps) < 2 || len(d.Steps) > 8 {
			return bad("a funnel has 2 to 8 steps")
		}
		for _, f := range d.Filters {
			if f.Step < 0 || f.Step >= len(d.Steps) || strings.TrimSpace(f.Key) == "" {
				return bad("a filter needs a property and one of the funnel's steps")
			}
		}
		if d.WindowHours < 0 {
			return bad("the conversion window can't be negative")
		}
	case "retention":
		if d.StartEvent == "" || d.ReturnEvent == "" {
			return bad("retention needs a start and a return event")
		}
	case "flow":
		if d.Event == "" {
			return bad("a flow needs its focus event")
		}
	default:
		return fmt.Errorf("%w: unknown report kind %q", ErrInvalid, d.Kind)
	}
	if d.Unit != "" && d.Unit != "day" && d.Unit != "week" {
		return bad("unit is day or week")
	}
	return nil
}

func (b Board) validate() error {
	name := strings.TrimSpace(b.Name)
	if name == "" || len(name) > maxName {
		return fmt.Errorf("%w: a board needs a name of up to %d characters", ErrInvalid, maxName)
	}
	if len(b.Reports) > maxReports {
		return fmt.Errorf("%w: a board holds up to %d reports", ErrInvalid, maxReports)
	}
	for _, r := range b.Reports {
		if t := strings.TrimSpace(r.Title); t == "" || len(t) > maxName {
			return fmt.Errorf("%w: each report needs a title of up to %d characters", ErrInvalid, maxName)
		}
		if err := r.Definition.validate(); err != nil {
			return err
		}
	}
	return nil
}

// DefinitionJSON is a definition as the wire carries it
func DefinitionJSON(d Definition) (string, error) {
	raw, err := json.Marshal(d)
	return string(raw), err
}
