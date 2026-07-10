package preview

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestLoadMigrationsIsContiguous(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].version != 1 || migrations[0].name != "001_initial.sql" {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
}

func TestMigrateAdoptsExistingVersionZeroSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work-preview.db")
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, migrations[0].sql); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO previews (id, prefix, port, status, created_at, last_access_at, expires_at)
VALUES ('existing', 'existing', 3000, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if version := schemaVersion(t, store.db); version != 1 {
		t.Fatalf("schema version=%d, want 1", version)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM previews WHERE id = 'existing'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("migration did not preserve existing preview")
	}
}

func TestMigrationFailureRollsBackAndKeepsPreviousVersion(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "work-preview.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations := []migration{
		{version: 1, name: "001_valid.sql", sql: "CREATE TABLE first_table (id INTEGER);"},
		{version: 2, name: "002_invalid.sql", sql: "CREATE TABLE rolled_back (id INTEGER); INVALID SQL;"},
	}
	if err := applyMigrations(ctx, db, migrations); err == nil {
		t.Fatal("applyMigrations unexpectedly succeeded")
	}
	if version := schemaVersion(t, db); version != 1 {
		t.Fatalf("schema version=%d, want 1", version)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'rolled_back'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration was not rolled back")
	}
}

func TestMigrateRejectsNewerDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work-preview.db")
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := OpenSQLite(ctx, path); !errors.Is(err, ErrSchemaTooNew) {
		if store != nil {
			store.Close()
		}
		t.Fatalf("OpenSQLite error=%v, want ErrSchemaTooNew", err)
	}
}

func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}
