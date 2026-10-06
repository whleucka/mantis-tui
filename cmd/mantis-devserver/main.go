// Command mantis-devserver runs an in-memory MantisBT REST server seeded with
// the scrubbed test fixtures, for trying mantis-tui without a real host.
//
//	go run ./cmd/mantis-devserver -addr 127.0.0.1:8989
//
// Then point a config host at http://127.0.0.1:8989 with any token.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/whleucka/mantis-tui/internal/devserver"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8989", "listen address")
	fixtures := flag.String("fixtures", "internal/mantis/testdata", "fixture directory")
	delay := flag.Duration("delay", 150*time.Millisecond, "artificial latency per request")
	flag.Parse()

	logger := log.New(os.Stderr, "devserver: ", log.Ltime)
	srv, err := devserver.New(*fixtures, logger)
	if err != nil {
		logger.Fatal(err)
	}
	srv.Delay = *delay
	logger.Printf("serving Mantis fixtures on http://%s (writes are logged and kept in memory)", *addr)
	logger.Fatal(http.ListenAndServe(*addr, srv)) //nolint:gosec // local dev tool
}
