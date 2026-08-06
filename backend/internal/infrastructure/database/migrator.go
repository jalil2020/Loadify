package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loadify/backend/pkg/logger"
)

// Migration is one versioned pair of .up.sql / .down.sql files.
type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}

// AppliedMigration is a row of schema_migrations.
type AppliedMigration struct {
	Version   int64
	Name      string
	AppliedAt time.Time
	Dirty     bool
}

// Migrator applies SQL migrations inside advisory-locked transactions.
// Each migration runs in its own transaction: a failure rolls that migration
// back and leaves the previous ones applied, which is what you want when
// re-running after a fix.
type Migrator struct {
	db   *sql.DB
	dir  string
	log  logger.Logger
	lock int64
}

// advisoryLockID keeps two instances from migrating at the same time.
const advisoryLockID int64 = 8_913_402_115

// NewMigrator builds a migrator over the given directory of .sql files.
func NewMigrator(db *sql.DB, dir string, log logger.Logger) *Migrator {
	return &Migrator{db: db, dir: dir, log: log, lock: advisoryLockID}
}

func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    BIGINT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			dirty      BOOLEAN NOT NULL DEFAULT FALSE
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

// Load reads and pairs migration files from disk.
func (m *Migrator) Load() ([]Migration, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %s: %w", m.dir, err)
	}

	byVersion := map[int64]*Migration{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, name, direction, err := parseFilename(e.Name())
		if err != nil {
			return nil, err
		}
		body, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		mig, ok := byVersion[version]
		if !ok {
			mig = &Migration{Version: version, Name: name}
			byVersion[version] = mig
		}
		if direction == "up" {
			mig.UpSQL = string(body)
		} else {
			mig.DownSQL = string(body)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for _, mig := range byVersion {
		if mig.UpSQL == "" {
			return nil, fmt.Errorf("migration %d (%s) has no .up.sql", mig.Version, mig.Name)
		}
		out = append(out, *mig)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// parseFilename splits "000007_bookings_trips.up.sql" into its parts.
func parseFilename(fname string) (version int64, name, direction string, err error) {
	base := strings.TrimSuffix(fname, ".sql")
	dotIdx := strings.LastIndex(base, ".")
	if dotIdx < 0 {
		return 0, "", "", fmt.Errorf("migration %q must end in .up.sql or .down.sql", fname)
	}
	direction = base[dotIdx+1:]
	if direction != "up" && direction != "down" {
		return 0, "", "", fmt.Errorf("migration %q has unknown direction %q", fname, direction)
	}
	rest := base[:dotIdx]
	underscore := strings.Index(rest, "_")
	if underscore < 0 {
		return 0, "", "", fmt.Errorf("migration %q must be named <version>_<name>.<direction>.sql", fname)
	}
	version, err = strconv.ParseInt(rest[:underscore], 10, 64)
	if err != nil {
		return 0, "", "", fmt.Errorf("migration %q has a non-numeric version: %w", fname, err)
	}
	return version, rest[underscore+1:], direction, nil
}

// Applied lists migrations recorded in the database.
func (m *Migrator) Applied(ctx context.Context) ([]AppliedMigration, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT version, name, applied_at, dirty FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	var out []AppliedMigration
	for rows.Next() {
		var a AppliedMigration
		if err := rows.Scan(&a.Version, &a.Name, &a.AppliedAt, &a.Dirty); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Up applies every pending migration in version order.
func (m *Migrator) Up(ctx context.Context) error {
	unlock, err := m.acquireLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	if err := m.ensureTable(ctx); err != nil {
		return err
	}

	applied, err := m.Applied(ctx)
	if err != nil {
		return err
	}
	done := make(map[int64]bool, len(applied))
	for _, a := range applied {
		if a.Dirty {
			return fmt.Errorf("migration %d (%s) is dirty; fix it and run 'migrate force %d'",
				a.Version, a.Name, a.Version)
		}
		done[a.Version] = true
	}

	migrations, err := m.Load()
	if err != nil {
		return err
	}

	pending := 0
	for _, mig := range migrations {
		if done[mig.Version] {
			continue
		}
		pending++
		start := time.Now()
		if err := m.applyOne(ctx, mig); err != nil {
			return err
		}
		m.log.Info("migration applied",
			logger.Int64("version", mig.Version),
			logger.String("name", mig.Name),
			logger.Duration("took", time.Since(start)))
	}

	if pending == 0 {
		m.log.Info("database schema is up to date")
	}
	return nil
}

func (m *Migrator) applyOne(ctx context.Context, mig Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for migration %d: %w", mig.Version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, mig.UpSQL); err != nil {
		return fmt.Errorf("migration %d (%s) failed: %w", mig.Version, mig.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, dirty) VALUES ($1, $2, FALSE)`,
		mig.Version, mig.Name); err != nil {
		return fmt.Errorf("record migration %d: %w", mig.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", mig.Version, err)
	}
	return nil
}

// Down rolls back the last n applied migrations.
func (m *Migrator) Down(ctx context.Context, n int) error {
	if n <= 0 {
		return errors.New("down: n must be positive")
	}
	unlock, err := m.acquireLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	applied, err := m.Applied(ctx)
	if err != nil {
		return err
	}
	migrations, err := m.Load()
	if err != nil {
		return err
	}
	byVersion := make(map[int64]Migration, len(migrations))
	for _, mig := range migrations {
		byVersion[mig.Version] = mig
	}

	for i := len(applied) - 1; i >= 0 && n > 0; i, n = i-1, n-1 {
		a := applied[i]
		mig, ok := byVersion[a.Version]
		if !ok || mig.DownSQL == "" {
			return fmt.Errorf("migration %d (%s) has no .down.sql", a.Version, a.Name)
		}
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		if _, err := tx.ExecContext(ctx, mig.DownSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("rollback of migration %d failed: %w", a.Version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM schema_migrations WHERE version = $1`, a.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("remove migration record %d: %w", a.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit rollback of %d: %w", a.Version, err)
		}
		m.log.Info("migration rolled back",
			logger.Int64("version", a.Version), logger.String("name", a.Name))
	}
	return nil
}

// Force marks a version as applied and clean, for recovering from a partial failure
// that was repaired by hand.
func (m *Migrator) Force(ctx context.Context, version int64) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	migrations, err := m.Load()
	if err != nil {
		return err
	}
	name := "forced"
	for _, mig := range migrations {
		if mig.Version == version {
			name = mig.Name
			break
		}
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, dirty) VALUES ($1, $2, FALSE)
		ON CONFLICT (version) DO UPDATE SET dirty = FALSE, name = EXCLUDED.name`, version, name)
	if err != nil {
		return fmt.Errorf("force version %d: %w", version, err)
	}
	m.log.Info("migration version forced", logger.Int64("version", version))
	return nil
}

// Status returns every known migration with its applied state.
func (m *Migrator) Status(ctx context.Context) (map[int64]bool, []Migration, error) {
	migrations, err := m.Load()
	if err != nil {
		return nil, nil, err
	}
	applied, err := m.Applied(ctx)
	if err != nil {
		return nil, nil, err
	}
	state := make(map[int64]bool, len(migrations))
	for _, mig := range migrations {
		state[mig.Version] = false
	}
	for _, a := range applied {
		state[a.Version] = true
	}
	return state, migrations, nil
}

func (m *Migrator) acquireLock(ctx context.Context) (func(), error) {
	if _, err := m.db.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, m.lock); err != nil {
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	return func() {
		if _, err := m.db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, m.lock); err != nil {
			m.log.Warn("release migration lock failed", logger.Err(err))
		}
	}, nil
}

// DefaultDir resolves the migrations directory relative to the working directory,
// falling back to the path used inside the container image.
func DefaultDir() string {
	for _, candidate := range []string{"migrations", "backend/migrations", "/app/migrations"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
	}
	return "migrations"
}
