package signalsgrpc

// The suppression list over the contract - every method takes core's system token
// The send loops, the bounce webhook and the admin screens all call as core itself, the admin gate having run in core's own resolver

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/suppression"
)

type suppressionServer struct {
	signalsv1.UnimplementedSuppressionServiceServer
	svc *suppression.Service
}

func (s *suppressionServer) IsSuppressed(ctx context.Context, req *signalsv1.IsSuppressedRequest) (*signalsv1.IsSuppressedResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	return &signalsv1.IsSuppressedResponse{Suppressed: s.svc.IsSuppressed(ctx, req.GetEmail())}, nil
}

// Answers with the normalized forms, so the caller maps them back onto whatever it passed in
func (s *suppressionServer) ListSuppressed(ctx context.Context, req *signalsv1.ListSuppressedRequest) (*signalsv1.ListSuppressedResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	set := s.svc.SuppressedSet(ctx, req.GetEmails())
	out := make([]string, 0, len(set))
	seen := map[string]struct{}{}
	for email, suppressed := range set {
		n := suppression.Normalize(email)
		if _, dup := seen[n]; !suppressed || dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return &signalsv1.ListSuppressedResponse{Emails: out}, nil
}

func (s *suppressionServer) Suppress(ctx context.Context, req *signalsv1.SuppressRequest) (*signalsv1.SuppressResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if suppression.Normalize(req.GetEmail()) == "" {
		return nil, invalidArgument("email is required")
	}
	if err := s.svc.Suppress(ctx, req.GetEmail(), req.GetReason(), req.GetDetail()); err != nil {
		return nil, err
	}
	return &signalsv1.SuppressResponse{}, nil
}

func (s *suppressionServer) RecordSoftBounce(ctx context.Context, req *signalsv1.RecordSoftBounceRequest) (*signalsv1.RecordSoftBounceResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if suppression.Normalize(req.GetEmail()) == "" {
		return nil, invalidArgument("email is required")
	}
	suppressed, n, err := s.svc.RecordSoftBounce(ctx, req.GetEmail(), req.GetDetail(), int(req.GetThreshold()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordSoftBounceResponse{Suppressed: suppressed, SoftBounces: int32(n)}, nil
}

func (s *suppressionServer) Unsuppress(ctx context.Context, req *signalsv1.UnsuppressRequest) (*signalsv1.UnsuppressResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if err := s.svc.Unsuppress(ctx, req.GetEmail()); err != nil {
		return nil, err
	}
	return &signalsv1.UnsuppressResponse{}, nil
}

func (s *suppressionServer) ListSuppressions(ctx context.Context, req *signalsv1.ListSuppressionsRequest) (*signalsv1.ListSuppressionsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	rows, total, err := s.svc.List(ctx, req.GetReason(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.Suppression, 0, len(rows))
	for _, row := range rows {
		out = append(out, &signalsv1.Suppression{
			Id: row.ID.String(), Email: row.Email, Reason: row.Reason, Detail: row.Detail,
			SoftBounces: row.SoftBounces,
			CreatedAt:   timestamppb.New(row.CreatedAt), UpdatedAt: timestamppb.New(row.UpdatedAt),
		})
	}
	return &signalsv1.ListSuppressionsResponse{Suppressions: out, Total: int32(total)}, nil
}
