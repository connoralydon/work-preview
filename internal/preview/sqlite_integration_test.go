package preview

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteLifecycleEventsAndPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work-preview.db")
	store, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if version := schemaVersion(t, store.db); version != 3 {
		t.Fatalf("schema version=%d, want 3", version)
	}
	created := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	first := Preview{
		ID: "first", Prefix: "feature", Port: 3000, Status: StatusActive,
		CreatedAt: created, LastAccessAt: created, ExpiresAt: created.Add(time.Hour),
	}
	if err := store.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	accessed := created.Add(10 * time.Minute)
	if err := store.Touch(ctx, first.ID, accessed, accessed.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(ctx, first.ID, StatusDeleted, accessed.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	second := first
	second.ID = "second"
	second.Port = 3001
	second.CreatedAt = accessed.Add(2 * time.Minute)
	second.LastAccessAt = second.CreatedAt
	second.ExpiresAt = second.CreatedAt.Add(time.Hour)
	if err := store.Create(ctx, second); err != nil {
		t.Fatalf("reuse prefix after deletion: %v", err)
	}
	if err := store.RecordEvent(ctx, second.ID, EventReloadFailed, second.CreatedAt, `{"error":"test"}`); err != nil {
		t.Fatal(err)
	}

	var journalMode string
	if err := store.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode=%q, want wal", journalMode)
	}
	var foreignKeys int
	if err := store.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d, want 1", foreignKeys)
	}
	var firstEvents int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM preview_events WHERE preview_id = ?", first.ID).Scan(&firstEvents); err != nil {
		t.Fatal(err)
	}
	if firstEvents != 3 {
		t.Fatalf("first preview events=%d, want created, accessed, deleted", firstEvents)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	active, err := reopened.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active previews after reopen: %+v", active)
	}
	var secondEvents int
	if err := reopened.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM preview_events WHERE preview_id = ?", second.ID).Scan(&secondEvents); err != nil {
		t.Fatal(err)
	}
	if secondEvents != 2 {
		t.Fatalf("second preview events=%d, want created and reload_failed", secondEvents)
	}
}
