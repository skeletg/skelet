package catalog_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
)

// Every exported catalog sentinel must be paired with a code and class via apperr.RegisterSentinel
// A forgotten one shows up as extensions.code = INTERNAL on what should be a typed error - easy to miss, so lock it down
func TestCatalogSentinelsRegistered(t *testing.T) {
	cases := []struct {
		err       error
		wantCode  string
		wantClass apperr.Class
	}{
		{catalog.ErrCategoryNotFound, "CATEGORY_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrCategoryProductNotFound, "CATEGORY_PRODUCT_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrCategoryChannelListingNotFound, "CATEGORY_STORE_LISTING_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrProductNotFound, "PRODUCT_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrProductContentNotFound, "PRODUCT_CONTENT_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrCollectionNotFound, "COLLECTION_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrCollectionContentNotFound, "COLLECTION_CONTENT_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrProductTypeNotFound, "PRODUCT_TYPE_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrAttributeNotFound, "ATTRIBUTE_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrMediaNotFound, "MEDIA_NOT_FOUND", apperr.ClassNotFound},

		{catalog.ErrSlugTaken, "SLUG_TAKEN", apperr.ClassConflict},
		{catalog.ErrCategorySlugTaken, "CATEGORY_SLUG_TAKEN", apperr.ClassConflict},
		{catalog.ErrCategoryHasChildren, "CATEGORY_HAS_CHILDREN", apperr.ClassConflict},
		{catalog.ErrCategoryHasProducts, "CATEGORY_HAS_PRODUCTS", apperr.ClassConflict},
		{catalog.ErrSKUTaken, "SKU_TAKEN", apperr.ClassConflict},
		{catalog.ErrAlreadyLinked, "PRODUCT_CHILD_ALREADY_LINKED", apperr.ClassConflict},
		{catalog.ErrGroupedMemberDuplicate, "GROUPED_MEMBER_DUPLICATE", apperr.ClassConflict},

		{catalog.ErrGroupedMemberNotFound, "GROUPED_MEMBER_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrGroupedParentNotGrouped, "GROUPED_PARENT_NOT_GROUPED", apperr.ClassValidation},
		{catalog.ErrGroupedMemberNotSimple, "GROUPED_MEMBER_NOT_SIMPLE", apperr.ClassValidation},
		{catalog.ErrGroupedMemberIsParent, "GROUPED_MEMBER_IS_PARENT", apperr.ClassValidation},

		{catalog.ErrProductOptionNotFound, "PRODUCT_OPTION_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrProductOptionValueNotFound, "PRODUCT_OPTION_VALUE_NOT_FOUND", apperr.ClassNotFound},
		{catalog.ErrInvalidProductOptionInputType, "INVALID_PRODUCT_OPTION_INPUT_TYPE", apperr.ClassValidation},
		{catalog.ErrOptionInputKindNotPurchasable, "PRODUCT_OPTION_KIND_NOT_PURCHASABLE", apperr.ClassValidation},
		{catalog.ErrOptionValueOnNonSelect, "PRODUCT_OPTION_VALUE_ON_NON_SELECT", apperr.ClassValidation},
		{catalog.ErrOptionPriceOnSelect, "PRODUCT_OPTION_PRICE_ON_SELECT", apperr.ClassValidation},
		{catalog.ErrOptionMultiDefaultOnSingle, "PRODUCT_OPTION_MULTI_DEFAULT_ON_SINGLE", apperr.ClassValidation},

		{catalog.ErrInvalidSlug, "INVALID_SLUG", apperr.ClassValidation},
		{catalog.ErrLocaleMustBeDefault, "LOCALE_MUST_BE_DEFAULT", apperr.ClassValidation},
		{catalog.ErrInvalidCurrency, "INVALID_CURRENCY_CODE", apperr.ClassValidation},
		{catalog.ErrFileTooLarge, "FILE_TOO_LARGE", apperr.ClassValidation},

		{catalog.ErrUploadNotReceived, "UPLOAD_NOT_RECEIVED", apperr.ClassUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.wantCode, func(t *testing.T) {
			if got := apperr.CodeOf(tc.err); got != tc.wantCode {
				t.Errorf("CodeOf(%v) = %q, want %q", tc.err, got, tc.wantCode)
			}
			if got := apperr.ClassOf(tc.err); got != tc.wantClass {
				t.Errorf("ClassOf(%v) = %v, want %v", tc.err, got, tc.wantClass)
			}
		})
	}
}

// The registry has to walk wrapped chains - services often return fmt.Errorf("ctx: %w", err)
func TestCatalogSentinelWrappedByFmtErrorf(t *testing.T) {
	wrapped := fmt.Errorf("getting category: %w", catalog.ErrCategoryNotFound)
	if !apperr.IsNotFound(wrapped) {
		t.Fatalf("IsNotFound should walk wrap chain, but returned false")
	}
	if !errors.Is(wrapped, catalog.ErrCategoryNotFound) {
		t.Fatalf("errors.Is still required to match the original sentinel")
	}
	if got := apperr.CodeOf(wrapped); got != "CATEGORY_NOT_FOUND" {
		t.Fatalf("CodeOf(wrapped) = %q, want CATEGORY_NOT_FOUND", got)
	}
}

// An apperr.WrapClass wrap keeps the original sentinel for CodeOf, but the wrapper's Class wins for ClassOf
func TestCatalogSentinelWrappedByAppErr(t *testing.T) {
	wrapped := apperr.WrapClass(catalog.ErrCategoryNotFound, apperr.ClassNotFound, "test.Op")
	if got := apperr.CodeOf(wrapped); got != "CATEGORY_NOT_FOUND" {
		t.Fatalf("CodeOf(WrapClass(...)) = %q, want CATEGORY_NOT_FOUND", got)
	}
}
