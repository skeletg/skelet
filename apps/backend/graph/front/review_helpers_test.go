package front

import (
	"testing"
	"time"

	"github.com/google/uuid"

	adminmodel "github.com/skeletg/skelet/graph/admin/model"
	frontmodel "github.com/skeletg/skelet/graph/front/model"
	"github.com/skeletg/skelet/internal/review"
)

func rating(n int32) *int32 { return &n }

func answer(r *int32) *adminmodel.ReviewAnswer { return &adminmodel.ReviewAnswer{Rating: r} }

// The star count on the PDP is this average, so a non-rating answer counted as a zero would drag every product's score down
func TestComputeOverallRatingAveragesOnlyRatedAnswers(t *testing.T) {
	for name, tc := range map[string]struct {
		answers []*adminmodel.ReviewAnswer
		want    *float64
	}{
		"single":             {[]*adminmodel.ReviewAnswer{answer(rating(4))}, ptrF(4)},
		"whole average":      {[]*adminmodel.ReviewAnswer{answer(rating(4)), answer(rating(2))}, ptrF(3)},
		"fractional average": {[]*adminmodel.ReviewAnswer{answer(rating(5)), answer(rating(4))}, ptrF(4.5)},
		"free-text ignored":  {[]*adminmodel.ReviewAnswer{answer(rating(4)), answer(nil)}, ptrF(4)},
		"nil entry ignored":  {[]*adminmodel.ReviewAnswer{answer(rating(4)), nil}, ptrF(4)},
		"only unrated":       {[]*adminmodel.ReviewAnswer{answer(nil), nil}, nil},
		"no answers":         {nil, nil},
		"empty":              {[]*adminmodel.ReviewAnswer{}, nil},
		"a legitimate zero":  {[]*adminmodel.ReviewAnswer{answer(rating(0)), answer(rating(4))}, ptrF(2)},
	} {
		got := computeOverallRating(tc.answers)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("%s: computeOverallRating = %v, want nil so the PDP renders no score", name, *got)
		case tc.want != nil && got == nil:
			t.Errorf("%s: computeOverallRating = nil, want %v", name, *tc.want)
		case tc.want != nil && *got != *tc.want:
			t.Errorf("%s: computeOverallRating = %v, want %v", name, *got, *tc.want)
		}
	}
}

func ptrF(f float64) *float64 { return &f }

func reviewAt(t time.Time, ratings ...int32) *adminmodel.Review {
	r := &adminmodel.Review{ID: uuid.NewString(), SubmittedAt: t}
	for _, n := range ratings {
		r.Answers = append(r.Answers, answer(rating(n)))
	}
	return r
}

// NEWEST is the natural order the repo already returns, and the sort must not disturb it
func TestSortReviewsNewestFirst(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	old := reviewAt(base.Add(-48 * time.Hour))
	mid := reviewAt(base.Add(-24 * time.Hour))
	recent := reviewAt(base)

	list := []*adminmodel.Review{old, recent, mid}
	sortReviews(list, frontmodel.ReviewSortNewest)
	if list[0] != recent || list[1] != mid || list[2] != old {
		t.Errorf("NEWEST order = %v, want recent, mid, old", submittedOrder(list))
	}
}

func TestSortReviewsOldestFirst(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	old := reviewAt(base.Add(-48 * time.Hour))
	mid := reviewAt(base.Add(-24 * time.Hour))
	recent := reviewAt(base)

	list := []*adminmodel.Review{recent, old, mid}
	sortReviews(list, frontmodel.ReviewSortOldest)
	if list[0] != old || list[1] != mid || list[2] != recent {
		t.Errorf("OLDEST order = %v, want old, mid, recent", submittedOrder(list))
	}
}

func submittedOrder(list []*adminmodel.Review) []time.Time {
	out := make([]time.Time, 0, len(list))
	for _, r := range list {
		out = append(out, r.SubmittedAt)
	}
	return out
}

// HIGHEST and LOWEST sort on the computed average of the review's answers
func TestSortReviewsByRating(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	five := reviewAt(base, 5)
	three := reviewAt(base, 3)
	one := reviewAt(base, 1)

	list := []*adminmodel.Review{three, one, five}
	sortReviews(list, frontmodel.ReviewSortHighest)
	if list[0] != five || list[1] != three || list[2] != one {
		t.Errorf("HIGHEST order = %v, want 5, 3, 1", ratingOrder(list))
	}

	list = []*adminmodel.Review{three, five, one}
	sortReviews(list, frontmodel.ReviewSortLowest)
	if list[0] != one || list[1] != three || list[2] != five {
		t.Errorf("LOWEST order = %v, want 1, 3, 5", ratingOrder(list))
	}
}

// A free-text-only review has no average, and both rating sorts order it as if it rated lower than anything - last under HIGHEST, first under LOWEST
func TestSortReviewsTreatsAnUnratedReviewAsTheLowest(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	five := reviewAt(base, 5)
	one := reviewAt(base, 1)
	unrated := reviewAt(base)

	list := []*adminmodel.Review{five, unrated, one}
	sortReviews(list, frontmodel.ReviewSortHighest)
	if indexOf(list, unrated) != 2 {
		t.Errorf("HIGHEST put the unrated review at %d, want it last: %v", indexOf(list, unrated), ratingOrder(list))
	}

	list = []*adminmodel.Review{five, unrated, one}
	sortReviews(list, frontmodel.ReviewSortLowest)
	if indexOf(list, unrated) != 0 {
		t.Errorf("LOWEST put the unrated review at %d, want it first: %v", indexOf(list, unrated), ratingOrder(list))
	}

	// whichever direction, the rated ones stay correctly ordered relative to each other
	if indexOf(list, one) >= indexOf(list, five) {
		t.Errorf("LOWEST order = %v, want the 1-star ahead of the 5-star", ratingOrder(list))
	}
}

func ratingOrder(list []*adminmodel.Review) []any {
	out := make([]any, 0, len(list))
	for _, r := range list {
		if avg := computeOverallRating(r.Answers); avg != nil {
			out = append(out, *avg)
		} else {
			out = append(out, "unrated")
		}
	}
	return out
}

func indexOf(list []*adminmodel.Review, want *adminmodel.Review) int {
	for i, r := range list {
		if r == want {
			return i
		}
	}
	return -1
}

// A multi-question review averages its answers, and that average is what the rating sorts compare
func TestSortReviewsUsesTheAveragedRating(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	fourAvg := reviewAt(base, 5, 3)   // 4.0
	threeFive := reviewAt(base, 4, 3) // 3.5

	list := []*adminmodel.Review{threeFive, fourAvg}
	sortReviews(list, frontmodel.ReviewSortHighest)
	if list[0] != fourAvg {
		t.Errorf("HIGHEST head averages %v, want the 4.0 review first", ratingOrder(list))
	}
}

// An unrecognized sort value must not scramble the list - it falls through to NEWEST
func TestSortReviewsFallsBackToNewest(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	old := reviewAt(base.Add(-time.Hour))
	recent := reviewAt(base)
	list := []*adminmodel.Review{old, recent}
	sortReviews(list, frontmodel.ReviewSort("TRENDING"))
	if list[0] != recent {
		t.Errorf("an unknown sort = %v, want the NEWEST fallback", submittedOrder(list))
	}
}

func TestSortReviewsHandlesTrivialInput(t *testing.T) {
	sortReviews(nil, frontmodel.ReviewSortHighest)                    // must not panic
	sortReviews([]*adminmodel.Review{}, frontmodel.ReviewSortHighest) //
	one := []*adminmodel.Review{reviewAt(time.Now(), 3)}
	sortReviews(one, frontmodel.ReviewSortLowest)
	if len(one) != 1 {
		t.Errorf("a single-element sort changed the length to %d", len(one))
	}
}

// The storefront renders a different control per input type, so collapsing two onto one shows the wrong widget
func TestInputTypeToModelIsInjective(t *testing.T) {
	inputs := []review.InputType{review.InputTypeRating1To5, review.InputTypeYesNo, review.InputTypeFreeText}
	seen := make(map[adminmodel.ReviewQuestionInputType]review.InputType, len(inputs))
	for _, in := range inputs {
		got := inputTypeToModel(in)
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, got)
		}
		seen[got] = in
	}
	// an unknown type falls back to the rating control, which is the only one that can render a star row
	for _, in := range []review.InputType{"", "slider", "RATING_1_5"} {
		if got := inputTypeToModel(in); got != adminmodel.ReviewQuestionInputTypeRating1_5 {
			t.Errorf("inputTypeToModel(%q) = %q, want the rating fallback", in, got)
		}
	}
}

// The front Label resolver reads Content, and AllContent has to carry the same row so either read path works
func TestReviewQuestionToModelMirrorsContentIntoAllContent(t *testing.T) {
	q := review.Question{
		ID: uuid.New(), Slug: "fit", InputType: review.InputTypeRating1To5,
		IsRequired: true, AllowComment: true, SortOrder: 3,
		Content: &review.QuestionContent{Locale: "de", Label: "Passform", HelpText: "Wie faellt sie aus"},
	}

	got := reviewQuestionToModel(q)
	if got.Slug != "fit" || !got.IsRequired || !got.AllowComment || got.SortOrder != 3 {
		t.Errorf("question = %+v, want the scalar fields carried through", got)
	}
	if got.Content == nil {
		t.Fatal("Content = nil, want the hydrated row")
	}
	if got.Content.Label != "Passform" || got.Content.Locale != "de" {
		t.Errorf("content = %+v, want the localized row", got.Content)
	}
	if len(got.AllContent) != 1 || got.AllContent[0] != got.Content {
		t.Errorf("AllContent = %+v, want it to hold the same row Content does", got.AllContent)
	}
}

// A question with no content for the active locale must leave both fields alone rather than emit a blank label
func TestReviewQuestionToModelWithNoContent(t *testing.T) {
	got := reviewQuestionToModel(review.Question{ID: uuid.New(), Slug: "fit", InputType: review.InputTypeYesNo})
	if got.Content != nil {
		t.Errorf("Content = %+v, want nil when nothing was hydrated", got.Content)
	}
	if len(got.AllContent) != 0 {
		t.Errorf("AllContent = %+v, want it empty", got.AllContent)
	}
}

// A malformed id has to fail here rather than travel down into the service as a zero uuid
func TestProductIDFromObj(t *testing.T) {
	id := uuid.New()
	got, err := productIDFromObj(&adminmodel.Product{ID: id.String()})
	if err != nil || got != id {
		t.Errorf("productIDFromObj = (%v, %v), want %v", got, err, id)
	}

	for name, obj := range map[string]*adminmodel.Product{
		"nil object": nil,
		"empty id":   {ID: ""},
		"garbage id": {ID: "not-a-uuid"},
	} {
		got, err := productIDFromObj(obj)
		if err == nil {
			t.Errorf("%s: productIDFromObj = %v, want an error", name, got)
		}
		if got != uuid.Nil {
			t.Errorf("%s: productIDFromObj returned %v alongside the error, want uuid.Nil", name, got)
		}
	}
}
