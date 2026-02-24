package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const migrationLockID = 982451653

func Run(ctx context.Context, db *sql.DB, dir string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		id VARCHAR(100) PRIMARY KEY,
		file_name TEXT NOT NULL,
		executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		duration_ms INTEGER NOT NULL
	)`); err != nil {
		return err
	}

	var gotLock bool
	if err := db.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, migrationLockID).Scan(&gotLock); err != nil {
		return err
	}
	if !gotLock {
		return fmt.Errorf("another migration process is running")
	}
	defer db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			files = append(files, name)
		}
	}
	sort.Strings(files)

	for _, file := range files {
		id := strings.TrimSuffix(file, filepath.Ext(file))
		var exists int
		err := db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE id = $1`, id).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}

		sqlBytes, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return err
		}
		started := time.Now()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", file, err)
		}
		dur := time.Since(started).Milliseconds()
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (id, file_name, duration_ms) VALUES ($1,$2,$3)`, id, file, dur); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
