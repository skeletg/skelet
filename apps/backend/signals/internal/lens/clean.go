package lens

import (
	"encoding/json"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// What the public tracker may write - every field below lands in a row, a rollup key or the spine
const (
	maxEventID    = 64
	maxTitle      = 300
	maxReferrer   = 512
	maxSelector   = 256
	maxText       = 120
	maxState      = 64
	maxLayoutHash = 64
	maxEventName  = 64
	maxPropsBytes = 4 << 10
	maxUserAgent  = 400

	maxReplayChunks         = 1000
	defaultReplayMaxMinutes = 30
	replayGrace             = 5 * time.Minute
	maxSnapshotLayouts      = 20
)

// The most a recording hands the player - the rest of a longer one stays in storage
var maxReplayReadBytes = 64 << 20

var (
	viewports    = map[string]bool{"desktop": true, "tablet": true, "mobile": true}
	layoutHashRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// clean bounds one tracker event in place - false drops a page-bound event with no usable page key, or a grid event with no viewport
func clean(e *Event) bool {
	e.ID = cut(e.ID, maxEventID)
	e.Title = cut(e.Title, maxTitle)
	e.Referrer = cut(e.Referrer, maxReferrer)
	e.Selector = cut(e.Selector, maxSelector)
	e.Text = cut(e.Text, maxText)
	e.Name = cut(e.Name, maxEventName)
	e.State = cut(e.State, maxState)
	if len(e.LayoutHash) > maxLayoutHash || !layoutHashRe.MatchString(e.LayoutHash) {
		e.LayoutHash = ""
	}
	if len(e.Props) > 0 {
		if raw, err := json.Marshal(e.Props); err != nil || len(raw) > maxPropsBytes {
			e.Props = nil
		}
	}
	if e.Type == analytics.EventIdentify {
		return true
	}
	key, ok := analytics.PageKey(e.PageKey)
	if !ok {
		return false
	}
	e.PageKey = key
	if !viewports[e.Viewport] {
		switch e.Type {
		case "click", "move", "scroll", "snapshot":
			return false
		}
		e.Viewport = ""
	}
	return true
}

// cut caps s at n bytes on a rune boundary - Postgres refuses invalid UTF-8, so a split rune would drop the whole row
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
