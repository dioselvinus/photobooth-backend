package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

//go:embed *.sql
var MigrationFS embed.FS

// RunMigrations automatically applies all .up.sql migrations in order and tracks applied migrations in schema_migrations.
func RunMigrations(db *sql.DB) error {
	// Create schema_migrations table if it doesn't exist
	trackingTableQuery := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.Exec(trackingTableQuery); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	entries, err := MigrationFS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read migrations dir: %w", err)
	}

	var upFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			upFiles = append(upFiles, entry.Name())
		}
	}
	sort.Strings(upFiles)

	for _, file := range upFiles {
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`
		if err := db.QueryRow(checkQuery, file).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", file, err)
		}

		if exists {
			slog.Debug("Migration already applied, skipping", "file", file)
			continue
		}

		content, err := MigrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		slog.Info("Applying database migration", "file", file)

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", file, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", file, err)
		}

		recordQuery := `INSERT INTO schema_migrations (name) VALUES ($1)`
		if _, err := tx.Exec(recordQuery, file); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s in schema_migrations: %w", file, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration transaction for %s: %w", file, err)
		}
	}

	slog.Info("All database migrations verified and up to date")
	return nil
}
