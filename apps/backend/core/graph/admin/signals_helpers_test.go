package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func TestSignalsErrCodes(t *testing.T) {
	cases := []struct {
		err   error
		code  string
		class apperr.Class
	}{
		{status.Error(codes.NotFound, "x"), "SIGNALS_NOT_FOUND", apperr.ClassNotFound},
		{status.Error(codes.InvalidArgument, "x"), "SIGNALS_INVALID_ARGUMENT", apperr.ClassValidation},
		{status.Error(codes.PermissionDenied, "x"), "SIGNALS_PERMISSION_DENIED", apperr.ClassForbidden},
		{status.Error(codes.Internal, "x"), "SIGNALS_INTERNAL", apperr.ClassInternal},
		{status.Error(codes.Unavailable, "dial tcp"), "SIGNALS_UNAVAILABLE", apperr.ClassUnavailable},
		{status.Error(codes.DeadlineExceeded, "x"), "SIGNALS_UNAVAILABLE", apperr.ClassUnavailable},
	}
	for _, c := range cases {
		got := signalsErr(c.err)
		if apperr.CodeOf(got) != c.code || apperr.ClassOf(got) != c.class {
			t.Errorf("%v: got %s/%s, want %s/%s", c.err, apperr.CodeOf(got), apperr.ClassOf(got), c.code, c.class)
		}
	}
	plain := errors.New("not a status")
	if got := signalsErr(plain); !errors.Is(got, plain) {
		t.Errorf("a non-status error must pass through, got %v", got)
	}
	if signalsErr(nil) != nil {
		t.Error("nil must stay nil")
	}
}

func TestMarkSignalsRelayed(t *testing.T) {
	markSignalsRelayed(context.Background()) // outside a resolver it must not panic

	ctx := graphql.WithResponseContext(context.Background(), graphql.DefaultErrorPresenter, nil)
	ctx = graphql.WithFieldContext(ctx, &graphql.FieldContext{})
	markSignalsRelayed(ctx)
	markSignalsRelayed(ctx)
	if v := graphql.GetExtensions(ctx)[signalsRelayedExtension]; v != true {
		t.Fatalf("extension = %v, want true", v)
	}
}
