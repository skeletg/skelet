package behavioralsegment

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

func init() {
	apperr.RegisterSentinel(ErrNotBehavioral, "SEGMENT_NOT_BEHAVIORAL", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrEmptyDefinition, "SEGMENT_DEFINITION_EMPTY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidChannel, "SEGMENT_CHANNEL_INVALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrUnknownKind, "SEGMENT_NODE_KIND_UNKNOWN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrEmptyGroup, "SEGMENT_GROUP_EMPTY", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrNameRequired, "SEGMENT_NAME_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrOtherChannel, "SEGMENT_WRONG_CHANNEL", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrLifecycleWindows, "SEGMENT_LIFECYCLE_WINDOWS_INVALID", apperr.ClassValidation)
}
