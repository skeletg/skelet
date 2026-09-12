package analytics

import (
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/uaclass"
)

// The classifier is a leaf both sides of the Signals boundary use - these keep the suite's callers reading in one vocabulary
type Enrichment = uaclass.Enrichment

func Classify(ua string, sinceSend time.Duration) Enrichment { return uaclass.Classify(ua, sinceSend) }
