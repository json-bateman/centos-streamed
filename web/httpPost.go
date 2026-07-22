package web

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/json-bateman/centos-streamed/sql/sqlcgen"
	nats "github.com/nats-io/nats.go"
	"github.com/starfederation/datastar-go/datastar"
)

// MessageSignals are the Datastar signals bound to the message form inputs.
type MessageSignals struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

func postMessage(db *sql.DB, nc *nats.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var signals MessageSignals
		if err := json.NewDecoder(r.Body).Decode(&signals); err != nil {
			slog.Error("decode message signals", "err", err)
			return
		}

		author := strings.TrimSpace(signals.Author)
		body := strings.TrimSpace(signals.Body)
		if author == "" {
			author = "anonymous"
		}
		if body == "" {
			return
		}
		if len(body) > 500 {
			body = body[:500]
		}

		q := sqlcgen.New(db)
		if _, err := q.CreateMessage(r.Context(), sqlcgen.CreateMessageParams{
			Author: author,
			Body:   body,
		}); err != nil {
			slog.Error("CreateMessage", "err", err)
			return
		}

		// Notify every open SSE connection (including this client) to re-render.
		_ = nc.Publish(messagesSubject, nil)

		// Clear the message input on the posting client.
		sse := datastar.NewSSE(w, r)
		_ = sse.PatchSignals([]byte(`{"body": ""}`))
	}
}
