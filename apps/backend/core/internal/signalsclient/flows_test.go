package signalsclient

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEnrollReadsOnlyARefusalAsADefiniteNo(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{status.Error(codes.InvalidArgument, "bad"), true},
		{status.Error(codes.PermissionDenied, "no"), true},
		{status.Error(codes.Internal, "db down"), false},
		{status.Error(codes.Unavailable, "gone"), false},
		{errors.New("plain"), false},
	} {
		if got := refused(c.err); got != c.want {
			t.Errorf("refused(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
