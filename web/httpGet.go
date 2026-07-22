package web

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/json-bateman/centos-streamed/sql/sqlcgen"
	nats "github.com/nats-io/nats.go"
	"github.com/starfederation/datastar-go/datastar"
)

// recentMessages returns the newest messages, most recent first.
func recentMessages(ctx context.Context, db *sql.DB) []sqlcgen.Message {
	q := sqlcgen.New(db)
	msgs, err := q.GetRecentMessages(ctx, 50)
	if err != nil {
		slog.Error("GetRecentMessages", "err", err)
		return nil
	}
	return msgs
}

func homePage(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := collectServerInfo()
		msgs := recentMessages(r.Context(), db)
		if err := Home(info, msgs).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "Home", "err", err)
		}
	}
}

// homePageSse keeps the page live: a one-second ticker refreshes the server-info
// card, and a NATS subscription refreshes the message list whenever anyone posts.
func homePageSse(db *sql.DB, nc *nats.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		msgCh := make(chan *nats.Msg, 8)
		sub, err := nc.ChanSubscribe(messagesSubject, msgCh)
		if err != nil {
			slog.Error("homePageSse subscribe", "err", err)
			return
		}
		defer func() { _ = sub.Unsubscribe() }()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		// Prime both fragments immediately on connect.
		if err := sse.PatchElementTempl(ServerInfoCard(collectServerInfo())); err != nil {
			return
		}
		if err := sse.PatchElementTempl(MessageList(recentMessages(r.Context(), db))); err != nil {
			return
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if err := sse.PatchElementTempl(ServerInfoCard(collectServerInfo())); err != nil {
					return
				}
			case <-msgCh:
				if err := sse.PatchElementTempl(MessageList(recentMessages(r.Context(), db))); err != nil {
					return
				}
			}
		}
	}
}
