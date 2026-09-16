// Command poster runs the Telegram posting worker.
//
// It is a separate process from the API on purpose: Telegram sessions are
// opened only here, a deploy of the API never interrupts a send, and the worker
// can be scaled with the number of connected accounts rather than with traffic.
package main

import (
	"fmt"
	"os"

	"github.com/loadify/backend/internal/bootstrap"
	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "startup failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(".env", "../.env", "../../.env")
	if err != nil {
		return err
	}

	log := logger.New(cfg.App.Env, cfg.App.LogLevel)
	defer func() { _ = log.Sync() }()

	container, err := bootstrap.NewPosterContainer(cfg, log)
	if err != nil {
		return err
	}
	defer container.Close()

	return bootstrap.RunPoster(container)
}
