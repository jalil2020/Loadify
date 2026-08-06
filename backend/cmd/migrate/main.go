// Command migrate applies, rolls back and inspects database migrations.
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down 1
//	go run ./cmd/migrate status
//	go run ./cmd/migrate force 7
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/infrastructure/database"
	"github.com/loadify/backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: migrate <up|down|status|force> [n]")
	}
	command := os.Args[1]

	cfg, err := config.Load(".env", "../.env", "../../.env")
	if err != nil {
		return err
	}
	log := logger.New(cfg.App.Env, cfg.App.LogLevel)
	defer func() { _ = log.Sync() }()

	db, err := database.New(cfg.DB, cfg.App.Env, log)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}

	migrator := database.NewMigrator(sqlDB, database.DefaultDir(), log)
	ctx := context.Background()

	switch command {
	case "up":
		return migrator.Up(ctx)

	case "down":
		steps := 1
		if len(os.Args) > 2 {
			n, err := strconv.Atoi(os.Args[2])
			if err != nil {
				return fmt.Errorf("down: %q is not a number", os.Args[2])
			}
			steps = n
		}
		return migrator.Down(ctx, steps)

	case "force":
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: migrate force <version>")
		}
		version, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			return fmt.Errorf("force: %q is not a version number", os.Args[2])
		}
		return migrator.Force(ctx, version)

	case "status":
		state, migrations, err := migrator.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-10s %-30s %s\n", "VERSION", "NAME", "STATUS")
		for _, m := range migrations {
			status := "pending"
			if state[m.Version] {
				status = "applied"
			}
			fmt.Printf("%-10d %-30s %s\n", m.Version, m.Name, status)
		}
		return nil

	default:
		return fmt.Errorf("unknown command %q; use up, down, status or force", command)
	}
}
