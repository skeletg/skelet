package ptr

import (
	"testing"

	"github.com/google/uuid"
)

// The nil case is the whole reason this exists - every caller is a nullable GraphQL argument or column,
// and the zero value is what "not supplied" has to read as
func TestDerefGivesTheZeroValueForNil(t *testing.T) {
	if got := Deref[string](nil); got != "" {
		t.Errorf("Deref[string](nil) = %q, want empty", got)
	}
	if got := Deref[int32](nil); got != 0 {
		t.Errorf("Deref[int32](nil) = %d, want 0", got)
	}
	if got := Deref[bool](nil); got {
		t.Error("Deref[bool](nil) = true, want false")
	}
	if got := Deref[uuid.UUID](nil); got != uuid.Nil {
		t.Errorf("Deref[uuid.UUID](nil) = %v, want the nil uuid", got)
	}
}

func TestDerefReadsThroughThePointer(t *testing.T) {
	s := "value"
	if got := Deref(&s); got != "value" {
		t.Errorf("Deref(&s) = %q, want %q", got, s)
	}
	// An explicit zero must survive - collapsing it to the zero value would be indistinguishable from nil,
	// which is exactly what callers use the pointer to tell apart
	empty, no := "", false
	if got := Deref(&empty); got != "" {
		t.Errorf("Deref(&\"\") = %q, want empty", got)
	}
	if got := Deref(&no); got {
		t.Error("Deref(&false) = true")
	}
	id := uuid.New()
	if got := Deref(&id); got != id {
		t.Errorf("Deref(&id) = %v, want %v", got, id)
	}
}
