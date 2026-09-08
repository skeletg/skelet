package segment

import (
	"encoding/json"
	"strings"
)

// Which segments stop existing when SIGNALS_ENABLED is off - their membership is materialised, so with the suite off it goes stale rather than wrong, and a frozen membership still gates promotions and CMS blocks

// Condition kinds backed by customer_traits, the table the Synapse trait-compute job owns
var synapseRuleKinds = map[string]bool{"trait": true, "lifecycle": true}

// NeedsSignals reports whether any condition can only be answered by a table Signals maintains
func (r *Rules) NeedsSignals() bool {
	if r == nil {
		return false
	}
	for _, c := range r.Conditions {
		if synapseRuleKinds[c.Kind] {
			return true
		}
	}
	return false
}

// SignalsMaintained reports whether Signals owns this segment's membership, taking raw columns so a membership read can filter rows without loading whole segments
func SignalsMaintained(rules json.RawMessage, refreshSQL string) bool {
	if synapseDefined(refreshSQL) {
		return true
	}
	// Parse errors mean the rules never compiled, so the segment has no computed membership to distrust
	parsed, err := ParseRules(rules)
	if err != nil {
		return false
	}
	return parsed.NeedsSignals()
}

// SignalsMaintained reports whether Signals owns this segment's membership
func (s Segment) SignalsMaintained() bool { return SignalsMaintained(s.Rules, s.RefreshSQL) }

// A Synapse-defined segment always carries a channelId - anything else in the column is the reserved placeholder
func synapseDefined(refreshSQL string) bool {
	if strings.TrimSpace(refreshSQL) == "" {
		return false
	}
	var def struct {
		ChannelID string `json:"channelId"`
	}
	return json.Unmarshal([]byte(refreshSQL), &def) == nil && def.ChannelID != ""
}
