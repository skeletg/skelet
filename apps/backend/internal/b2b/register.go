package b2b

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrInvoiceNotOpen, "B2B_INVOICE_NOT_OPEN", apperr.ClassConflict)
}
