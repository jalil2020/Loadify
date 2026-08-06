// Package database owns the PostgreSQL connection and the migration runner.
package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/pkg/logger"
)

// DB wraps the GORM handle.
type DB struct {
	*gorm.DB
	log logger.Logger
}

// New opens the pool, verifies connectivity and applies pool limits.
func New(cfg config.DBConfig, appEnv string, log logger.Logger) (*DB, error) {
	level := gormlogger.Warn
	if appEnv == "development" {
		level = gormlogger.Info
	}

	gormCfg := &gorm.Config{
		Logger: gormlogger.New(&gormLogWriter{log: log}, gormlogger.Config{
			SlowThreshold:             cfg.LogSlowQueries,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
		NamingStrategy:                           schema.NamingStrategy{SingularTable: false},
		DisableForeignKeyConstraintWhenMigrating: true,
		PrepareStmt:                              true,
		SkipDefaultTransaction:                   true,
		NowFunc:                                  func() time.Time { return time.Now().UTC() },
	}

	gdb, err := gorm.Open(postgres.Open(cfg.DSN()), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("access sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.Info("postgres connected",
		logger.String("host", cfg.Host),
		logger.String("database", cfg.Name),
		logger.Int("max_open_conns", cfg.MaxOpenConns))

	return &DB{DB: gdb, log: log}, nil
}

// Health reports whether the database answers within the context deadline.
func (d *DB) Health(ctx context.Context) error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Stats exposes pool counters for the metrics endpoint.
func (d *DB) Stats() map[string]any {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	s := sqlDB.Stats()
	return map[string]any{
		"open_connections": s.OpenConnections,
		"in_use":           s.InUse,
		"idle":             s.Idle,
		"wait_count":       s.WaitCount,
		"wait_duration_ms": s.WaitDuration.Milliseconds(),
	}
}

// Close drains the pool.
func (d *DB) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

type gormLogWriter struct{ log logger.Logger }

func (w *gormLogWriter) Printf(format string, args ...any) {
	w.log.Debug(fmt.Sprintf(format, args...))
}
