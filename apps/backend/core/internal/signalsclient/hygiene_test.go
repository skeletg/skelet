package signalsclient

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

type loopingSegments struct {
	signalsv1.SegmentServiceClient
	calls int
}

func (l *loopingSegments) ListMembers(context.Context, *signalsv1.ListMembersRequest, ...grpc.CallOption) (*signalsv1.ListMembersResponse, error) {
	l.calls++
	return &signalsv1.ListMembersResponse{NextPageToken: "same"}, nil
}

func TestSegmentMembersStopOnARepeatedPageToken(t *testing.T) {
	seg := &loopingSegments{}
	c := &Client{keys: auth.MustServiceKeys("s"), ttl: time.Minute, Segments: seg}
	if _, err := c.SegmentMemberIDs(context.Background(), uuid.New()); err == nil {
		t.Fatal("a server repeating its page token was paged forever")
	}
	if seg.calls != 2 {
		t.Errorf("asked %d pages, want 2", seg.calls)
	}
}

type recordingSuppressions struct {
	signalsv1.SuppressionServiceClient
	emails []string
}

func (r *recordingSuppressions) IsSuppressed(_ context.Context, in *signalsv1.IsSuppressedRequest, _ ...grpc.CallOption) (*signalsv1.IsSuppressedResponse, error) {
	r.emails = append(r.emails, in.GetEmail())
	return &signalsv1.IsSuppressedResponse{}, nil
}

func (r *recordingSuppressions) Suppress(_ context.Context, in *signalsv1.SuppressRequest, _ ...grpc.CallOption) (*signalsv1.SuppressResponse, error) {
	r.emails = append(r.emails, in.GetEmail())
	return &signalsv1.SuppressResponse{}, nil
}

func (r *recordingSuppressions) RecordSoftBounce(_ context.Context, in *signalsv1.RecordSoftBounceRequest, _ ...grpc.CallOption) (*signalsv1.RecordSoftBounceResponse, error) {
	r.emails = append(r.emails, in.GetEmail())
	return &signalsv1.RecordSoftBounceResponse{}, nil
}

func (r *recordingSuppressions) Unsuppress(_ context.Context, in *signalsv1.UnsuppressRequest, _ ...grpc.CallOption) (*signalsv1.UnsuppressResponse, error) {
	r.emails = append(r.emails, in.GetEmail())
	return &signalsv1.UnsuppressResponse{}, nil
}

func TestSuppressionCallsSendTheNormalizedAddress(t *testing.T) {
	rec := &recordingSuppressions{}
	c := &Client{keys: auth.MustServiceKeys("s"), ttl: time.Minute, Suppressions: rec}
	ctx := context.Background()
	const raw = "  Ann@Example.COM "
	c.IsSuppressed(ctx, raw)
	_ = c.Suppress(ctx, raw, SuppressionManual, "")
	_, _, _ = c.RecordSoftBounce(ctx, raw, "", 3)
	_ = c.Unsuppress(ctx, raw)
	if len(rec.emails) != 4 {
		t.Fatalf("got %d calls, want 4", len(rec.emails))
	}
	for i, e := range rec.emails {
		if e != "ann@example.com" {
			t.Errorf("call %d sent %q, want ann@example.com", i, e)
		}
	}
}
