package web

import (
	"net/http"
	"strings"
	"time"
)

// humanTime renders a unix timestamp (seconds) as a short local clock time.
func humanTime(unix int64) string {
	return time.Unix(unix, 0).Format("15:04:05")
}

// clientIP returns the requesting client's address. Behind Caddy the real
// client is in X-Forwarded-For (which may be a comma-separated list — the first
// entry is the origin); otherwise fall back to the transport-level RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}
