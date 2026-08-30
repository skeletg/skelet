package front

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/apperr"
	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/catalog"
)

// The allow-list is admin-authored free text, so it has to absorb the ways it gets typed while still refusing everything not on it
func TestValidateUploadExtensionAcceptsWhatTheAllowListNames(t *testing.T) {
	const allow = "jpg, PNG, .pdf"
	for _, filename := range []string{
		"photo.jpg", "photo.JPG", "photo.Jpg",
		"art.png", "art.PNG",
		"terms.pdf", "terms.PDF",
		"my.holiday.photo.jpg", // only the last segment is the extension
		"a.jpg",
	} {
		if err := validateUploadExtension(allow, filename); err != nil {
			t.Errorf("validateUploadExtension(%q, %q) = %v, want nil", allow, filename, err)
		}
	}
}

// This is the presign-time half of the gate, so anything it lets through gets a signed URL to object storage
func TestValidateUploadExtensionRefusesAnythingElse(t *testing.T) {
	const allow = "jpg, PNG, .pdf"
	for name, filename := range map[string]string{
		"executable":        "payload.exe",
		"script":            "shell.sh",
		"php":               "backdoor.php",
		"svg":               "vector.svg",
		"no extension":      "README",
		"trailing dot":      "photo.",
		"empty":             "",
		"double extension":  "invoice.pdf.exe",
		"near miss":         "photo.jpeg", // jpeg is not jpg
		"extension in stem": "jpg.exe",
		"padded extension":  "photo.jpg ",
		"query string":      "photo.jpg?x=1",
	} {
		err := validateUploadExtension(allow, filename)
		if err == nil {
			t.Errorf("%s: validateUploadExtension(%q) = nil, want it refused", name, filename)
			continue
		}
		if code := apperr.CodeOf(err); code != "FILE_TYPE_NOT_ALLOWED" {
			t.Errorf("%s: error code = %q, want FILE_TYPE_NOT_ALLOWED", name, code)
		}
	}
}

// The gate is about the TYPE, not the name - a bare ".jpg" has extension jpg and passes, and the downstream key builder is what deals with the empty stem
func TestValidateUploadExtensionJudgesTheExtensionNotTheStem(t *testing.T) {
	for _, filename := range []string{".jpg", "..jpg", "a.jpg"} {
		if err := validateUploadExtension("jpg", filename); err != nil {
			t.Errorf("validateUploadExtension(%q) = %v, want nil - the extension is jpg either way", filename, err)
		}
	}
	// a trailing dot leaves no extension at all, so there is nothing to match
	if err := validateUploadExtension("jpg", "photo."); err == nil {
		t.Error(`validateUploadExtension("jpg", "photo.") = nil, want it refused`)
	}
}

// An option with no allow-list set accepts anything by design, which is why the option editor defaults to a list rather than empty
func TestValidateUploadExtensionWithNoAllowListAcceptsEverything(t *testing.T) {
	for _, allow := range []string{"", "   ", "\t"} {
		for _, filename := range []string{"payload.exe", "photo.jpg", "no-extension"} {
			if err := validateUploadExtension(allow, filename); err != nil {
				t.Errorf("validateUploadExtension(%q, %q) = %v, want nil for an unset allow-list", allow, filename, err)
			}
		}
	}
}

// A malformed allow-list must not accidentally permit an empty extension, which would let "README" through
func TestValidateUploadExtensionIgnoresEmptyAllowListEntries(t *testing.T) {
	for _, allow := range []string{"jpg,,png", "jpg, , png", "jpg,.,png", ",jpg,"} {
		if err := validateUploadExtension(allow, "README"); err == nil {
			t.Errorf("validateUploadExtension(%q, \"README\") = nil, want the empty entry ignored", allow)
		}
		if err := validateUploadExtension(allow, "photo.jpg"); err != nil {
			t.Errorf("validateUploadExtension(%q, \"photo.jpg\") = %v, want the real entries still honored", allow, err)
		}
	}
}

// The refusal is shown to the shopper, so it has to name the type they actually picked
func TestValidateUploadExtensionErrorNamesTheExtension(t *testing.T) {
	err := validateUploadExtension("jpg", "payload.exe")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "exe") {
		t.Errorf("error = %v, want it to name the rejected extension", err)
	}
	if apperr.ClassOf(err) != apperr.ClassValidation {
		t.Errorf("error class = %v, want ClassValidation so it surfaces as a user error", apperr.ClassOf(err))
	}
}

// Thumbnail wins, then main, then the first ready image - a cart row showing the wrong image is a support ticket
func TestPickCartImagePrefersTheThumbnailRole(t *testing.T) {
	thumb := catalog.ProductMedia{ID: uuid.New(), URL: "/thumb.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}
	main := catalog.ProductMedia{ID: uuid.New(), URL: "/main.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}
	other := catalog.ProductMedia{ID: uuid.New(), URL: "/other.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}
	items := []catalog.ProductMedia{other, main, thumb}

	got := pickCartImage(items, map[catalog.MediaRole]uuid.UUID{
		catalog.MediaRoleThumbnail: thumb.ID,
		catalog.MediaRoleMain:      main.ID,
	})
	if got == nil || got.URL != "/thumb.jpg" {
		t.Errorf("pickCartImage = %v, want the thumbnail", got)
	}

	// no thumbnail role set, so main takes over
	got = pickCartImage(items, map[catalog.MediaRole]uuid.UUID{catalog.MediaRoleMain: main.ID})
	if got == nil || got.URL != "/main.jpg" {
		t.Errorf("pickCartImage = %v, want the main image", got)
	}
}

// A role pointing at media this batch didn't load has to fall through rather than render nothing
func TestPickCartImageFallsBackToTheFirstReadyImage(t *testing.T) {
	pending := catalog.ProductMedia{ID: uuid.New(), URL: "/pending.jpg", Status: catalog.MediaStatusPending, MimeType: "image/jpeg"}
	doc := catalog.ProductMedia{ID: uuid.New(), URL: "/spec.pdf", Status: catalog.MediaStatusReady, MimeType: "application/pdf"}
	ready := catalog.ProductMedia{ID: uuid.New(), URL: "/ready.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}
	items := []catalog.ProductMedia{pending, doc, ready}

	// the role points at a row that isn't in this slice
	got := pickCartImage(items, map[catalog.MediaRole]uuid.UUID{catalog.MediaRoleThumbnail: uuid.New()})
	if got == nil || got.URL != "/ready.jpg" {
		t.Errorf("pickCartImage = %v, want the first ready image", got)
	}
	if got := pickCartImage(items, nil); got == nil || got.URL != "/ready.jpg" {
		t.Errorf("pickCartImage with no roles = %v, want the first ready image", got)
	}
}

// A pending upload or a PDF is not a cart thumbnail - showing either would render a broken image
func TestPickCartImageSkipsPendingAndNonImages(t *testing.T) {
	for name, items := range map[string][]catalog.ProductMedia{
		"only pending":   {{ID: uuid.New(), Status: catalog.MediaStatusPending, MimeType: "image/jpeg"}},
		"only documents": {{ID: uuid.New(), Status: catalog.MediaStatusReady, MimeType: "application/pdf"}},
		"only video":     {{ID: uuid.New(), Status: catalog.MediaStatusReady, MimeType: "video/mp4"}},
		"empty":          {},
		"nil":            nil,
	} {
		if got := pickCartImage(items, nil); got != nil {
			t.Errorf("%s: pickCartImage = %v, want nil", name, got)
		}
	}
}

// A role that resolves must win even when an earlier item would have satisfied the ready-image fallback
func TestPickCartImageRoleBeatsTheFallback(t *testing.T) {
	first := catalog.ProductMedia{ID: uuid.New(), URL: "/first.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}
	tagged := catalog.ProductMedia{ID: uuid.New(), URL: "/tagged.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}
	got := pickCartImage([]catalog.ProductMedia{first, tagged}, map[catalog.MediaRole]uuid.UUID{catalog.MediaRoleMain: tagged.ID})
	if got == nil || got.URL != "/tagged.jpg" {
		t.Errorf("pickCartImage = %v, want the role-tagged image rather than the first ready one", got)
	}
}

// The pointer has to address the slice element, not a loop copy, or callers mutating it write to nothing
func TestPickCartImageReturnsAPointerIntoTheSlice(t *testing.T) {
	items := []catalog.ProductMedia{{ID: uuid.New(), URL: "/a.jpg", Status: catalog.MediaStatusReady, MimeType: "image/jpeg"}}
	got := pickCartImage(items, nil)
	if got != &items[0] {
		t.Errorf("pickCartImage returned %p, want a pointer to items[0] at %p", got, &items[0])
	}
}

// Line totals are unit price times a possibly-fractional quantity, rounded once - recomputing per render is what keeps the cart matching PlaceOrder
func TestToCartLineModelComputesTheLineTotal(t *testing.T) {
	for name, tc := range map[string]struct {
		unit  int64
		qty   float64
		total int32
	}{
		"whole":          {5000, 4, 20000},
		"fractional":     {1000, 1.5, 1500},
		"rounds half up": {333, 1.5, 500}, // 499.5 rounds to 500
		"zero quantity":  {5000, 0, 0},
		"zero price":     {0, 3, 0},
		"single":         {1299, 1, 1299},
	} {
		got := toCartLineModel(cart.CartLine{ID: uuid.New(), UnitPrice: tc.unit, Quantity: tc.qty})
		if got.TotalPrice != tc.total {
			t.Errorf("%s: total for %d x %v = %d, want %d", name, tc.unit, tc.qty, got.TotalPrice, tc.total)
		}
		if got.UnitPrice != int32(tc.unit) {
			t.Errorf("%s: unit price = %d, want %d", name, got.UnitPrice, tc.unit)
		}
	}
}

// Both lists are non-null in the schema, so a plain line must serialize them as [] rather than null
func TestToCartLineModelNeverReturnsNilLists(t *testing.T) {
	got := toCartLineModel(cart.CartLine{ID: uuid.New()})
	if got.Components == nil {
		t.Error("Components = nil, want an empty slice")
	}
	if got.Options == nil {
		t.Error("Options = nil, want an empty slice")
	}
}

// An option value id is nullable for free-text and file options, and an all-zeros uuid would read as a real selection
func TestToCartLineModelLeavesAnAbsentOptionValueNil(t *testing.T) {
	valueID := uuid.New()
	line := cart.CartLine{
		ID: uuid.New(),
		Options: []cart.CartLineOption{
			{ID: uuid.New(), OptionTitle: "Size", ValueLabel: "Large", OptionValueID: &valueID},
			{ID: uuid.New(), OptionTitle: "Engraving", TextValue: "For Grace"},
		},
	}
	got := toCartLineModel(line)
	if len(got.Options) != 2 {
		t.Fatalf("%d options, want 2", len(got.Options))
	}
	if got.Options[0].OptionValueID == nil || *got.Options[0].OptionValueID != valueID.String() {
		t.Errorf("select option value id = %v, want %q", got.Options[0].OptionValueID, valueID)
	}
	if got.Options[1].OptionValueID != nil {
		t.Errorf("free-text option value id = %v, want nil", got.Options[1].OptionValueID)
	}
}

// The bundle components carry their own money, and the option/selection ids are what the storefront re-renders the picker from
func TestToCartLineModelCarriesComponents(t *testing.T) {
	optID, selID := uuid.New(), uuid.New()
	got := toCartLineModel(cart.CartLine{
		ID: uuid.New(),
		Components: []cart.CartLineComponent{{
			ID: uuid.New(), BundleOptionID: optID, BundleSelectionID: selID,
			ProductSku: "W-1", ProductName: "Alloy", UnitPrice: 5000, Quantity: 4, SortOrder: 2,
		}},
	})
	if len(got.Components) != 1 {
		t.Fatalf("%d components, want 1", len(got.Components))
	}
	c := got.Components[0]
	if c.OptionID != optID.String() || c.SelectionID != selID.String() {
		t.Errorf("component ids = %q / %q, want %q / %q", c.OptionID, c.SelectionID, optID, selID)
	}
	if c.UnitPrice != 5000 || c.Quantity != 4 || c.SortOrder != 2 {
		t.Errorf("component = %+v, want the snapshot values carried through", c)
	}
}

// The cart subtotal is the sum of per-line totals, each rounded on its own - summing then rounding drifts by a cent per fractional line
func TestToCartModelSubtotalSumsRoundedLineTotals(t *testing.T) {
	c := cart.Cart{
		ID:   uuid.New(),
		Note: "leave at door",
		Lines: []cart.CartLine{
			{ID: uuid.New(), UnitPrice: 333, Quantity: 1.5}, // 500 after rounding
			{ID: uuid.New(), UnitPrice: 333, Quantity: 1.5}, // 500
			{ID: uuid.New(), UnitPrice: 1000, Quantity: 2},  // 2000
		},
	}
	got := toCartModel(c)
	if got.Subtotal != 3000 {
		t.Errorf("subtotal = %d, want 3000 from the three rounded line totals", got.Subtotal)
	}
	if len(got.Lines) != 3 {
		t.Errorf("%d lines, want 3", len(got.Lines))
	}
	if got.Note != "leave at door" {
		t.Errorf("note = %q, want it carried through", got.Note)
	}
	if got.ID != c.ID.String() {
		t.Errorf("id = %q, want %q", got.ID, c.ID)
	}
}

// The subtotal has to agree with what the line models report, or the storefront's own sum disagrees with the server's
func TestToCartModelSubtotalMatchesTheSumOfItsLines(t *testing.T) {
	c := cart.Cart{ID: uuid.New(), Lines: []cart.CartLine{
		{ID: uuid.New(), UnitPrice: 1299, Quantity: 3},
		{ID: uuid.New(), UnitPrice: 4999, Quantity: 1.25},
		{ID: uuid.New(), UnitPrice: 7, Quantity: 0.5},
	}}
	got := toCartModel(c)
	var sum int32
	for _, l := range got.Lines {
		sum += l.TotalPrice
	}
	if got.Subtotal != sum {
		t.Errorf("subtotal = %d but the lines sum to %d", got.Subtotal, sum)
	}
}

func TestToCartModelHandlesAnEmptyCart(t *testing.T) {
	got := toCartModel(cart.Cart{ID: uuid.New()})
	if got.Lines == nil {
		t.Error("Lines = nil, want an empty slice for the non-null field")
	}
	if got.Subtotal != 0 {
		t.Errorf("subtotal = %d, want 0", got.Subtotal)
	}
}
