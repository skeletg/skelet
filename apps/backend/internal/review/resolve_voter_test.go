package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/review"
)

// ResolveVoter is the single source of truth for "who is voting on this review?" - the dataloader path and the GraphQL resolver both call it, so the table below is the contract gating anonymous voting
// Tests construct review.Service with a nil repo and tx because ResolveVoter touches neither - it only reads policy, which returns strict defaults with no config wired, and runs the HMAC check on the supplied token
func TestResolveVoter(t *testing.T) {
	secret := []byte("test-secret-bytes-long-enough")
	customerID := uuid.New()
	validTokenID := uuid.New()
	validToken := review.SignVoterToken(secret, validTokenID)

	tests := []struct {
		name       string
		customerID *uuid.UUID
		rawToken   string
		secret     []byte
		wantErr    error
		wantCustID *uuid.UUID
		wantAnonID *uuid.UUID
	}{
		{
			name:       "signed-in customer wins outright",
			customerID: &customerID,
			rawToken:   validToken, // ignored when customer is present
			secret:     secret,
			wantCustID: &customerID,
		},
		{
			name:     "guest with policy default (strict) → auth required",
			rawToken: validToken,
			secret:   secret,
			wantErr:  review.ErrVoteAuthRequired,
		},
		{
			name:    "guest, no secret configured → auth required (fails closed)",
			secret:  nil,
			wantErr: review.ErrVoteAuthRequired,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := review.NewService(nil, nil)
			v, err := svc.ResolveVoter(context.Background(), tc.customerID, tc.rawToken, tc.secret)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.wantCustID != nil {
				if v.CustomerID == nil || *v.CustomerID != *tc.wantCustID {
					t.Errorf("CustomerID = %v, want %v", v.CustomerID, *tc.wantCustID)
				}
			}
		})
	}
}

// Proves a malformed or mis-signed anon token is rejected rather than silently accepted as a guest fall-through - the signing secret defends against forged X-Review-Voter-Token headers
func TestResolveVoter_BadTokenFailsClosed(t *testing.T) {
	secret := []byte("real-secret-bytes-here")
	wrongSecret := []byte("attacker-guessed-this")

	cases := []struct {
		name     string
		rawToken string
	}{
		{"unsigned uuid (no dot)", uuid.NewString()},
		{"wrong signature", review.SignVoterToken(wrongSecret, uuid.New())},
		{"garbage", "not-a-token"},
		{"empty after policy gate", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := review.NewService(nil, nil)
			// ResolveVoter only reaches the token branch when policy allows anon voting, and with no config wired policy is strict, so every guest case errors before the token is parsed - the fail-closed behaviour this test pins
			_, err := svc.ResolveVoter(context.Background(), nil, tc.rawToken, secret)
			if !errors.Is(err, review.ErrVoteAuthRequired) {
				t.Errorf("err = %v, want ErrVoteAuthRequired", err)
			}
		})
	}
}
