package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/starfederation/datastar-go/datastar"
)

var UpdateTick = 1

type TickRequest struct {
	SelectValue string `json:"select_value"`
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
				if err := sse.PatchElementTempl(Home(collectServerInfo())); err != nil {
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
				if err := sse.PatchElementTempl(ProcessesPage(collectProcesses(processLimit))); err != nil {
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
		if !entry.IsDir() {
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
				if err := sse.PatchElementTempl(EtcPage(caddyConfig, containers)); err != nil {
					return
				}
			}
		}
	}
}

func sshPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := SshPage().Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "EtcPage", "err", err)
		}
	}
}

func sshPageSSE() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		ticker := time.NewTicker(time.Duration(UpdateTick) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if err := sse.PatchElementTempl(SshPage()); err != nil {
					return
				}
			}
		}
	}
}
