// Retention archiving: instead of deleting aged rows, the retention sweep can export them to object storage first (retention_action=archive) - identity-stripped gzip JSONL, one file per aged day (spine) or month (send log)
// Stripping customer_id / anon_id / dedupe_key / email at export time is what keeps GDPR trivial: archives hold anonymous event streams, so an erasure request never has to rewrite a cold object
package analytics

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
)

// Aliased rather than re-exported, so spinech and signalsboot keep their existing spelling
type (
	ArchiveSink = archivefile.Sink
	ArchiveFile = archivefile.File
	ArchiveOpen = archivefile.Open
)

var ErrNoArchiveSink = archivefile.ErrNoSink

// ArchivedEvent is one spine event as archived - no customer_id, anon_id or dedupe_key on purpose (identify's dedupe key embeds both ids)
type ArchivedEvent struct {
	ID         uuid.UUID       `json:"id"`
	EventName  string          `json:"event_name"`
	OccurredAt time.Time       `json:"occurred_at"`
	ReceivedAt time.Time       `json:"received_at"`
	ChannelID  *uuid.UUID      `json:"channel_id,omitempty"`
	Source     string          `json:"source"`
	SendID     *uuid.UUID      `json:"send_id,omitempty"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

// SetArchiveSink wires the retention archive destination - unset leaves archive mode failing closed
func (r *Recorder) SetArchiveSink(s ArchiveSink) { r.archive = s }

func (r *Recorder) openArchive(ctx context.Context, dir string) ArchiveOpen {
	return func(day time.Time, first uuid.UUID) (*ArchiveFile, error) {
		key := dir + "/" + day.UTC().Format("2006-01-02") + "-" + first.String()[:8] + ".jsonl.gz"
		return archivefile.New(ctx, r.archive, key), nil
	}
}

// ArchivePrune is Prune's archive twin: aged event days are exported to the sink before their rows delete, while identity mappings still just die - they're pure PII linkage, exactly what archives strip
func (r *Recorder) ArchivePrune(ctx context.Context, before time.Time) (int64, error) {
	if r.archive == nil {
		return 0, ErrNoArchiveSink
	}
	if n, err := r.spine.PruneIdentities(ctx, before); err != nil {
		log.Printf("[analytics] prune identities: %v", err)
	} else if n > 0 {
		log.Printf("[analytics] pruned %d stale identity mappings", n)
	}
	return r.spine.ArchiveEvents(ctx, "", before, r.openArchive(ctx, "spine"))
}

// ArchiveEventsBySource is PruneEventsBySource's archive twin, for the shorter Lens window
func (r *Recorder) ArchiveEventsBySource(ctx context.Context, source string, before time.Time) (int64, error) {
	if r.archive == nil {
		return 0, ErrNoArchiveSink
	}
	return r.spine.ArchiveEvents(ctx, source, before, r.openArchive(ctx, "spine/"+source))
}
