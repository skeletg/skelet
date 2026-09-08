package admin

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

var errSignalsDisabled = apperr.New("SIGNALS_DISABLED", apperr.ClassValidation, "Signals features are disabled on this server") // server-wide SIGNALS_ENABLED switch

func (r *Resolver) requireSignals() error { // flows and email templates stay usable with Signals off - transactional mail still sends, and flows are what deliver it
	if !r.SignalsEnabled {
		return errSignalsDisabled
	}
	return nil
}
