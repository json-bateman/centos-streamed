package web

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/starfederation/datastar-go/datastar"
)

// processLimit caps how many processes the table shows (top N by memory).
const processLimit = 40

func homePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := Home(collectServerInfo(), collectProcesses(processLimit)).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "Page", "err", err)
		}
	}
}

// homePageSse keeps the page live
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
				if err := sse.PatchElementTempl(Home(collectServerInfo(), collectProcesses(processLimit))); err != nil {
					return
				}
			}
		}
	}
}
