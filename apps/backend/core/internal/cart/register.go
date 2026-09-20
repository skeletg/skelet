package cart

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() { // registers cart sentinels so the front presenter forwards their messages verbatim instead of scrubbing them to "internal error"
	apperr.RegisterSentinel(ErrCartAlreadyConverted, "CART_ALREADY_CONVERTED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCartForbidden, "CART_FORBIDDEN", apperr.ClassForbidden)
	apperr.RegisterSentinel(ErrCartNotActive, "CART_NOT_ACTIVE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCartLineNotFound, "CART_LINE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrCartStoreMismatch, "CART_STORE_MISMATCH", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrProductNotPurchasable, "PRODUCT_NOT_PURCHASABLE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrQuantityOutOfBounds, "QUANTITY_OUT_OF_BOUNDS", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductNotBundle, "PRODUCT_NOT_BUNDLE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrBundleSelectionMissing, "BUNDLE_SELECTION_MISSING", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrBundleSelectionInvalid, "BUNDLE_SELECTION_INVALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrBundleSelectionCardinality, "BUNDLE_SELECTION_CARDINALITY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrBundleOptionUnknown, "BUNDLE_OPTION_UNKNOWN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionUnknown, "PRODUCT_OPTION_UNKNOWN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionMissing, "PRODUCT_OPTION_MISSING", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionInvalid, "PRODUCT_OPTION_INVALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionCardinality, "PRODUCT_OPTION_CARDINALITY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionHidden, "PRODUCT_OPTION_HIDDEN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionQtyConflict, "PRODUCT_OPTION_QTY_CONFLICT", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionQtyForced, "PRODUCT_OPTION_QTY_FORCED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrProductOptionsRequired, "PRODUCT_OPTIONS_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrLineConfigured, "CART_LINE_CONFIGURED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrOptionsUnsupportedKind, "PRODUCT_OPTIONS_UNSUPPORTED_KIND", apperr.ClassValidation)
}
