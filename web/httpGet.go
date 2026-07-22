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

// recentEvents returns the newest events, most recent first.
func recentEvents(ctx context.Context, db *sql.DB) []sqlcgen.Event {
	q := sqlcgen.New(db)
	events, err := q.GetRecentEvents(ctx, 50)
	if err != nil {
		slog.Error("GetRecentEvents", "err", err)
		return nil
	}
	return events
}

// recordVisit inserts a "visit" event for the requesting client and notifies
// every open SSE connection to re-render the activity feed.
func recordVisit(ctx context.Context, db *sql.DB, nc *nats.Conn, ip string) {
	q := sqlcgen.New(db)
	if _, err := q.CreateEvent(ctx, sqlcgen.CreateEventParams{Kind: "visit", Ip: ip}); err != nil {
		slog.Error("CreateEvent", "err", err)
		return
	}
	_ = nc.Publish(eventsSubject, nil)
}

func homePage(db *sql.DB, nc *nats.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Loading the app is the "logged on" moment: record it and broadcast.
		recordVisit(r.Context(), db, nc, clientIP(r))

		info := collectServerInfo()
		events := recentEvents(r.Context(), db)
		if err := Home(info, events).Render(r.Context(), w); err != nil {
			slog.Debug("render error", "component", "Home", "err", err)
		}
	}
}

// homePageSse keeps the page live: a one-second ticker refreshes the server-info
// card, and a NATS subscription refreshes the activity feed whenever a new event
// is recorded (by anyone loading the app).
func homePageSse(db *sql.DB, nc *nats.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithBrotli()))

		eventCh := make(chan *nats.Msg, 8)
		sub, err := nc.ChanSubscribe(eventsSubject, eventCh)
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
		if err := sse.PatchElementTempl(EventList(recentEvents(r.Context(), db))); err != nil {
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
			case <-eventCh:
				if err := sse.PatchElementTempl(EventList(recentEvents(r.Context(), db))); err != nil {
					return
				}
			}
		}
	}
}
