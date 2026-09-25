package db

import "testing"

type stateErr string

func (e stateErr) Error() string    { return "db error " + string(e) }
func (e stateErr) SQLState() string { return string(e) }

func TestARestrictRefusalIsAForeignKeyViolation(t *testing.T) {
	for code, want := range map[string]bool{"23503": true, "23001": true, "23505": false} {
		if got := IsForeignKeyViolation(stateErr(code)); got != want {
			t.Errorf("IsForeignKeyViolation(%s) = %v, want %v", code, got, want)
		}
	}
}
