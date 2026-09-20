package plugin

import (
	"sort"
	"sync"
	"time"
)

// AsyncFailure is one plugin's error tally on one event type since the last drain
type AsyncFailure struct {
	PluginSlug string
	EventType  string
	Count      int64
	LastError  string
	LastAt     time.Time
}

// Async handler errors are logged and dropped by design - the tally is the operator-visible half, drained by the
// notification scan, because a silently failing ERP sync is data loss that log lines alone never surface
var asyncFailures = struct {
	sync.Mutex
	m map[[2]string]*AsyncFailure
}{m: map[[2]string]*AsyncFailure{}}

func recordAsyncFailure(slug, eventType string, err error) {
	if slug == "" {
		return // unattributed core-side handlers have their own reporting
	}
	asyncFailures.Lock()
	defer asyncFailures.Unlock()
	key := [2]string{slug, eventType}
	f := asyncFailures.m[key]
	if f == nil {
		f = &AsyncFailure{PluginSlug: slug, EventType: eventType}
		asyncFailures.m[key] = f
	}
	f.Count++
	f.LastError = err.Error()
	f.LastAt = time.Now()
}

// TakeAsyncFailures drains the tally - one entry per (plugin, event type), sorted for stable output
func TakeAsyncFailures() []AsyncFailure {
	asyncFailures.Lock()
	defer asyncFailures.Unlock()
	if len(asyncFailures.m) == 0 {
		return nil
	}
	out := make([]AsyncFailure, 0, len(asyncFailures.m))
	for _, f := range asyncFailures.m {
		out = append(out, *f)
	}
	asyncFailures.m = map[[2]string]*AsyncFailure{}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PluginSlug != out[j].PluginSlug {
			return out[i].PluginSlug < out[j].PluginSlug
		}
		return out[i].EventType < out[j].EventType
	})
	return out
}
