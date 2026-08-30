package customer

import (
	"errors"
	"testing"
)

// The (channel, email) unique index firing on REGISTRATION must surface as ErrEmailTaken - the
// update paths always mapped it, the create path didn't, and every reused email reached the
// storefront as a bare INTERNAL instead of "email already registered"
func TestRegisterDuplicateEmailIsErrEmailTaken(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	email := randomEmail()
	f.register(t, ch, email, "password123")

	_, err := f.svc.Register(f.ctx, ch, email, "password123", "Du", "Pe", nil)
	if !errors.Is(err, ErrEmailTaken) {
		t.Errorf("duplicate register: got %v, want ErrEmailTaken", err)
	}
}
