package signalsgrpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The interceptor is non-blocking, so every handler refuses on its own - one that forgot would answer an anonymous caller, and this surface decides who may be mailed
// It is core's alone, so anyone but the system principal is refused outright
func TestEverySuppressionMethodRefusesAnyoneButCore(t *testing.T) {
	c := signalsv1.NewSuppressionServiceClient(testConn(t))
	ctx := context.Background()

	for name, call := range map[string]func() error{
		"IsSuppressed": func() error { _, e := c.IsSuppressed(ctx, &signalsv1.IsSuppressedRequest{Email: "a@b.c"}); return e },
		"ListSuppressed": func() error {
			_, e := c.ListSuppressed(ctx, &signalsv1.ListSuppressedRequest{Emails: []string{"a@b.c"}})
			return e
		},
		"Suppress": func() error { _, e := c.Suppress(ctx, &signalsv1.SuppressRequest{Email: "a@b.c"}); return e },
		"RecordSoftBounce": func() error {
			_, e := c.RecordSoftBounce(ctx, &signalsv1.RecordSoftBounceRequest{Email: "a@b.c"})
			return e
		},
		"Unsuppress":       func() error { _, e := c.Unsuppress(ctx, &signalsv1.UnsuppressRequest{Email: "a@b.c"}); return e },
		"ListSuppressions": func() error { _, e := c.ListSuppressions(ctx, &signalsv1.ListSuppressionsRequest{}); return e },
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s answered an anonymous caller with %s, want PermissionDenied", name, got)
		}
	}
}

// An admin's relayed grants are not enough either - this surface is core's to call as itself, the
// admin gate having run in core's own resolver
func TestSuppressionIsTheSystemPrincipals(t *testing.T) {
	c := signalsv1.NewSuppressionServiceClient(testConn(t))
	if got := status.Code(mustErr(c.ListSuppressions(tokenCtx(t), &signalsv1.ListSuppressionsRequest{}))); got != codes.PermissionDenied {
		t.Errorf("an admin token answered %s, want PermissionDenied", got)
	}
}

// A blank address is a caller bug, not a mailbox to suppress
func TestSuppressingNobodyIsInvalidArgument(t *testing.T) {
	c := signalsv1.NewSuppressionServiceClient(testConn(t))
	if got := status.Code(mustErr(c.Suppress(systemCtx(t), &signalsv1.SuppressRequest{Email: "  "}))); got != codes.InvalidArgument {
		t.Errorf("a blank address answered %s, want InvalidArgument", got)
	}
}
