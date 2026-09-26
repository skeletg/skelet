package db

import (
	"errors"
	"strings"
)

// Matched via SQLState so wrapped driver errors still classify
// The text fallback only reads the "(SQLSTATE nnnnn)" suffix a %v-flattened pgx error keeps
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgRestrictViolation   = "23001"
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
	return strings.Contains(err.Error(), "SQLSTATE "+pgUniqueViolation)
}

// IsForeignKeyViolation reports whether err is an FK refusal - 23503 from NO ACTION, 23001 from an ON DELETE RESTRICT the delete hit
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	if code := sqlState(err); code != "" {
		return code == pgForeignKeyViolation || code == pgRestrictViolation
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE "+pgForeignKeyViolation) || strings.Contains(msg, "SQLSTATE "+pgRestrictViolation)
}
