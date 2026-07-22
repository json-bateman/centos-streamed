// Command web is the user-facing server for the centos-streamed platform.
//
// It is a Datastar-driven webserver built on the same stack as the basicauth
// reference project — chi, goose, sqlc, templ and an embedded NATS bus — minus
// any authentication. It renders live host information and a shared message
// board, both pushed to the browser over server-sent events.
//
// It runs as a Quadlet-managed Podman container (streamed.container) on the
// shared "proxy" network. Caddy terminates TLS and reverse-proxies to it by
// container name (streamed:8080), so this server itself only speaks plain HTTP on the
// internal network and is never published directly to the host.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	streamed "github.com/json-bateman/centos-streamed"
	"github.com/json-bateman/centos-streamed/sql"
	"github.com/json-bateman/centos-streamed/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// CTRL+C sends SIGINT; `kill` and container stop send SIGTERM. Both trigger
	// a graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	cfg := streamed.LoadSettings()

	db, err := sql.NewDatabase(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := web.RunBlocking(ctx, db); err != nil {
		return fmt.Errorf("run web server: %w", err)
	}
	return nil
}
