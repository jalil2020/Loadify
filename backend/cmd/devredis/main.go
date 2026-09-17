// Command devredis runs an in-memory Redis for local development without
// Docker.
//
// It speaks the parts of the protocol Loadify uses — strings with expiry, Lua
// scripts, streams with consumer groups, pub/sub — and keeps nothing on disk:
// stop it and every rate-limit counter, queued job id and cached value is
// gone. The database is the source of truth, so the relay re-queues what is
// due on the next tick, but the sending limits start again from zero. Never
// point anything but a developer machine at it.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6379", "address to listen on")
	flag.Parse()

	server := miniredis.NewMiniRedis()
	if err := server.StartAddr(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "devredis: %v\n", err)
		os.Exit(1)
	}
	defer server.Close()
	log.Printf("devredis listening on %s (in memory, development only)", server.Addr())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// The in-memory server only ages keys when told to. Without this, a
	// cooldown or a lock set with a TTL would never expire.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-stop:
			log.Print("devredis stopped; everything it held is gone")
			return
		case now := <-ticker.C:
			server.FastForward(now.Sub(last))
			last = now
		}
	}
}
