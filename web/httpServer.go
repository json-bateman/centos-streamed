package web

import (
	"context"
	"embed"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sort"
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
	ProcessesUrl = "/proc"
	EtcUrl       = "/etc"
)

var UpdateTick = 1

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

	r.Get(EtcUrl, etcPage())
	r.Get(EtcUrl+"/sse", etcPageSSE())

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
		if err := Home(collectServerInfo()).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "Page", "err", err)
		}
	}
}

func homePageSse() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		ticker := time.NewTicker(time.Duration(UpdateTick) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if err := sse.PatchElementTempl(HomeSSE(collectServerInfo())); err != nil {
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

		ticker := time.NewTicker(time.Duration(UpdateTick) * time.Second)
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

// readCaddyfile reads the Caddy configuration file from /etc/caddy/Caddyfile
func readCaddyfile() string {
	data, err := os.ReadFile("/etc/caddy/Caddyfile")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
			return "Caddyfile not present in etc/caddy/Caddyfile\nInstall Caddy and Add Caddyfile."
		}
		fmt.Println(err.Error())
		return "Error reading Caddyfile: " + err.Error()
	}
	return string(data)
}

// readContainerFiles reads all .container files from /etc/containers/systemd/ and returns them sorted
func readContainerFiles() []struct{ Name, Content string } {
	var containers []struct{ Name, Content string }
	dir := "/etc/containers/systemd"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return containers
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".container") {
			filePath := dir + "/" + entry.Name()
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			containers = append(containers, struct{ Name, Content string }{
				Name:    entry.Name(),
				Content: string(data),
			})
		}
	}

	// Sort alphabetically by name
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	return containers
}

func etcPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caddyConfig := readCaddyfile()
		containers := readContainerFiles()
		if err := EtcPage(caddyConfig, containers).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "EtcPage", "err", err)
		}
	}
}

func etcPageSSE() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		ticker := time.NewTicker(time.Duration(UpdateTick) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				caddyConfig := readCaddyfile()
				containers := readContainerFiles()
				if err := sse.PatchElementTempl(EtcPageSSE(caddyConfig, containers)); err != nil {
					return
				}
			}
		}
	}
}
