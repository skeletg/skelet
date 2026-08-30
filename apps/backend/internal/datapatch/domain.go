// Package datapatch runs one-time, code-based data changes on deploy - the Go counterpart to golang-migrate for core schema and pluginmigrate for plugin schema
package datapatch

import (
	"context"
	"database/sql"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

type Context struct {
	Tx      *sql.Tx
	Queries *db.Queries
}

type Patch struct {
	ID        string // the marker stored in data_patches - never reuse or rename
	Namespace string
	DependsOn []string // patch IDs that must run first
	Apply     func(ctx context.Context, pc *Context) error
}
