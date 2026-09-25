package front

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAStorelessRequestSeesNoQuestions(t *testing.T) {
	got, err := (&queryResolver{&Resolver{}}).ProductQuestions(context.Background(), uuid.NewString(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("productQuestions = %d, %v, want none without a store", len(got), err)
	}
}
