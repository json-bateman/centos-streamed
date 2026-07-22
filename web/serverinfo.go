package web

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ServerInfo is the live host information rendered into the page. Fields that
// describe the host (Name, OS, Kernel) come from environment variables set by
// the platform CLI on the container; the rest are read at request time from the
// Go runtime and /proc, which is not namespaced for these values and so
// reflects the host the container runs on.
type ServerInfo struct {
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
	Goroutines int
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// collectServerInfo gathers a fresh snapshot of host information.
func collectServerInfo() ServerInfo {
	total, free := meminfo()
	return ServerInfo{
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
		Goroutines: runtime.NumGoroutine(),
	}
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
