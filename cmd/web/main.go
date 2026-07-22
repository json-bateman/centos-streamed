// Command web is the user-facing server for the centos-streamed platform.
// It renders live information about the machine it runs on and is served to
// the public over HTTPS by Caddy.
//
// It runs as a Quadlet-managed Podman container (hello.container) on the
// shared "proxy" network. Caddy terminates TLS and reverse-proxies to it by
// container name (hello:8080), so this server itself only speaks plain HTTP
// on the internal network and is never published directly to the host.
package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// serverInfo is the data rendered into the page. Values that describe the host
// (Name, OS, Kernel) are injected by the platform CLI as environment variables
// on the container; the rest are read live at request time from the Go runtime
// and from /proc, which is not namespaced for these fields and so reflects the
// host the container runs on.
type serverInfo struct {
	Name       string
	OS         string
	Kernel     string
	Arch       string
	CPUs       int
	GoVersion  string
	Uptime     string
	MemTotal   string
	MemFree    string
	Now        string
	ClientAddr string
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// uptime reads /proc/uptime and returns a human-readable duration.
func uptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "unknown"
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "unknown"
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	case hours > 0:
		return strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	default:
		return strconv.Itoa(mins) + "m"
	}
}

// meminfo returns MemTotal and MemAvailable from /proc/meminfo, formatted in GiB.
func meminfo() (total, free string) {
	total, free = "unknown", "unknown"
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		gib := kb / 1024 / 1024
		formatted := strconv.FormatFloat(gib, 'f', 1, 64) + " GiB"
		switch fields[0] {
		case "MemTotal:":
			total = formatted
		case "MemAvailable:":
			free = formatted
		}
	}
	return
}

var page = template.Must(template.New("info").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Name}} — centos-streamed</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh;
    display: grid; place-items: center;
    font: 15px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
    background: radial-gradient(120% 120% at 50% 0%, #14203a 0%, #0a0f1c 55%);
    color: #e6edf7;
  }
  .card {
    width: min(640px, 92vw);
    background: #0f1626;
    border: 1px solid #1e2b47;
    border-radius: 14px;
    padding: 32px 34px;
    box-shadow: 0 24px 60px -20px rgba(0,0,0,.7);
  }
  .brand { display: flex; align-items: baseline; gap: 10px; margin-bottom: 4px; }
  .brand h1 { font-size: 1.35rem; margin: 0; letter-spacing: -0.02em; }
  .dot { width: 9px; height: 9px; border-radius: 50%; background: #34d399; box-shadow: 0 0 10px #34d399; }
  .sub { color: #7f8db0; margin: 0 0 22px; font-size: .85rem; }
  dl { display: grid; grid-template-columns: 128px 1fr; gap: 10px 16px; margin: 0; }
  dt { color: #7f8db0; }
  dd { margin: 0; color: #e6edf7; word-break: break-word; }
  footer { margin-top: 24px; padding-top: 16px; border-top: 1px solid #1e2b47; color: #5c6a8c; font-size: .8rem; }
  .lock { color: #34d399; }
</style>
</head>
<body>
  <main class="card">
    <div class="brand"><span class="dot"></span><h1>{{.Name}}</h1></div>
    <p class="sub">centos-streamed platform · live server information</p>
    <dl>
      <dt>Operating&nbsp;system</dt><dd>{{.OS}}</dd>
      <dt>Kernel</dt><dd>{{.Kernel}}</dd>
      <dt>Architecture</dt><dd>{{.Arch}}</dd>
      <dt>CPUs</dt><dd>{{.CPUs}}</dd>
      <dt>Memory</dt><dd>{{.MemFree}} free of {{.MemTotal}}</dd>
      <dt>Uptime</dt><dd>{{.Uptime}}</dd>
      <dt>Runtime</dt><dd>{{.GoVersion}}</dd>
      <dt>Server&nbsp;time</dt><dd>{{.Now}}</dd>
      <dt>Your&nbsp;address</dt><dd>{{.ClientAddr}}</dd>
    </dl>
    <footer><span class="lock">&#128274; HTTPS</span> · served by Caddy &rarr; hello:8080</footer>
  </main>
</body>
</html>
`))

func handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	total, free := meminfo()

	client := r.Header.Get("X-Forwarded-For")
	if client == "" {
		client = r.RemoteAddr
	}

	info := serverInfo{
		Name:       envOr("SERVER_NAME", "unknown-host"),
		OS:         envOr("SERVER_OS", "unknown"),
		Kernel:     envOr("SERVER_KERNEL", "unknown"),
		Arch:       runtime.GOARCH,
		CPUs:       runtime.NumCPU(),
		GoVersion:  runtime.Version(),
		Uptime:     uptime(),
		MemTotal:   total,
		MemFree:    free,
		Now:        time.Now().Format("2006-01-02 15:04:05 MST"),
		ClientAddr: client,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, info); err != nil {
		log.Printf("render: %v", err)
	}
}

func main() {
	addr := envOr("LISTEN_ADDR", ":8080")

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("hello server listening on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
