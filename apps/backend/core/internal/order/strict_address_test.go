package order

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type scriptedChecker struct {
	strict  bool
	invalid map[string]bool // by Street1
	err     error
	calls   []string
}

func (c *scriptedChecker) Strict(context.Context, uuid.UUID) bool { return c.strict }

func (c *scriptedChecker) Validate(_ context.Context, _ uuid.UUID, in address.Input) (address.Result, error) {
	c.calls = append(c.calls, in.Street1)
	if c.err != nil {
		return address.Result{}, c.err
	}
	if c.invalid[in.Street1] {
		return address.Result{Valid: false, Messages: []string{"no such street"}}, nil
	}
	return address.Result{Valid: true}, nil
}

func addr(street string) OrderAddress {
	return OrderAddress{Street1: street, City: "Austin", PostalCode: "78701", CountryCode: "US"}
}

func TestStrictModeChecksShippingAndBilling(t *testing.T) {
	for name, tc := range map[string]struct {
		checker   scriptedChecker
		shipping  OrderAddress
		billing   OrderAddress
		wantCode  string
		wantWhich string
		wantCalls int
	}{
		"off skips the validator":                  {checker: scriptedChecker{invalid: map[string]bool{"bad": true}}, shipping: addr("bad"), billing: addr("bad"), wantCalls: 0},
		"both valid":                               {checker: scriptedChecker{strict: true}, shipping: addr("1 Ship St"), billing: addr("2 Bill St"), wantCalls: 2},
		"same address is checked once":             {checker: scriptedChecker{strict: true}, shipping: addr("1 Ship St"), billing: addr("1 Ship St"), wantCalls: 1},
		"invalid shipping refuses":                 {checker: scriptedChecker{strict: true, invalid: map[string]bool{"bad": true}}, shipping: addr("bad"), billing: addr("2 Bill St"), wantCode: "ADDRESS_INVALID", wantWhich: "shipping", wantCalls: 1},
		"invalid billing refuses":                  {checker: scriptedChecker{strict: true, invalid: map[string]bool{"bad": true}}, shipping: addr("1 Ship St"), billing: addr("bad"), wantCode: "ADDRESS_INVALID", wantWhich: "billing", wantCalls: 2},
		"ordinary validator error lets it through": {checker: scriptedChecker{strict: true, err: errors.New("config read failed")}, shipping: addr("1 Ship St"), billing: addr("2 Bill St"), wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			checker := tc.checker
			err := (&Service{addresses: &checker}).checkStrictAddresses(context.Background(), uuid.New(), tc.shipping, tc.billing)
			if len(checker.calls) != tc.wantCalls {
				t.Errorf("validator called %d times (%v), want %d", len(checker.calls), checker.calls, tc.wantCalls)
			}
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("err = %v, want the order through", err)
				}
				return
			}
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != tc.wantCode {
				t.Fatalf("err = %v, want %s", err, tc.wantCode)
			}
			if !strings.Contains(err.Error(), tc.wantWhich+" address") {
				t.Errorf("err = %q, want it to name the %s address", err, tc.wantWhich)
			}
		})
	}
}

func TestStrictModeRefusesWhenAFailClosedValidatorIsDown(t *testing.T) {
	checker := &scriptedChecker{strict: true, err: address.ErrUnavailable}
	err := (&Service{addresses: checker}).checkStrictAddresses(context.Background(), uuid.New(), addr("1 Ship St"), addr("2 Bill St"))
	if !errors.Is(err, address.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable so the order is refused with its code", err)
	}
	if code, class := apperr.CodeOf(err), apperr.ClassOf(err); code != "ADDRESS_VALIDATION_UNAVAILABLE" || class != apperr.ClassUnavailable {
		t.Errorf("err resolves to %s/%v, want ADDRESS_VALIDATION_UNAVAILABLE/unavailable", code, class)
	}
}
