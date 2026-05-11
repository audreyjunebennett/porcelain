package gatewaymetrics

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const bootstrapDDL = `CREATE TABLE IF NOT EXISTS gateway_migrations (
	version INTEGER NOT NULL PRIMARY KEY
);`

var migrationFileRE = regexp.MustCompile(`^(\d{6})_.+\.sql$`)

// ApplyMigrations runs any *.sql files in migrationsDir whose leading numeric version is not yet
// recorded in gateway_migrations. Filenames must be like 000001_description.sql. SQL is executed
// as one script per file (multiple statements allowed). Intended for process startup only.
func ApplyMigrations(db *sql.DB, migrationsDir string, log *slog.Logger) error {
	if _, err := os.Stat(migrationsDir); err != nil {
		return fmt.Errorf("gateway metrics migrations dir: %w", err)
	}
	if _, err := db.Exec(bootstrapDDL); err != nil {
		return fmt.Errorf("gateway metrics bootstrap: %w", err)
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		if !migrationFileRE.MatchString(name) {
			return fmt.Errorf("gateway metrics: invalid migration filename %q (want 000001_name.sql)", name)
		}
		files = append(files, name)
	}
	sort.Strings(files)

	applied, err := loadAppliedVersions(db)
	if err != nil {
		return err
	}

	for _, name := range files {
		sub := migrationFileRE.FindStringSubmatch(name)
		if len(sub) != 2 {
			continue
		}
		v, err := strconv.Atoi(sub[1])
		if err != nil {
			return fmt.Errorf("migration version %q: %w", name, err)
		}
		if _, done := applied[v]; done {
			continue
		}
		body, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO gateway_migrations (version) VALUES (?)`, v); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		if log != nil {
			log.Info("gateway metrics migration applied", "msg", "gateway.metrics.migration_applied", "version", v, "file", name)
		}
		applied[v] = struct{}{}
	}
	return nil
}

func loadAppliedVersions(db *sql.DB) (map[int]struct{}, error) {
	rows, err := db.Query(`SELECT version FROM gateway_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	out := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}
