package actions

import "github.com/skeletg/skelet/apps/backend/internal/promotion"

// Each file adds itself here from init(), so dropping a new type in the package still picks it up - what changed
// is that nothing lands in a registry until Register runs, which is why no entrypoint has to blank-import this
var builtins []promotion.ActionApplier

func add(t promotion.ActionApplier) { builtins = append(builtins, t) }

// Idempotent because the registry panics on a duplicate slug, and both a fresh registry in a test and the default
// one at boot can be handed here more than once
func Register(r *promotion.Registry) {
	for _, t := range builtins {
		if _, exists := r.LookupAction(t.Slug()); !exists {
			r.RegisterAction(t)
		}
	}
}
