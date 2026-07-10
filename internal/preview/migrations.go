package preview

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrSchemaTooNew = errors.New("sqlite schema is newer than this binary")

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		separator := strings.IndexByte(entry.Name(), '_')
		if separator < 1 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(entry.Name()[:separator])
		if err != nil {
			return nil, fmt.Errorf("invalid migration filename %q: %w", entry.Name(), err)
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(contents)})
	}
	for index, migration := range migrations {
		if migration.version != index+1 {
			return nil, fmt.Errorf("migration %q has version %d, want %d", migration.name, migration.version, index+1)
		}
	}
	return migrations, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load sqlite migrations: %w", err)
	}
	return applyMigrations(ctx, db, migrations)
}

func applyMigrations(ctx context.Context, db *sql.DB, migrations []migration) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if current > len(migrations) {
		return fmt.Errorf("%w: database=%d binary=%d", ErrSchemaTooNew, current, len(migrations))
	}
	for _, migration := range migrations[current:] {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", migration.version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", migration.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", migration.name, err)
		}
	}
	return nil
}
