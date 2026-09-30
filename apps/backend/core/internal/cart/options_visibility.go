package cart

import (
	"strings"

	"github.com/google/uuid"
)

// A VisibleIf group only references EARLIER options (authoring validation enforces the lower sort_order), so evaluation is one ordered pass with no cycles, each condition seeing only already-visible options' picks - hidden options contribute nothing downstream

type OptionVisibilityRule struct {
	OptionID uuid.UUID
	Op       string // anyOf | noneOf | filled | empty
	ValueIDs []uuid.UUID
}

type OptionVisibilityGroup struct {
	Logic  string // "and" (default) | "or"
	Rules  []OptionVisibilityRule
	Groups []OptionVisibilityGroup
}

type optionPicks struct { // VISIBLE options' picks so far - which carry a value, and which value IDs a select-family option picked
	filled map[uuid.UUID]bool
	values map[uuid.UUID]map[uuid.UUID]bool
}

func visibleOptionIDs(res ProductOptionsResolution, inputs []CartLineOptionInput) map[uuid.UUID]bool { // walks res.Options in canonical order
	byOption := make(map[uuid.UUID]CartLineOptionInput, len(inputs))
	for _, in := range inputs {
		if _, dup := byOption[in.OptionID]; !dup {
			byOption[in.OptionID] = in
		}
	}
	picks := optionPicks{
		filled: make(map[uuid.UUID]bool, len(res.Options)),
		values: make(map[uuid.UUID]map[uuid.UUID]bool, len(res.Options)),
	}
	visible := make(map[uuid.UUID]bool, len(res.Options))
	for _, opt := range res.Options {
		if opt.VisibleIf != nil && !evalVisibilityGroup(*opt.VisibleIf, picks) {
			continue
		}
		visible[opt.ID] = true
		in, ok := byOption[opt.ID]
		if !ok {
			continue
		}
		if isSelectInput(opt.InputType) {
			if len(in.ValueIDs) == 0 {
				continue
			}
			set := make(map[uuid.UUID]bool, len(in.ValueIDs))
			for _, vid := range in.ValueIDs {
				set[vid] = true
			}
			picks.values[opt.ID] = set
			picks.filled[opt.ID] = true
			continue
		}
		picks.filled[opt.ID] = pickCarriesValue(in)
	}
	return visible
}

func pickCarriesValue(in CartLineOptionInput) bool { // drives "filled", and decides whether a pick on a hidden option is real and rejectable or an empty husk
	return len(in.ValueIDs) > 0 ||
		strings.TrimSpace(in.TextValue) != "" ||
		in.FileAssetID != nil
}

func evalVisibilityGroup(g OptionVisibilityGroup, picks optionPicks) bool { // empty logic means "and"
	or := g.Logic == "or"
	for _, r := range g.Rules {
		ok := evalVisibilityRule(r, picks)
		if or && ok {
			return true
		}
		if !or && !ok {
			return false
		}
	}
	for _, sub := range g.Groups {
		ok := evalVisibilityGroup(sub, picks)
		if or && ok {
			return true
		}
		if !or && !ok {
			return false
		}
	}
	return !or // "and" over nothing - validation forbids empty groups, but one that slips through is always-true rather than hidden forever
}

func evalVisibilityRule(r OptionVisibilityRule, picks optionPicks) bool { // a hidden or unfilled parent has no picks, so filled and anyOf are false while empty and noneOf are true
	switch r.Op {
	case "filled":
		return picks.filled[r.OptionID]
	case "empty":
		return !picks.filled[r.OptionID]
	case "anyOf", "noneOf":
		picked := picks.values[r.OptionID]
		hit := false
		for _, vid := range r.ValueIDs {
			if picked[vid] {
				hit = true
				break
			}
		}
		if r.Op == "anyOf" {
			return hit
		}
		return !hit
	default:
		return false // an unknown op shouldn't survive validation - fail closed and keep the option hidden rather than silently pricing it in
	}
}

// Maps a persisted line's snapshot onto the CURRENT config for the forced line quantity its picks imply, nil when none - keeps a "Set of 4"-style line locked on quantity edits, and first-wins is safe because add rejects conflicting forcers
func forcedQtyForSnapshot(res ProductOptionsResolution, snapshot []CartLineOption) *float64 {
	forcedByValue := make(map[uuid.UUID]*float64)
	for _, opt := range res.Options {
		for _, v := range opt.Values {
			if v.ForcedLineQty != nil {
				forcedByValue[v.ID] = v.ForcedLineQty
			}
		}
	}
	for _, row := range snapshot {
		if row.OptionValueID == nil {
			continue
		}
		if q := forcedByValue[*row.OptionValueID]; q != nil {
			out := *q
			return &out
		}
	}
	return nil
}
