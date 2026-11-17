package front

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// A refusal reaches the shopper in its own words; a fault never does
func TestQuoteProblemWording(t *testing.T) {
	refusal := apperr.New("ACME_COD_LIMIT", apperr.ClassValidation, "cash on delivery takes orders up to 100.00")
	if got := *quoteProblem(uuid.New(), refusal); got != "cash on delivery takes orders up to 100.00" {
		t.Errorf("a refusal read %q", got)
	}
	for name, fault := range map[string]error{
		"a raw error":   errors.New("pq: connection refused"),
		"internal":      apperr.New("X", apperr.ClassInternal, "compute tax: rule 7 is malformed"),
		"a plugin down": apperr.New("PLUGIN_TIMEOUT", apperr.ClassUnavailable, "plugin acme: fee cod timed out"),
	} {
		if got := *quoteProblem(uuid.New(), fault); got != "This order can't be priced right now - try again in a moment." {
			t.Errorf("%s leaked as %q", name, got)
		}
	}
}
