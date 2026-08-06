// Command api runs the Loadify HTTP and WebSocket server.
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

	log.Info("starting loadify api",
		logger.String("version", bootstrap.Version),
		logger.String("env", cfg.App.Env))

	container, err := bootstrap.NewContainer(cfg, log)
	if err != nil {
		return err
	}

	app := bootstrap.NewFiberApp(container)
	return bootstrap.Run(container, app)
}
