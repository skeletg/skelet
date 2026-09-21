package front

import (
	"context"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Shared by paymentMethods and Order.paymentInstructions - kept out of a *.resolvers.go so gqlgen regeneration doesn't drop the config import The scope is the caller's: offline instructions are per channel (bank details and pickup wording differ per market), and reading them at the default scope showed every channel the same text
func (r *Resolver) paymentInstructions(ctx context.Context, slug string, scope config.Scope) string {
	if r.ConfigSvc == nil || strings.TrimSpace(slug) == "" {
		return "" // empty is normal - gateway methods carry no instructions
	}
	v, _ := r.ConfigSvc.GetString(ctx, "payment/"+slug+"/instructions", scope)
	return strings.TrimSpace(v)
}
