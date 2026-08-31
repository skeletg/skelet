// Command media serves the image-transform endpoint as its own tier, away from
// the commerce engine cmd/server runs

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/applog"
	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/envcheck"
	"github.com/skeletg/skelet/apps/backend/internal/httpx"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/skeletg/skelet/apps/backend/internal/mediahttp"
	"github.com/skeletg/skelet/apps/backend/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/otelx"
)

const defaultPort = "8083"

func main() {
	if err := run(); err != nil {
		slog.Error("media tier exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	applog.Setup(os.Getenv("APP_ENV"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// It writes to the bucket and optionally opens a pool - no session tokens, so no JWT gate here
	if err := envcheck.Report(envcheck.Production(os.Getenv, envcheck.Database, envcheck.S3)); err != nil {
		return err
	}

	telemetry := observability.Init(ctx, os.Getenv("APP_ENV"))
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		if err := telemetry.Shutdown(sctx); err != nil {
			slog.Warn("telemetry shutdown", "err", err)
		}
	}()
	queryTracer := otelx.PGXTracer()

	port := os.Getenv("MEDIA_PORT")
	if port == "" {
		port = defaultPort
	}

	storage, err := media.New(ctx, media.Config{
		Bucket:           os.Getenv("S3_BUCKET_NAME"),
		Region:           os.Getenv("AWS_REGION"),
		InternalEndpoint: os.Getenv("S3_ENDPOINT_URL"),
		PresignEndpoint:  os.Getenv("S3_PRESIGN_ENDPOINT_URL"),
		PublicURLBase:    os.Getenv("S3_PUBLIC_URL_BASE"),
		UsePathStyle:     os.Getenv("S3_USE_PATH_STYLE") == "true",
	})
	if err != nil {
		return fmt.Errorf("media storage: %w", err)
	}
	if !storage.HasPublicBase() {
		// Not fatal, but warm hits then get proxied instead of redirected - which is most of the point
		slog.Warn("S3_PUBLIC_URL_BASE is unset: warm derivatives will be proxied, not redirected")
	}

	transformer := media.NewTransformer(storage)
	transformer.SetMaxSourcePixels(envInt("MEDIA_MAX_SOURCE_PIXELS", 0))
	transformer.SetRenderConcurrency(envInt("MEDIA_RENDER_SLOTS", maxProcs()), 0)

	opts := mediahttp.Options{}

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		conn, err := dbconn.Connect(dbURL, dbconn.WithQueryTracer(queryTracer), dbconn.WithMaxConns(envInt("MEDIA_DB_MAX_CONNS", 2)))
		if err != nil {
			return fmt.Errorf("connect to database: %w", err)
		}
		defer conn.Close()
		lib := medialibrary.NewService(medialibrary.NewRepository(db.New(conn)), storage, os.Getenv("MEDIA_LIBRARY_ROOT_PREFIX"))
		opts.Focal = func(r *http.Request, key string) (float64, float64, bool) {
			asset, found := lib.AssetByObjectKey(r.Context(), key)
			if !found || asset.FocalX == nil || asset.FocalY == nil {
				return 0, 0, false
			}
			return *asset.FocalX, *asset.FocalY, true
		}
	} else {
		slog.Warn("DATABASE_URL is unset: cover crops will centre instead of using focal points")
	}

	mux := http.NewServeMux()
	mux.Handle("/media/t/", otelx.WrapHTTP(mediahttp.NewTransformHandler(transformer, opts), "media.transform"))
	mux.Handle("/healthz", mediahttp.Health())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httpx.Recover(httpx.RequestID(mux)), // panic -> structured 500 with request_id instead of an aborted connection
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute, // transforms stream large originals in and derivatives out
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(l net.Listener) context.Context { return ctx },
	}

	slog.Info("media transform tier", "url", fmt.Sprintf("http://localhost:%s/media/t/", port))

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received; draining media tier")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http listen: %w", err)
		}
	}

	// Nothing is stateful beyond the best-effort cache-back, so a render cut at the deadline just regenerates next time
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("http server shutdown", "err", err)
	}
	slog.Info("graceful shutdown complete")
	return nil
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func maxProcs() int { return runtime.GOMAXPROCS(0) }
