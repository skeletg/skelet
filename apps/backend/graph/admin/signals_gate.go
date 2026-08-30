package admin

import "github.com/skeletg/skelet/internal/apperr"

var errSignalsDisabled = apperr.New("SIGNALS_DISABLED", apperr.ClassValidation, "Signals features are disabled on this server") // server-wide SIGNALS_ENABLED switch

func (r *Resolver) requireSignals() error { // email templates and the library stay usable with Signals off - transactional mail still sends
	if !r.SignalsEnabled {
		return errSignalsDisabled
	}
	return nil
}
