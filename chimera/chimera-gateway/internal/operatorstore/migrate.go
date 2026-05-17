package operatorstore

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

const bootstrapDDL = `CREATE TABLE IF NOT EXISTS operator_migrations (
	version INTEGER NOT NULL PRIMARY KEY
);`

var migrationFileRE = regexp.MustCompile(`^(\d{6})_.+\.sql$`)

// ApplyMigrations runs *.sql in migrationsDir whose version is not recorded in operator_migrations.
func ApplyMigrations(db *sql.DB, migrationsDir string, log *slog.Logger) error {
	if _, err := os.Stat(migrationsDir); err != nil {
		return fmt.Errorf("operator migrations dir: %w", err)
	}
	if _, err := db.Exec(bootstrapDDL); err != nil {
		return fmt.Errorf("operator migrations bootstrap: %w", err)
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read operator migrations: %w", err)
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
			return fmt.Errorf("operator: invalid migration filename %q (want 000001_name.sql)", name)
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
			return fmt.Errorf("operator migration version %q: %w", name, err)
		}
		if _, done := applied[v]; done {
			continue
		}
		body, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read operator migration %s: %w", name, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin operator migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec operator migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO operator_migrations (version) VALUES (?)`, v); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record operator migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit operator migration %s: %w", name, err)
		}
		if log != nil {
			log.Info("operator DB migration applied", "msg", "gateway.operator.migration_applied", "version", v, "file", name)
		}
		applied[v] = struct{}{}
	}
	return nil
}

func loadAppliedVersions(db *sql.DB) (map[int]struct{}, error) {
	rows, err := db.Query(`SELECT version FROM operator_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied operator migrations: %w", err)
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
