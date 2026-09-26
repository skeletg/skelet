package cms

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type fakeWorkflowStore struct {
	pageWorkflowStore
	page        Page
	comments    []string
	failComment error
	stateWrites int
}

func (f *fakeWorkflowStore) CreateComment(_ context.Context, in CommentInput, _ uuid.UUID) (PageComment, error) {
	if f.failComment != nil {
		return PageComment{}, f.failComment
	}
	f.comments = append(f.comments, in.Body)
	return PageComment{}, nil
}

func (f *fakeWorkflowStore) SetPageWorkflowState(_ context.Context, _ uuid.UUID, state WorkflowState) (Page, error) {
	f.page.WorkflowState = state
	f.stateWrites++
	return f.page, nil
}

func (f *fakeWorkflowStore) GetPageByID(context.Context, uuid.UUID) (Page, error) { return f.page, nil }

func adminCtx() context.Context {
	return auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New()})
}

func TestRejectPageRecordsTheReason(t *testing.T) {
	store := &fakeWorkflowStore{page: draftPage(WorkflowInReview)}
	ws := &pageWorkflowService{core: &core{}, repo: store}
	got, err := ws.RejectPage(adminCtx(), store.page.ID, "  needs a hero image ")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got.WorkflowState != WorkflowDraft {
		t.Errorf("state = %s, want draft", got.WorkflowState)
	}
	if len(store.comments) != 1 || store.comments[0] != "Rejected: needs a hero image" {
		t.Errorf("comments = %q", store.comments)
	}
}

func TestRejectPageFailsRatherThanLosingTheReason(t *testing.T) {
	store := &fakeWorkflowStore{page: draftPage(WorkflowInReview), failComment: errors.New("insert failed")}
	ws := &pageWorkflowService{core: &core{}, repo: store}
	if _, err := ws.RejectPage(adminCtx(), store.page.ID, "needs a hero image"); err == nil {
		t.Fatal("reject reported success though the reason was never saved")
	}
	if store.stateWrites != 0 || store.page.WorkflowState != WorkflowInReview {
		t.Error("the page went back to draft with no record of why - the editor would see a rejection and no reason")
	}
}

func TestRejectPageWithoutANoteWritesNoComment(t *testing.T) {
	store := &fakeWorkflowStore{page: draftPage(WorkflowInReview), failComment: errors.New("must not be called")}
	ws := &pageWorkflowService{core: &core{}, repo: store}
	if _, err := ws.RejectPage(adminCtx(), store.page.ID, "   "); err != nil {
		t.Fatalf("reject with a blank note: %v", err)
	}
	if store.page.WorkflowState != WorkflowDraft {
		t.Errorf("state = %s, want draft", store.page.WorkflowState)
	}
}
