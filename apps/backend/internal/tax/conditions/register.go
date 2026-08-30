package conditions

import "github.com/skeletg/skelet/internal/tax"

// Each file adds itself here from init(), so dropping a new type in the package still picks it up - what changed
// is that nothing lands in a registry until Register runs, which is why no entrypoint has to blank-import this
var builtins []tax.ConditionEvaluator

func add(t tax.ConditionEvaluator) { builtins = append(builtins, t) }

// Idempotent because the registry panics on a duplicate slug, and both a fresh registry in a test and the default
// one at boot can be handed here more than once
func Register(r *tax.Registry) {
	for _, t := range builtins {
		if _, exists := r.LookupCondition(t.Slug()); !exists {
			r.RegisterCondition(t)
		}
	}
}
