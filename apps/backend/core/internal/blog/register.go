package blog

import "github.com/skeletg/skelet/apps/backend/core/lib/apperr"

func init() {
	apperr.RegisterSentinel(ErrAuthorNotFound, "BLOG_AUTHOR_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrAuthorContentNotFound, "BLOG_AUTHOR_CONTENT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrCategoryNotFound, "BLOG_CATEGORY_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrCategoryContentNotFound, "BLOG_CATEGORY_CONTENT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrTagNotFound, "BLOG_TAG_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrTagContentNotFound, "BLOG_TAG_CONTENT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrPostNotFound, "BLOG_POST_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrPostContentNotFound, "BLOG_POST_CONTENT_NOT_FOUND", apperr.ClassNotFound)

	apperr.RegisterSentinel(ErrCategoryHasChildren, "BLOG_CATEGORY_HAS_CHILDREN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCategoryHasPosts, "BLOG_CATEGORY_HAS_POSTS", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrSlugTaken, "BLOG_SLUG_TAKEN", apperr.ClassConflict)

	apperr.RegisterSentinel(ErrInvalidSlug, "BLOG_INVALID_SLUG", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrInvalidStatus, "BLOG_INVALID_STATUS", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPublishedAtRequired, "BLOG_PUBLISHED_AT_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrScheduledInPast, "BLOG_SCHEDULED_IN_PAST", apperr.ClassValidation)
}
