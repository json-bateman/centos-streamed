package web

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"log/slog"
	"net/http"

	"github.com/benbjohnson/hashfs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	streamed "github.com/json-bateman/centos-streamed"
	"github.com/nats-io/nats.go"
)

//go:embed static/*
var StaticFS embed.FS

// StaticSys serves embedded static files under content-hashed names so they can
// be cached forever; when a file changes its hash changes and busts the cache.
var StaticSys = hashfs.NewFS(StaticFS)

const (
	HomeUrl     = "/"
	SseUrl      = "/sse"
	MessagesUrl = "/messages"

	// messagesSubject is the NATS subject published whenever the message board
	// changes, so every open SSE connection re-renders the list.
	messagesSubject = "messages.updated"
)

// StaticPath returns the hashed URL for a file under static/, e.g.
// StaticPath("css/main.css") -> "/static/css/main.abc123.css".
func StaticPath(format string, args ...any) string {
	return "/" + StaticSys.HashName(fmt.Sprintf("static/"+format, args...))
}

func setupRoutes(db *sql.DB, nc *nats.Conn) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get(HomeUrl, homePage(db))
	r.Get(SseUrl, homePageSse(db, nc))
	r.Post(MessagesUrl, postMessage(db, nc))

	// Serve files embedded in the binary.
	r.Handle("/static/*", hashfs.FileServer(StaticSys))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if err := NotFound().Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "NotFound", "err", err)
		}
	})

	return r
}

// RunBlocking wires up the NATS bus and routes, starts the HTTP server, and
// blocks until setupCtx is cancelled, at which point it shuts down gracefully.
func RunBlocking(setupCtx context.Context, db *sql.DB) error {
	nc, err := startNats()
	if err != nil {
		return fmt.Errorf("start nats: %w", err)
	}
	defer nc.Close()

	router := setupRoutes(db, nc)

	addr := fmt.Sprintf(":%d", streamed.Env.Port)
	srv := http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		<-setupCtx.Done()
		log.Printf("shutdown 💽__💽")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}()

	log.Printf("Starting server on http://localhost%s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Error starting server: %v", err)
	}
	return nil
}
