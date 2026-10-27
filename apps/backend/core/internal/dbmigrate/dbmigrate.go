// Package dbmigrate brings the commerce database up to date - schema, plugin migrations, then data patches - once the plugin host is installed
package dbmigrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skeletg/skelet/apps/backend/core/internal/datapatch"
	_ "github.com/skeletg/skelet/apps/backend/core/internal/datapatch/patches"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/migrations"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/pluginmigrate"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/migrate"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func Up(ctx context.Context, dsn string) error {
	conn, err := dbconn.Connect(dsn, dbconn.WithMaxConns(2))
	if err != nil {
		return fmt.Errorf("connect to migrate: %w", err)
	}
	defer conn.Close()

	applied, err := migrate.Postgres(ctx, conn, "commerce", migrations.FS)
	if len(applied) > 0 {
		slog.Info("commerce schema migrated", "applied", applied)
	}
	if err != nil {
		return err
	}
	if err := pluginmigrate.New(conn).UpInOrder(ctx, pluginhost.OrderedManifestSlugs()); err != nil {
		return fmt.Errorf("plugin migrations: %w", err)
	}
	res, err := datapatch.Run(ctx, db.New(conn), txrunner.New(conn))
	if len(res.Applied) > 0 {
		slog.Info("data patches applied", "applied", res.Applied)
	}
	return err
}
