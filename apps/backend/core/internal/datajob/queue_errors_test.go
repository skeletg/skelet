package datajob

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// The HTTP layer echoes a request's own mistakes and hides the platform's, so the two have to be told apart here
func TestAStorageFailureIsClassedInternal(t *testing.T) {
	reg := dataio.NewRegistry()
	reg.Register(dataio.Resource{
		Slug: "things", Label: "Things", Group: "Test", ViewPermission: "test.view", ImportPermission: "test.manage",
		Fields:      []dataio.Field{{Name: "sku", Type: dataio.FieldText, Identifier: true, Required: true}},
		NewImporter: func(context.Context, dataio.Opts) (dataio.Importer, error) { return nil, nil },
	})
	blobs := newFakeBlobs()
	blobs.failUpload = errors.New("bucket at s3.internal:9000 refused")
	svc := New(nil, reg, blobs)

	_, err := svc.QueueImport(context.Background(), ImportRequest{Resource: "things", Format: dataio.FormatCSV}, strings.NewReader("sku\nA\n"))
	if apperr.ClassOf(err) != apperr.ClassInternal {
		t.Errorf("a storage failure = %v (class %v), want internal", err, apperr.ClassOf(err))
	}
	_, err = svc.QueueImport(context.Background(), ImportRequest{Resource: "things", Format: "xlsx-ish"}, strings.NewReader(""))
	if err == nil || apperr.ClassOf(err) == apperr.ClassInternal {
		t.Errorf("a bad format = %v, want a request error the uploader can read", err)
	}
}
