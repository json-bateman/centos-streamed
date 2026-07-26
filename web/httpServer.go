package web

import (
	"context"
	"embed"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/benbjohnson/hashfs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	streamed "github.com/json-bateman/centos-streamed"
	"github.com/starfederation/datastar-go/datastar"
)

//go:embed static/*
var StaticFS embed.FS

// StaticSys serves embedded static files under content-hashed names so they can
// be cached forever; when a file changes its hash changes and busts the cache.
var StaticSys = hashfs.NewFS(StaticFS)

var CommitHash = "dev"

const (
	HomeUrl      = "/"
	ProcessesUrl = "/processes"
)

// StaticPath returns the hashed URL for a file under static/, e.g.
// StaticPath("css/main.css") -> "/static/css/main.abc123.css".
func StaticPath(format string, args ...any) string {
	return "/" + StaticSys.HashName(fmt.Sprintf("static/"+format, args...))
}

func getCommitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func setupRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get(HomeUrl, homePage())
	r.Get(HomeUrl+"sse", homePageSse())

	r.Get(ProcessesUrl, processesPage())
	r.Get(ProcessesUrl+"/sse", processesPageSse())

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

// RunBlocking starts the HTTP server and blocks until setupCtx is cancelled, at
// which point it shuts down gracefully.
func RunBlocking(setupCtx context.Context) error {
	if CommitHash == "dev" {
		CommitHash = getCommitHash()
	}
	router := setupRoutes()

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

func homePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := Home(collectServerInfo(), collectProcesses(processLimit)).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "Page", "err", err)
		}
	}
}

func homePageSse() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if err := sse.PatchElementTempl(HomeSSE(collectServerInfo(), collectProcesses(processLimit))); err != nil {
					return
				}
			}
		}
	}
}

// processLimit caps how many processes the table shows (top N by memory).
const processLimit = 40

func processesPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ProcessesPage(collectProcesses(processLimit)).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "Page", "err", err)
		}
	}
}

func processesPageSse() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if err := sse.PatchElementTempl(ProcessesSSE(collectProcesses(processLimit))); err != nil {
					return
				}
			}
		}
	}
}
