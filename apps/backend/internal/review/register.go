package review

import "github.com/skeletg/skelet/apps/backend/internal/apperr"

// Same contract as the other register files: bare sentinels out of the service classify at the GraphQL edge instead of masking as INTERNAL
func init() {
	apperr.RegisterSentinel(ErrNotFound, "REVIEW_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrQuestionNotFound, "REVIEW_QUESTION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrCategoryNotFound, "REVIEW_CATEGORY_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrTagNotFound, "REVIEW_TAG_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrSlugTaken, "REVIEW_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrAlreadyReviewed, "ALREADY_REVIEWED", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidInputType, "REVIEW_INVALID_INPUT_TYPE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidStatus, "REVIEW_INVALID_STATUS", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrGuestNameRequired, "REVIEW_GUEST_NAME_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrAnswerOutOfRange, "REVIEW_ANSWER_OUT_OF_RANGE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrMissingRequired, "REVIEW_REQUIRED_ANSWER_MISSING", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrUnknownQuestion, "REVIEW_UNKNOWN_QUESTION", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidVote, "REVIEW_INVALID_VOTE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidVoter, "REVIEW_INVALID_VOTER", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidVoterToken, "REVIEW_INVALID_VOTER_TOKEN", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrFieldTooLong, "REVIEW_FIELD_TOO_LONG", apperr.ClassValidation)

	// policy denials, not validation - the storefront branches on these to prompt sign-in or hide the form
	apperr.RegisterSentinel(ErrNotPurchased, "REVIEW_NOT_PURCHASED", apperr.ClassForbidden)
	apperr.RegisterSentinel(ErrGuestNotAllowed, "REVIEW_GUEST_NOT_ALLOWED", apperr.ClassForbidden)
	apperr.RegisterSentinel(ErrVoteAuthRequired, "REVIEW_VOTE_AUTH_REQUIRED", apperr.ClassAuth)
}
