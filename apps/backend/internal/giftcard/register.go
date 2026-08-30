package giftcard

import "github.com/skeletg/skelet/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotFound, "GIFT_CARD_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrInactive, "GIFT_CARD_INACTIVE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrExpired, "GIFT_CARD_EXPIRED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrInsufficient, "GIFT_CARD_INSUFFICIENT", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCodeTaken, "GIFT_CARD_CODE_TAKEN", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidAmount, "GIFT_CARD_INVALID_AMOUNT", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCurrencyMismatch, "GIFT_CARD_CURRENCY_MISMATCH", apperr.ClassValidation)
}
