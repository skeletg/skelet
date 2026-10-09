package db

import (
	"errors"
	"testing"
)

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

func TestAFlattenedDriverErrorStillClassifies(t *testing.T) {
	flat := errors.New(`save: ERROR: duplicate key value violates unique constraint "x_key" (SQLSTATE 23505)`)
	if !IsUniqueViolation(flat) {
		t.Error("a pgx error flattened into plain text lost its classification")
	}
	if !IsForeignKeyViolation(errors.New("delete: ERROR: update or delete violates foreign key constraint (SQLSTATE 23503)")) {
		t.Error("a flattened FK refusal lost its classification")
	}
}

func TestAMessageThatOnlyMentionsAConstraintIsNotOne(t *testing.T) {
	for _, msg := range []string{"slug must satisfy the unique constraint on its parent", "duplicate key name in import row 4", "the foreign key constraint editor is read-only"} {
		if IsUniqueViolation(errors.New(msg)) || IsForeignKeyViolation(errors.New(msg)) {
			t.Errorf("%q was classified as a database violation", msg)
		}
	}
}

func TestAnOverflowingValueIsStringTooLong(t *testing.T) {
	for code, want := range map[string]bool{"22001": true, "23503": false} {
		if got := IsStringTooLong(stateErr(code)); got != want {
			t.Errorf("IsStringTooLong(%s) = %v, want %v", code, got, want)
		}
	}
	if !IsStringTooLong(errors.New("ERROR: value too long for type character(2) (SQLSTATE 22001)")) {
		t.Error("a flattened overflow lost its classification")
	}
}
