package admin

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

var errSignalsDisabled = apperr.New("SIGNALS_DISABLED", apperr.ClassValidation, "Signals features are disabled on this server") // server-wide SIGNALS_ENABLED switch

func (r *Resolver) requireSignals() error { // Pulse is the suite's too, so with the switch off there are no flows, campaigns or saved emails - core delivers every transactional mail from its own templates
	if !r.SignalsEnabled {
		return errSignalsDisabled
	}
	return nil
}
