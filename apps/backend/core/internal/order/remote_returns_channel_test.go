package order

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func TestAChannelsOwnReturnsPolicyIsTheOneAsked(t *testing.T) {
	strict := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"allowed": false, "reason": "final sale"}`))
	}))
	defer strict.Close()
	ch := uuid.New()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindReturnsPolicy, Slug: "strict", Name: "Strict", TargetURL: strict.URL, FailurePolicy: remoteprovider.FailClosed, ChannelID: &ch,
	}); err != nil {
		t.Fatal(err)
	}
	policy := NewRemoteReturnsPolicy(svc, remoteprovider.NewInvoker(svc, nil))

	in := returnInput()
	in.ChannelID = ch
	if allowed, reason := policy.Allowed(context.Background(), in); allowed || reason != "final sale" {
		t.Fatalf("an order on the channel = %v %q, want its stricter policy to refuse", allowed, reason)
	}
	in.ChannelID = uuid.New()
	if allowed, _ := policy.Allowed(context.Background(), in); !allowed {
		t.Fatal("another channel's order was held to a policy scoped elsewhere")
	}
}
