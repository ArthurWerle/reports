package migrations

import (
	"embed"
	"log/slog"
	"sort"

	"gorm.io/gorm"
)

//go:embed *.sql
var migrationFiles embed.FS

// RunMigrations applies each embedded SQL file exactly once, in filename
// order, recording applied files in schema_migrations. Each file runs inside
// a transaction together with its version record, so a failed migration
// leaves no partial state and is retried on the next boot.
func RunMigrations(db *gorm.DB, logger *slog.Logger) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		logger.Error("failed to create schema_migrations table", "error", err)
		return err
	}

	var appliedFiles []string
	if err := db.Raw("SELECT filename FROM schema_migrations").Scan(&appliedFiles).Error; err != nil {
		logger.Error("failed to read schema_migrations", "error", err)
		return err
	}
	applied := make(map[string]bool, len(appliedFiles))
	for _, f := range appliedFiles {
		applied[f] = true
	}

	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		logger.Error("failed to read migration directory", "error", err)
		return err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, file := range files {
		if applied[file] {
			continue
		}

		sqlContent, err := migrationFiles.ReadFile(file)
		if err != nil {
			logger.Error("failed to read migration file", "file", file, "error", err)
			return err
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(sqlContent)).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO schema_migrations (filename) VALUES (?)", file).Error
		})
		if err != nil {
			logger.Error("failed to execute migration", "file", file, "error", err)
			return err
		}

		logger.Info("migration applied", "file", file)
	}

	logger.Info("migrations up to date")
	return nil
}
