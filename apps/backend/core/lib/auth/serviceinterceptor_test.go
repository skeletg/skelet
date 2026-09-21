package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

type fakeRevocations struct{ killed map[uuid.UUID]bool }

func (f fakeRevocations) Revoke(_ context.Context, id uuid.UUID)       { f.killed[id] = true }
func (f fakeRevocations) Revoked(_ context.Context, id uuid.UUID) bool { return f.killed[id] }

func incoming(t *testing.T, claims ServiceClaims) context.Context {
	t.Helper()
	tok, err := GenerateServiceToken(claims, "signals", svcSecret, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tok))
}

// A token still verifies after its session was killed, so the interceptor is the one place that can refuse it
func TestServiceContextDropsARevokedSession(t *testing.T) {
	session := uuid.New()
	list := fakeRevocations{killed: map[uuid.UUID]bool{}}
	claims := ServiceClaims{AdminID: uuid.New(), SessionID: session, Email: "a@b.c", IsSuperuser: true}

	if _, ok := AdminFromContext(serviceContext(incoming(t, claims), "signals", svcSecret, list)); !ok {
		t.Fatal("a live session was not admitted")
	}
	list.Revoke(context.Background(), session)
	if _, ok := AdminFromContext(serviceContext(incoming(t, claims), "signals", svcSecret, list)); ok {
		t.Fatal("a revoked session was admitted")
	}
}

// Sessionless tokens carry nothing to revoke and pass through, and a nil list leaves every token as it verified
func TestServiceContextRevocationOnlyAppliesToSessions(t *testing.T) {
	list := fakeRevocations{killed: map[uuid.UUID]bool{uuid.New(): true}}
	background := ServiceClaims{AdminID: uuid.New(), Email: "a@b.c", IsSuperuser: true}
	if _, ok := AdminFromContext(serviceContext(incoming(t, background), "signals", svcSecret, list)); !ok {
		t.Error("a token with no session id was refused")
	}
	if !IsSystemCaller(serviceContext(incoming(t, ServiceClaims{System: true}), "signals", svcSecret, list)) {
		t.Error("the system token was refused")
	}
	withSession := ServiceClaims{AdminID: uuid.New(), SessionID: uuid.New(), Email: "a@b.c", IsSuperuser: true}
	if _, ok := AdminFromContext(serviceContext(incoming(t, withSession), "signals", svcSecret, nil)); !ok {
		t.Error("a nil list refused a token")
	}
}
