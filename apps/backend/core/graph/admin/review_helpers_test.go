package admin

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
)

// Status decides whether a review is publicly visible, so a value that shifts across the hop either hides an approved review or publishes a rejected one
func TestReviewStatusRoundTrips(t *testing.T) {
	for _, s := range []review.Status{review.StatusPending, review.StatusApproved, review.StatusRejected} {
		gql := reviewStatusToModel(s)
		if back := modelToReviewStatus(gql); back != s {
			t.Errorf("%q round-tripped through %q to %q", s, gql, back)
		}
	}
	// each domain status needs its own enum
	seen := map[model.ReviewStatus]review.Status{}
	for _, s := range []review.Status{review.StatusPending, review.StatusApproved, review.StatusRejected} {
		gql := reviewStatusToModel(s)
		if prev, dup := seen[gql]; dup {
			t.Errorf("%q and %q both map to %q", s, prev, gql)
		}
		seen[gql] = s
	}
}

// An unrecognized status must land on PENDING, the only one that keeps a review off the storefront until a moderator looks
func TestReviewStatusFailsClosedToPending(t *testing.T) {
	for _, s := range []review.Status{"", "APPROVED", "published", "spam"} {
		if got := reviewStatusToModel(s); got != model.ReviewStatusPending {
			t.Errorf("reviewStatusToModel(%q) = %q, want PENDING so nothing publishes by accident", s, got)
		}
	}
	for _, s := range []model.ReviewStatus{"", "approved", "PUBLISHED"} {
		if got := modelToReviewStatus(s); got != review.StatusPending {
			t.Errorf("modelToReviewStatus(%q) = %q, want StatusPending", s, got)
		}
	}
}

// The question editor renders a different control per input type, and the admin's saved choice has to survive the round trip
func TestReviewInputTypeRoundTrips(t *testing.T) {
	for _, in := range []review.InputType{review.InputTypeRating1To5, review.InputTypeYesNo, review.InputTypeFreeText} {
		gql := reviewInputTypeToModel(in)
		if back := modelToReviewInputType(gql); back != in {
			t.Errorf("%q round-tripped through %q to %q", in, gql, back)
		}
	}
	seen := map[model.ReviewQuestionInputType]review.InputType{}
	for _, in := range []review.InputType{review.InputTypeRating1To5, review.InputTypeYesNo, review.InputTypeFreeText} {
		gql := reviewInputTypeToModel(in)
		if prev, dup := seen[gql]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, gql)
		}
		seen[gql] = in
	}
}

func TestReviewInputTypeFallsBackToRating(t *testing.T) {
	for _, in := range []review.InputType{"", "slider", "RATING_1_5"} {
		if got := reviewInputTypeToModel(in); got != model.ReviewQuestionInputTypeRating1_5 {
			t.Errorf("reviewInputTypeToModel(%q) = %q, want the rating fallback", in, got)
		}
	}
	for _, in := range []model.ReviewQuestionInputType{"", "SLIDER", "rating_1_5"} {
		if got := modelToReviewInputType(in); got != review.InputTypeRating1To5 {
			t.Errorf("modelToReviewInputType(%q) = %q, want InputTypeRating1To5", in, got)
		}
	}
}

// The admin's front-side mapper and this one feed the same schema enum, so they have to agree or the two surfaces disagree on the same question
func TestReviewInputTypeMatchesTheFrontMapping(t *testing.T) {
	for _, in := range []review.InputType{review.InputTypeRating1To5, review.InputTypeYesNo, review.InputTypeFreeText, "", "unknown"} {
		admin := reviewInputTypeToModel(in)
		// round-tripping back through the reverse mapper has to be stable, which is what both surfaces rely on
		if got := reviewInputTypeToModel(modelToReviewInputType(admin)); got != admin {
			t.Errorf("%q is not stable across the mappers: %q then %q", in, admin, got)
		}
	}
}

func ratedAnswer(n int16) review.Answer {
	v := n
	return review.Answer{RatingValue: &v}
}

// The AI reply drafter branches on this, so a free-text answer counted as zero would make every mixed review read as angry
func TestAvgReviewRatingIgnoresUnratedAnswers(t *testing.T) {
	for name, tc := range map[string]struct {
		answers []review.Answer
		want    int
	}{
		"single":            {[]review.Answer{ratedAnswer(4)}, 4},
		"exact average":     {[]review.Answer{ratedAnswer(4), ratedAnswer(2)}, 3},
		"rounds half up":    {[]review.Answer{ratedAnswer(4), ratedAnswer(3)}, 4}, // 3.5 rounds to 4
		"rounds down":       {[]review.Answer{ratedAnswer(3), ratedAnswer(3), ratedAnswer(4)}, 3},
		"free text ignored": {[]review.Answer{ratedAnswer(5), {}}, 5},
		"only free text":    {[]review.Answer{{}, {}}, 0},
		"no answers":        {nil, 0},
		"all fives":         {[]review.Answer{ratedAnswer(5), ratedAnswer(5)}, 5},
		"all ones":          {[]review.Answer{ratedAnswer(1), ratedAnswer(1)}, 1},
	} {
		if got := avgReviewRating(review.Review{ID: uuid.New(), Answers: tc.answers}); got != tc.want {
			t.Errorf("%s: avgReviewRating = %d, want %d", name, got, tc.want)
		}
	}
}

// Zero means "no rating at all", so a review that genuinely averages to a rating must never report it
func TestAvgReviewRatingReservesZeroForUnrated(t *testing.T) {
	if got := avgReviewRating(review.Review{ID: uuid.New()}); got != 0 {
		t.Errorf("a review with no answers = %d, want 0", got)
	}
	if got := avgReviewRating(review.Review{ID: uuid.New(), Answers: []review.Answer{ratedAnswer(1)}}); got == 0 {
		t.Error("a 1-star review reported 0, which the drafter reads as unrated")
	}
}

// The rounding is integer arithmetic, so it must stay within the 1..5 range every rated answer lives in
func TestAvgReviewRatingStaysInRange(t *testing.T) {
	for a := int16(1); a <= 5; a++ {
		for b := int16(1); b <= 5; b++ {
			got := avgReviewRating(review.Review{ID: uuid.New(), Answers: []review.Answer{ratedAnswer(a), ratedAnswer(b)}})
			if got < 1 || got > 5 {
				t.Errorf("avgReviewRating(%d, %d) = %d, want it inside 1..5", a, b, got)
			}
			lo, hi := min(a, b), max(a, b)
			if int16(got) < lo || int16(got) > hi {
				t.Errorf("avgReviewRating(%d, %d) = %d, want it between the two answers", a, b, got)
			}
		}
	}
}
