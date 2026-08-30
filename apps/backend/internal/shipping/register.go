package shipping

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrZoneNotFound, "SHIPPING_ZONE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrMethodNotFound, "SHIPPING_METHOD_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrCarrierNotFound, "SHIPPING_CARRIER_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrCodeTaken, "SHIPPING_CODE_TAKEN", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrTrackingUnsupported, "SHIPPING_TRACKING_UNSUPPORTED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrLabelsUnsupported, "SHIPPING_LABELS_UNSUPPORTED", apperr.ClassValidation)
}
