package admin

// Shared plumbing for the resolvers that read the Signals suite. Core owns the whole dashboard
// surface, so these answer the operation and the suite answers the data

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/signalsclient"
)

// signalsCtx attaches the caller's own token, so the suite refuses what they could not read themselves
func signalsCtx(ctx context.Context, r *Resolver) (context.Context, error) {
	authed, err := r.SignalsAPI.Authed(ctx)
	if err != nil {
		return nil, signalsErr(err)
	}
	return authed, nil
}

// A gRPC status is the suite's answer, and it carries the class the dashboard needs to tell a bad
// request from a service that is simply down
func signalsErr(err error) error {
	if err == nil {
		return nil
	}
	if signalsclient.Unreachable(err) {
		return apperr.New("SIGNALS_UNAVAILABLE", apperr.ClassUnavailable, "the Signals service is unavailable")
	}
	st := status.Convert(err)
	class := apperr.ClassInternal
	switch st.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		class = apperr.ClassValidation
	case codes.NotFound:
		class = apperr.ClassNotFound
	case codes.PermissionDenied:
		class = apperr.ClassForbidden
	case codes.Unauthenticated:
		class = apperr.ClassAuth
	case codes.AlreadyExists, codes.Aborted:
		class = apperr.ClassConflict
	case codes.ResourceExhausted:
		class = apperr.ClassRateLimited
	}
	return apperr.New("SIGNALS_"+st.Code().String(), class, st.Message())
}

// signalsScoped gates on the permission here, then hands the caller's token to the suite, which gates again on the channel the data actually belongs to
func (r *Resolver) signalsScoped(ctx context.Context, perm string) (context.Context, error) {
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), perm); err != nil {
		return nil, err
	}
	return signalsCtx(ctx, r)
}

func signalsNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

// The window an operation asked for, or 0 to let the suite apply its own default
func signalsDays(sinceDays *int32) int32 {
	if sinceDays != nil && *sinceDays > 0 {
		return *sinceDays
	}
	return 0
}
