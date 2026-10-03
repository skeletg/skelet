package ai

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotConfigured, "AI_NOT_CONFIGURED", apperr.ClassUnavailable)
	apperr.RegisterSentinel(ErrDisabled, "AI_DISABLED", apperr.ClassUnavailable)
	apperr.RegisterSentinel(ErrTruncated, "AI_REPLY_TRUNCATED", apperr.ClassUnavailable)
	apperr.RegisterSentinel(ErrRefused, "AI_REFUSED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrConversationNotFound, "CHAT_CONVERSATION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrMessageTooLong, "CHAT_MESSAGE_TOO_LONG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidRole, "CHAT_INVALID_ROLE", apperr.ClassValidation)
}
