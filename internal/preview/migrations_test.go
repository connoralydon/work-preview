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
	if len(migrations) != 3 || migrations[0].version != 1 || migrations[0].name != "001_initial.sql" || migrations[1].version != 2 || migrations[1].name != "002_persistent.sql" || migrations[2].version != 3 || migrations[2].name != "003_remove_persistent.sql" {
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
	if version := schemaVersion(t, store.db); version != 3 {
		t.Fatalf("schema version=%d, want 3", version)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM previews WHERE id = 'existing'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("migration did not preserve existing preview")
	}
}

func TestMigrateRemovesPersistentPreviews(t *testing.T) {
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
	if err := applyMigrations(ctx, db, migrations[:2]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO previews (id, prefix, port, status, created_at, last_access_at, expires_at, persistent, boot_id)
VALUES
  ('persistent', 'persistent', 3000, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1, 'boot'),
  ('regular', 'regular', 3001, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0, '')`); err != nil {
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
	active, err := store.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "regular" {
		t.Fatalf("active previews after migration: %+v", active)
	}
	var status string
	if err := store.db.QueryRowContext(ctx, "SELECT status FROM previews WHERE id = 'persistent'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusExpired {
		t.Fatalf("persistent preview status=%q, want expired", status)
	}
	var columns int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('previews') WHERE name IN ('persistent', 'boot_id')").Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatalf("persistent schema columns remaining=%d", columns)
	}
	var events int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM preview_events WHERE preview_id = 'persistent' AND event_type = 'expired'").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("persistent preview expiry events=%d, want 1", events)
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
	if _, err := db.ExecContext(ctx, "PRAGMA user_version = 4"); err != nil {
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
