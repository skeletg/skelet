// Package board holds Boards: dashboards of saved reports, owned by an admin and private until shared, or shop boards
// everyone on the shop sees. A board stores each report's definition and
// the dashboard runs it through the same Synapse queries the report pages use, so there is one engine path per report
package board

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxName    = 120
	maxReports = 24
)

var (
	ErrNotFound = errors.New("board not found")
	ErrStale    = errors.New("the board was saved by someone else since you opened it - reload it and make your change again")
	ErrInvalid  = errors.New("invalid board")
)

type Board struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Name      string
	UpdatedAt time.Time
	Reports   []Report
	Ownership
	Shares []Share // for an admin who owns it
	Draft  *Draft  // the asking admin's unsaved edits
}

// Summary is one board in the list, without its reports
type Summary struct {
	ID          uuid.UUID
	Name        string
	UpdatedAt   time.Time
	ReportCount int
	Ownership
}

// Ownership is who a board belongs to, and what that means for the admin asking - a list and a single read both carry it
type Ownership struct {
	OwnerID uuid.UUID // uuid.Nil: a shop board
	Status  string    // StatusDraft until its first Save
	// For the admin asking: what they may do, whether they starred it, and when their unsaved edits were last kept
	Access    Access
	Favourite bool
	DraftAt   time.Time
}

type Report struct {
	Title      string
	Size       string // the tile's width on the board: small | medium (unset) | full
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
	WindowDays  int      `json:"windowDays,omitempty"`  // stickiness: the return window, 0 = the shop's default
}

type Filter struct {
	Step  int    `json:"step"`
	Key   string `json:"key"`
	Value string `json:"value"`
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
		if d.WindowDays < 0 || d.WindowDays > 365 {
			return bad("the return window is 1 to 365 days")
		}
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

// normalized is the board as it's stored - name and titles trimmed - or why it can't be
func (b Board) normalized() (Board, error) {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || utf8.RuneCountInString(b.Name) > maxName {
		return Board{}, fmt.Errorf("%w: a board needs a name of up to %d characters", ErrInvalid, maxName)
	}
	if len(b.Reports) > maxReports {
		return Board{}, fmt.Errorf("%w: a board holds up to %d reports", ErrInvalid, maxReports)
	}
	reports := make([]Report, len(b.Reports)) // a copy, so the caller's slice keeps its own titles
	for i, r := range b.Reports {
		r.Title = strings.TrimSpace(r.Title)
		if r.Title == "" || utf8.RuneCountInString(r.Title) > maxName {
			return Board{}, fmt.Errorf("%w: each report needs a title of up to %d characters", ErrInvalid, maxName)
		}
		if r.Size != "" && r.Size != "small" && r.Size != "medium" && r.Size != "full" {
			return Board{}, fmt.Errorf("%w: a report's size is small, medium or full", ErrInvalid)
		}
		if err := r.Definition.validate(); err != nil {
			return Board{}, err
		}
		reports[i] = r
	}
	b.Reports = reports
	return b, nil
}
