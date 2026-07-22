package web

import "time"

// humanTime renders a unix timestamp (seconds) as a short local clock time.
func humanTime(unix int64) string {
	return time.Unix(unix, 0).Format("15:04:05")
}
