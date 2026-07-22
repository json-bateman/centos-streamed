package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// templatesFS holds the Quadlet and Caddy source files. They are text/template
// files (static ones simply have no template actions) rendered by renderManaged.
//
//go:embed templates
var templatesFS embed.FS

const (
	quadletDir = "/etc/containers/systemd"
	caddyDir   = "/etc/caddy"
	sitesDir   = "/etc/caddy/sites"

	// streamedImage is the tag produced by the streamed.build unit and consumed
	// by streamed.container.
	streamedImage = "localhost/streamed:latest"
)

type fileDefinition struct {
	content string
	mode    fs.FileMode
}

// config holds the deployment inputs, sourced from flags and the host.
type config struct {
	host         string // hostname Caddy serves the streamed site on
	tlsMode      string // "internal" (Caddy local CA) or "auto" (Let's Encrypt)
	buildContext string // build context for streamed.build (the repo root)
	generateOnly bool   // when true, write files but run no systemctl/caddy steps

	// Host facts injected into streamed.container as environment variables.
	serverName   string
	serverOS     string
	serverKernel string
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	host := flag.String("host", "localhost", "hostname Caddy serves the streamed site on")
	tlsMode := flag.String("tls", "internal", "TLS mode: 'internal' (Caddy local CA) or 'auto' (Let's Encrypt)")
	generateOnly := flag.Bool("generate-only", false, "write files but do not run systemctl / caddy reload")
	teardown := flag.Bool("teardown", false, "stop services and remove everything the platform manages, then exit")
	flag.Parse()

	if os.Geteuid() != 0 {
		return errors.New("this command must be run as root")
	}

	if *teardown {
		return doTeardown()
	}

	if *tlsMode != "internal" && *tlsMode != "auto" {
		return fmt.Errorf("invalid -tls %q: must be 'internal' or 'auto'", *tlsMode)
	}

	buildContext, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determine build context: %w", err)
	}
	if _, err := os.Stat(filepath.Join(buildContext, "Containerfile")); err != nil {
		return fmt.Errorf(
			"no Containerfile in %s: run this from the repository root so streamed.build has a valid context",
			buildContext,
		)
	}

	name, osName, kernel := hostFacts()

	cfg := config{
		host:         *host,
		tlsMode:      *tlsMode,
		buildContext: buildContext,
		generateOnly: *generateOnly,
		serverName:   name,
		serverOS:     osName,
		serverKernel: kernel,
	}

	directories := []string{quadletDir, caddyDir, sitesDir}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", directory, err)
		}
	}

	rendered, err := renderManaged(cfg)
	if err != nil {
		return err
	}
	for path, definition := range rendered {
		if err := writeFileAtomic(path, []byte(definition.content), definition.mode); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", path)
	}

	if cfg.generateOnly {
		fmt.Println("\nFiles written. Skipping apply (-generate-only). Next run:")
		fmt.Println("  sudo systemctl daemon-reload")
		fmt.Println("  sudo systemctl start caddy.service streamed.service")
		return nil
	}

	if err := apply(cfg); err != nil {
		return err
	}

	scheme := "https"
	fmt.Printf("\nDone. streamed is live at %s://%s\n", scheme, cfg.host)
	if cfg.tlsMode == "internal" {
		fmt.Println("(TLS uses Caddy's internal CA — expect an untrusted-cert warning until you trust its root.)")
	}
	return nil
}

// hostFacts reads identifying details about the machine to display on the page.
func hostFacts() (name, osName, kernel string) {
	name, osName, kernel = "unknown-host", "unknown", "unknown"

	if h, err := os.Hostname(); err == nil && h != "" {
		name = h
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				osName = strings.Trim(value, `"`)
			}
		}
	}
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(data))
	}
	return
}

// managedFile maps an embedded template to where it is installed on the host.
type managedFile struct {
	template string      // filename under the embedded templates/ directory
	dest     string      // absolute destination path
	mode     fs.FileMode // permissions for the installed file
}

// managedLayout is every file the platform owns. This is the single source of
// truth for both applying (render + write) and teardown (remove by dest).
var managedLayout = []managedFile{
	{"proxy.network", filepath.Join(quadletDir, "proxy.network"), 0644},
	{"caddy-data.volume", filepath.Join(quadletDir, "caddy-data.volume"), 0644},
	{"caddy-config.volume", filepath.Join(quadletDir, "caddy-config.volume"), 0644},
	{"caddy.container", filepath.Join(quadletDir, "caddy.container"), 0644},
	{"Caddyfile", filepath.Join(caddyDir, "Caddyfile"), 0644},
	{"streamed.build", filepath.Join(quadletDir, "streamed.build"), 0644},
	{"streamed.container", filepath.Join(quadletDir, "streamed.container"), 0644},
	{"streamed.caddy", filepath.Join(sitesDir, "streamed.caddy"), 0644},
}

// templateData is the data exposed to the embedded templates.
type templateData struct {
	Image        string
	BuildContext string
	ServerName   string
	ServerOS     string
	ServerKernel string
	Host         string
	TLSInternal  bool
}

// renderManaged renders every template in managedLayout, keyed by destination
// path — the same shape the writer loop already consumes.
func renderManaged(cfg config) (map[string]fileDefinition, error) {
	data := templateData{
		Image:        streamedImage,
		BuildContext: cfg.buildContext,
		ServerName:   cfg.serverName,
		ServerOS:     cfg.serverOS,
		ServerKernel: cfg.serverKernel,
		Host:         cfg.host,
		TLSInternal:  cfg.tlsMode == "internal",
	}

	funcs := template.FuncMap{"env": systemdEnv}
	files := make(map[string]fileDefinition, len(managedLayout))

	for _, f := range managedLayout {
		source, err := templatesFS.ReadFile("templates/" + f.template)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", f.template, err)
		}

		tmpl, err := template.New(f.template).Funcs(funcs).Parse(string(source))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", f.template, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("render template %s: %w", f.template, err)
		}

		files[f.dest] = fileDefinition{content: buf.String(), mode: f.mode}
	}

	return files, nil
}

// systemdEnv formats a quoted Environment= value so that values containing
// spaces (e.g. "CentOS Stream 10") are passed as a single variable. It is
// exposed to templates as the "env" function.
func systemdEnv(key, value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return fmt.Sprintf(`"%s=%s"`, key, value)
}

// apply reloads systemd, (re)starts the services, and reloads Caddy so the new
// site is picked up — the whole point of the CLI being able to run unattended.
func apply(cfg config) error {
	steps := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "start", "caddy.service"},
		{"systemctl", "start", "streamed.service"},
	}
	for _, step := range steps {
		if err := runStep(step...); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(step, " "), err)
		}
	}
	return reloadCaddy()
}

// reloadCaddy asks the running Caddy container to reload its config in place.
// If that is not possible (e.g. Caddy was just started), it restarts the unit.
func reloadCaddy() error {
	err := runStep("podman", "exec", "caddy",
		"caddy", "reload", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
	if err == nil {
		return nil
	}
	fmt.Println("in-place caddy reload failed; restarting caddy.service instead")
	if err := runStep("systemctl", "restart", "caddy.service"); err != nil {
		return fmt.Errorf("restart caddy.service: %w", err)
	}
	return nil
}

// doTeardown reverses apply: it stops the services, removes every file the
// platform owns, reloads systemd, and deletes the podman resources. Each step
// is best-effort so teardown is idempotent — safe on a half-built or already
// clean host. Removing the image also guarantees the next apply rebuilds it.
func doTeardown() error {
	fmt.Println("Tearing down the centos-streamed platform...")

	// Stop every generated unit while its file still exists — including the
	// network, volume, and build oneshots. Those are Type=oneshot with
	// RemainAfterExit=yes, so if we delete the underlying podman network/volumes
	// without stopping the units, systemd still believes they exist and a later
	// apply won't recreate them (leaving containers with "network not found").
	services := []string{
		"streamed.service", "caddy.service",
		"streamed-build.service",
		"proxy-network.service",
		"caddy-data-volume.service", "caddy-config-volume.service",
	}
	tryStep(append([]string{"systemctl", "stop"}, services...)...)
	tryStep(append([]string{"systemctl", "reset-failed"}, services...)...)

	// Remove the managed files. Teardown works straight off managedLayout
	for _, f := range managedLayout {
		switch err := os.Remove(f.dest); {
		case err == nil:
			fmt.Printf("removed %s\n", f.dest)
		case errors.Is(err, fs.ErrNotExist):
			// already gone
		default:
			fmt.Printf("warning: remove %s: %v\n", f.dest, err)
		}
	}

	// Reload so systemd forgets the now-deleted Quadlet units.
	tryStep("systemctl", "daemon-reload")

	// Remove the podman resources. Containers first so the image, volumes, and
	// network are no longer in use.
	tryStep("podman", "rm", "-f", "caddy", "streamed")
	tryStep("podman", "rmi", streamedImage)
	tryStep("podman", "volume", "rm", "caddy-data", "caddy-config")
	tryStep("podman", "network", "rm", "proxy")

	fmt.Println("\nTeardown complete. Run 'sudo go run ./cmd/platform' to rebuild.")
	return nil
}

// tryStep runs a command like runStep but never fails the caller — teardown
// tolerates missing units, containers, or volumes.
func tryStep(args ...string) {
	if err := runStep(args...); err != nil {
		fmt.Printf("(ignored) %s: %v\n", strings.Join(args, " "), err)
	}
}

// runStep runs a command, echoing it and streaming its output to the terminal.
func runStep(args ...string) error {
	fmt.Printf("\n$ %s\n", strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeFileAtomic(path string, content []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)

	tempFile, err := os.CreateTemp(directory, ".platform-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}

	tempPath := tempFile.Name()
	removeTemp := true

	defer func() {
		_ = tempFile.Close()

		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}

	if err := tempFile.Chmod(mode); err != nil {
		return fmt.Errorf("set permissions for %s: %w", path, err)
	}

	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}

	removeTemp = false
	return nil
}
