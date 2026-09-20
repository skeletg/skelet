package db

import (
	"errors"
	"strings"
)

// The two Postgres error codes worth branching on - matched via SQLState so wrapped driver errors still classify, with a message fallback for drivers that only put the code in text
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

func sqlState(err error) string {
	var pgErr interface{ SQLState() string } // pgx wraps the server error - the code is only reachable via unwrap
	if errors.As(err, &pgErr) {
		return pgErr.SQLState()
	}
	return ""
}

// IsUniqueViolation reports whether err is a duplicate-key error (SQLSTATE 23505) - the shape ON CONFLICT-less inserts and unique indexes surface as
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if code := sqlState(err); code != "" {
		return code == pgUniqueViolation
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23505") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

// IsForeignKeyViolation reports whether err is an FK error (SQLSTATE 23503) - what a RESTRICT delete hits when rows still reference the target
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	if code := sqlState(err); code != "" {
		return code == pgForeignKeyViolation
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23503") || strings.Contains(msg, "foreign key constraint")
}
