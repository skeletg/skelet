package admin

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// The role fields and displayImage answer two different questions, and the dashboard depends on the difference: the media-role editor uses mainImage as the assignment itself AND as its staged-save baseline, so a stand-in there would make "set this as main" read as no change and never fire
func TestAdminProductImageFieldsSeparateRolesFromDisplay(t *testing.T) {
	first := &model.ProductMedia{ID: uuid.NewString(), URL: "first.jpg"}
	second := &model.ProductMedia{ID: uuid.NewString(), URL: "second.jpg"}
	mediaList := []*model.ProductMedia{first, second}
	roleFor := func(role catalog.MediaRole, m *model.ProductMedia) catalog.Product {
		id, err := uuid.Parse(m.ID)
		if err != nil {
			t.Fatalf("parse id: %v", err)
		}
		return catalog.Product{MediaRoles: map[catalog.MediaRole]uuid.UUID{role: id}}
	}

	// no roles at all: the roles stay empty, display falls back to the first image
	bare := toProductBaseModel(catalog.Product{}, nil, nil, nil, mediaList, nil, nil, nil)
	if bare.MainImage != nil || bare.Thumbnail != nil || bare.OgImage != nil {
		t.Errorf("a role was invented: main=%v thumb=%v og=%v", bare.MainImage, bare.Thumbnail, bare.OgImage)
	}
	if bare.DisplayImage != first {
		t.Errorf("DisplayImage = %v, want the first image", bare.DisplayImage)
	}

	// an assigned main image is what both fields report
	withMain := toProductBaseModel(roleFor(catalog.MediaRoleMain, second), nil, nil, nil, mediaList, nil, nil, nil)
	if withMain.MainImage != second || withMain.DisplayImage != second {
		t.Errorf("main=%v display=%v, want both to be the assigned image", withMain.MainImage, withMain.DisplayImage)
	}

	// no main but a thumbnail: display prefers the thumbnail over document order
	withThumb := toProductBaseModel(roleFor(catalog.MediaRoleThumbnail, second), nil, nil, nil, mediaList, nil, nil, nil)
	if withThumb.MainImage != nil {
		t.Errorf("MainImage = %v, want nil - only a thumbnail was assigned", withThumb.MainImage)
	}
	if withThumb.DisplayImage != second {
		t.Errorf("DisplayImage = %v, want the thumbnail", withThumb.DisplayImage)
	}

	// and with no media, everything stays nil rather than becoming a zero-value struct
	empty := toProductBaseModel(catalog.Product{}, nil, nil, nil, nil, nil, nil, nil)
	if empty.DisplayImage != nil {
		t.Errorf("DisplayImage = %v, want nil with no media", empty.DisplayImage)
	}
}

func TestPriceBoundTakesPlainAmountsOnly(t *testing.T) {
	for in, want := range map[string]string{"": "", "  7 ": "7", "12.50": "12.50", "1000": "1000"} {
		if got, err := priceBound("priceMin", &in); err != nil || got != want {
			t.Errorf("priceBound(%q) = %q, %v - want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"-1", "1e5", "12.5.0", "abc", "12,50", "1234567890123"} {
		if _, err := priceBound("priceMin", &in); apperr.CodeOf(err) != "INVALID_INPUT" {
			t.Errorf("priceBound(%q) err = %v, want INVALID_INPUT", in, err)
		}
	}
	if got, err := priceBound("priceMax", nil); err != nil || got != "" {
		t.Errorf("priceBound(nil) = %q, %v", got, err)
	}
}
