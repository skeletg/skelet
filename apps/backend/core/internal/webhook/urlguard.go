package webhook

import (
	"net/http"
	"os"
	"strconv"

	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
)

// SSRF guard for outbound webhook deliveries - the implementation lives in internal/urlguard, a leaf package, so subsystems that would otherwise cycle with webhook can share the same guard - These aliases keep webhook's surface and every existing caller unchanged

func ValidateTargetURL(raw string, allowPrivate bool) error {
	return urlguard.ValidateTargetURL(raw, allowPrivate)
}

func SafeTransport(allowPrivate bool) *http.Transport {
	return urlguard.SafeTransport(allowPrivate)
}

// One policy for every process that posts webhooks (cmd/server, cmd/signals) - letting the two disagree means dev split-jobs forwarding silently loses its localhost receivers - explicit ALLOW_PRIVATE_WEBHOOK_TARGETS wins, else private targets are allowed only in development
func AllowPrivateTargetsFromEnv() bool {
	if v := os.Getenv("ALLOW_PRIVATE_WEBHOOK_TARGETS"); v != "" {
		b, err := strconv.ParseBool(v)
		return err == nil && b
	}
	return os.Getenv("APP_ENV") == "development"
}
