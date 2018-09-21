/*
Sniperkit-Bot
- Status: analyzed
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	maintainer "github.com/sniperkit/snk.fork.bradleyfalzon-maintainer.me"
	"github.com/sniperkit/snk.fork.bradleyfalzon-maintainer.me/events"
	"github.com/sniperkit/snk.fork.bradleyfalzon-maintainer.me/notifier"
)

func main() {
	_ = godotenv.Load() // Ignore errors as .env is optional

	ctx := context.Background()

	m, err := maintainer.NewMaintainer()
	if err != nil {
		log.Fatal(err)
	}

	// Notifiers
	notifier := &notifier.Writer{Writer: os.Stdout}

	// Poller
	poller := events.NewPoller(m.Logger, m.DB, notifier, m.Cache)
	err = poller.Poll(ctx, 60*time.Second) // blocking
	if err != nil {
		m.Logger.WithError(err).Fatalf("Poller failed")
	}
	m.Logger.Info("Poller exiting")
}
