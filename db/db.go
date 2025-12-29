package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

//go:generate go tool github.com/sqlc-dev/sqlc/cmd/sqlc generate

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open opens an sqlite database and prepares pragmas suitable for a small web app.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Light pragmas similar
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=wal;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=1000;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	return db, nil
}

// MigrationResult contains information about an applied migration
type MigrationResult struct {
	Filename  string
	StartTime time.Time
	EndTime   time.Time
}

// RunMigrations executes database migrations in numeric order (NNN-*.sql),
// similar in spirit to exed's exedb.RunMigrations.
// Returns a list of migrations that were applied.
func RunMigrations(db *sql.DB) ([]MigrationResult, error) {
	var results []MigrationResult

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var migrations []string
	pat := regexp.MustCompile(`^(\d{3})-.*\.sql$`)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if pat.MatchString(name) {
			migrations = append(migrations, name)
		}
	}
	sort.Strings(migrations)

	executed := make(map[int]bool)
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='migrations'").Scan(&tableName)
	switch {
	case err == nil:
		rows, err := db.Query("SELECT migration_number FROM migrations")
		if err != nil {
			return nil, fmt.Errorf("query executed migrations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				return nil, fmt.Errorf("scan migration number: %w", err)
			}
			executed[n] = true
		}
	case errors.Is(err, sql.ErrNoRows):
		slog.Info("db: migrations table not found; running all migrations")
	default:
		return nil, fmt.Errorf("check migrations table: %w", err)
	}

	for _, m := range migrations {
		match := pat.FindStringSubmatch(m)
		if len(match) != 2 {
			return nil, fmt.Errorf("invalid migration filename: %s", m)
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration number %s: %w", m, err)
		}
		if executed[n] {
			continue
		}

		startTime := time.Now()
		if err := executeMigration(db, m); err != nil {
			return results, fmt.Errorf("execute %s: %w", m, err)
		}
		endTime := time.Now()

		results = append(results, MigrationResult{
			Filename:  m,
			StartTime: startTime,
			EndTime:   endTime,
		})
		slog.Info("db: applied migration", "file", m, "number", n)
	}
	return results, nil
}

func executeMigration(db *sql.DB, filename string) error {
	content, err := migrationFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	return nil
}
