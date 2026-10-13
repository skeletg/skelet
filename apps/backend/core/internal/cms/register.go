package cms

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

// Pairs each exported sentinel with a stable code + class so resolvers can return them bare and the presenter classifies instead of masking them as INTERNAL - new sentinels get added here (the registry panics at boot on a duplicate code with a different class)
func init() {
	apperr.RegisterSentinel(ErrAttributeNotFound, "CMS_ATTRIBUTE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrAttributeValueNotFound, "CMS_ATTRIBUTE_VALUE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrAttributeGroupNotFound, "CMS_ATTRIBUTE_GROUP_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrPageTypeNotFound, "CMS_PAGE_TYPE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrPageNotFound, "CMS_PAGE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrPageContentNotFound, "CMS_PAGE_CONTENT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrSnippetNotFound, "CMS_SNIPPET_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrBlockTypeNotFound, "CMS_BLOCK_TYPE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrRevisionNotFound, "CMS_REVISION_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrCommentNotFound, "CMS_COMMENT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrChannelNotFound, "CMS_CHANNEL_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrTagNotFound, "CMS_TAG_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrSlugTaken, "CMS_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrAttributeInUse, "CMS_ATTRIBUTE_IN_USE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrPageTypeHasPages, "CMS_PAGE_TYPE_HAS_PAGES", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrPageTypeIsSystem, "CMS_PAGE_TYPE_IS_SYSTEM", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrSingletonExists, "CMS_SINGLETON_EXISTS", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrApprovalRequired, "CMS_APPROVAL_REQUIRED", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrURLPathConflict, "CMS_URL_PATH_CONFLICT", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrPageHasChildren, "CMS_PAGE_HAS_CHILDREN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrNotInReview, "CMS_PAGE_NOT_IN_REVIEW", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrAttributeValueInUse, "CMS_ATTRIBUTE_VALUE_IN_USE", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrSelfApproval, "CMS_SELF_APPROVAL", apperr.ClassForbidden)
	apperr.RegisterSentinel(ErrAdminRequired, "CMS_ADMIN_REQUIRED", apperr.ClassAuth)

	apperr.RegisterSentinel(ErrInvalidSlug, "CMS_INVALID_SLUG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidInputType, "CMS_INVALID_INPUT_TYPE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidRelationship, "CMS_INVALID_RELATIONSHIP", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrValuesNotApplicable, "CMS_VALUES_NOT_APPLICABLE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrRequiredFieldMissing, "CMS_REQUIRED_FIELD_MISSING", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidFieldValue, "CMS_INVALID_FIELD_VALUE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPageCycle, "CMS_PAGE_CYCLE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPageTooDeep, "CMS_PAGE_TOO_DEEP", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrReservedBlockKey, "CMS_RESERVED_BLOCK_KEY", apperr.ClassValidation)
}
