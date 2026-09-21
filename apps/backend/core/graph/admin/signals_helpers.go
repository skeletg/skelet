package admin

// Shared plumbing for the resolvers that read the Signals suite - core answers the operation, the
// suite answers the data

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// signalsCtx attaches the caller's own token, so the suite refuses what they could not read themselves
// Every Signals-backed resolver reaches the suite through here, which is what carries the switch
func signalsCtx(ctx context.Context, r *Resolver) (context.Context, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err // switched off is a different answer from unreachable, and the operator gets the one that is true
	}
	authed, err := r.SignalsAPI.Authed(ctx)
	if err != nil {
		return nil, signalsErr(err)
	}
	return authed, nil
}

// A gRPC status is the suite answering, and its code carries the class the dashboard renders on
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

// The permission gate core can make on its own - the suite gates again on the channel the data belongs to
func (r *Resolver) signalsScoped(ctx context.Context, perm string) (context.Context, error) {
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), perm); err != nil {
		return nil, err
	}
	return signalsCtx(ctx, r)
}

// Adds the channel gate too, where the operation names one
func (r *Resolver) signalsChannel(ctx context.Context, perm, channelID string) (context.Context, error) {
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), perm); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, auth.GrantCheckerFrom(ctx), perm, cid, nil); err != nil {
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
