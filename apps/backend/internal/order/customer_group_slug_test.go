package order

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/customer"
)

// A nil Group pointer is the realistic shape from CLI and test paths that skip GetByIDWithAddresses' group hydration - returning the zero string lets the engine simply not match group-gated rules instead of panicking
func TestCustomerGroupSlug(t *testing.T) {
	tests := []struct {
		name string
		c    customer.Customer
		want string
	}{
		{
			name: "nil group → empty",
			c:    customer.Customer{ID: uuid.New()},
			want: "",
		},
		{
			name: "populated group → slug",
			c: customer.Customer{
				ID:    uuid.New(),
				Group: &customer.CustomerGroup{Slug: "vip"},
			},
			want: "vip",
		},
		{
			name: "empty slug on populated group → empty",
			c: customer.Customer{
				ID:    uuid.New(),
				Group: &customer.CustomerGroup{Slug: ""},
			},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := customerGroupSlug(tc.c); got != tc.want {
				t.Errorf("customerGroupSlug = %q, want %q", got, tc.want)
			}
		})
	}
}
