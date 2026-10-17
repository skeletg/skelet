package admin

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

// A typo'd segment id used to be dropped silently, which cleared the form's segment on save
func TestAMalformedTargetSegmentIsRefused(t *testing.T) {
	bad := "not-a-uuid"
	if _, err := saveInputToForm(model.SaveSignupFormInput{TargetSegmentID: &bad, Display: &model.SignupFormDisplayInput{}}, uuid.New()); err == nil {
		t.Fatal("a malformed segment id was accepted")
	}
}
