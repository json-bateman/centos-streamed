package web

import (
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// startNats boots an in-process NATS server on a random port and returns a
// client connected to it. It is used as a lightweight pub/sub bus to fan out
// "something changed" notifications to every open SSE connection.
func startNats() (*nats.Conn, error) {
	ns, err := server.NewServer(&server.Options{
		Port: server.RANDOM_PORT,
	})
	if err != nil {
		return nil, err
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		return nil, fmt.Errorf("nats not ready")
	}
	return nats.Connect(ns.ClientURL())
}
