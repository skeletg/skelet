package sendlog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
)

// Archive mode with no destination must never quietly fall back to deleting
func TestArchiveModeFailsClosedWithoutSink(t *testing.T) {
	s := NewStore(nil)
	if _, err := s.Prune(context.Background(), time.Now(), true); !errors.Is(err, archivefile.ErrNoSink) {
		t.Fatalf("Prune(archive) err = %v, want ErrNoSink", err)
	}
}

// The three sources the log distinguishes - a send recorded under the wrong one reports against the wrong sender
func TestSourcesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, src := range []string{SourceCampaign, SourceFlow, SourceTransactional} {
		if src == "" {
			t.Fatal("a send source is empty")
		}
		if seen[src] {
			t.Errorf("duplicate send source %q", src)
		}
		seen[src] = true
	}
}
