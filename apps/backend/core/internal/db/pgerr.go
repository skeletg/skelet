package db

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Matched via SQLState so wrapped driver errors still classify
// The text fallback only reads the "(SQLSTATE nnnnn)" suffix a %v-flattened pgx error keeps
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgRestrictViolation   = "23001"
	pgStringTooLong       = "22001"
)

func hasState(err error, codes ...string) bool {
	if err == nil {
		return false
	}
	var pgErr interface{ SQLState() string } // pgx wraps the server error - the code is only reachable via unwrap
	if errors.As(err, &pgErr) {
		for _, code := range codes {
			if pgErr.SQLState() == code {
				return true
			}
		}
		return false
	}
	msg := err.Error()
	for _, code := range codes {
		if strings.Contains(msg, "(SQLSTATE "+code+")") {
			return true
		}
	}
	return false
}

// IsUniqueViolation reports whether err is a duplicate-key error (SQLSTATE 23505) - the shape ON CONFLICT-less inserts and unique indexes surface as
func IsUniqueViolation(err error) bool { return hasState(err, pgUniqueViolation) }

// IsForeignKeyViolation reports whether err is an FK refusal - 23503 from NO ACTION, 23001 from an ON DELETE RESTRICT the delete hit
func IsForeignKeyViolation(err error) bool {
	return hasState(err, pgForeignKeyViolation, pgRestrictViolation)
}

// IsStringTooLong reports whether a value overflowed a bounded column such as char(2) or varchar(n) - SQLSTATE 22001
func IsStringTooLong(err error) bool { return hasState(err, pgStringTooLong) }

// ConstraintName is the constraint or unique index a violation names, "" when err carries none
func ConstraintName(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.ConstraintName
	}
	return ""
}
