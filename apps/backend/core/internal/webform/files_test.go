package webform

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Every key exists, so only the fence can turn a forged key away
type anyObjectStore struct{ heads []string }

func (s *anyObjectStore) PresignUpload(context.Context, string, string, int64, time.Duration) (string, error) {
	return "", errors.New("unused")
}
func (s *anyObjectStore) Head(_ context.Context, key string) (int64, string, error) {
	s.heads = append(s.heads, key)
	return 1234, "application/pdf", nil
}
func (s *anyObjectStore) MaxUploadBytes() int64 { return 1 << 20 }

func TestSubmittedFilesMustBeThisFieldsOwnUploads(t *testing.T) {
	formID, otherForm := uuid.New(), uuid.New()
	fields := []Field{{Key: "cv", Type: FieldFile}, {Key: "cover", Type: FieldFile}}
	own := uploadPrefix(formID, "cv") + "abc-cv.pdf"

	for name, tc := range map[string]struct {
		key  string
		keep bool
	}{
		"its own upload":              {own, true},
		"another form's upload":       {uploadPrefix(otherForm, "cv") + "abc-cv.pdf", false},
		"another field's upload":      {uploadPrefix(formID, "cover") + "abc-cv.pdf", false},
		"anything else in the bucket": {"product-options/secret.pdf", false},
		"a prefix-lookalike":          {"webforms/" + formID.String() + "/cvx/abc.pdf", false},
	} {
		t.Run(name, func(t *testing.T) {
			store := &anyObjectStore{}
			s := &Service{files: store}
			kept, errs := s.validateFiles(context.Background(), formID, fields, []SubmissionFile{{FieldKey: "cv", ObjectKey: tc.key, Name: "cv.pdf", Size: 1, MimeType: "text/plain"}})
			if tc.keep {
				if len(errs) != 0 || len(kept) != 1 {
					t.Fatalf("own upload refused: kept=%v errs=%v", kept, errs)
				}
				if kept[0].Size != 1234 || kept[0].MimeType != "application/pdf" {
					t.Errorf("kept %+v, want size and type read from the store", kept[0])
				}
				return
			}
			if len(kept) != 0 || len(errs) != 1 || errs[0].Field != "cv" {
				t.Fatalf("forged key kept: kept=%v errs=%v", kept, errs)
			}
			if len(store.heads) != 0 {
				t.Errorf("forged key was looked up in storage: %v", store.heads)
			}
		})
	}
}
