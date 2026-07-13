package preview

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type constraintError struct{}

func (constraintError) Error() string { return "unique constraint failed" }
func (constraintError) Code() int     { return 2067 }

func TestSQLiteStoreCreateMapsDuplicatePrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &SQLiteStore{db: db}
	now := time.Now().UTC()
	p := Preview{ID: "id", Prefix: "same", Port: 3000, Status: StatusActive, CreatedAt: now, LastAccessAt: now, ExpiresAt: now.Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO previews (id, prefix, port, status, created_at, last_access_at, expires_at)\nVALUES (?, ?, ?, ?, ?, ?, ?)")).
		WithArgs(p.ID, p.Prefix, p.Port, p.Status, p.CreatedAt, p.LastAccessAt, p.ExpiresAt).
		WillReturnError(constraintError{})
	mock.ExpectRollback()
	if err := store.Create(context.Background(), p); !errors.Is(err, ErrPrefixConflict) {
		t.Fatalf("got %v, want ErrPrefixConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStoreReadsAndUpdatesActivePreview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &SQLiteStore{db: db}
	created := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	expires := created.Add(time.Hour)
	rows := sqlmock.NewRows([]string{"id", "prefix", "port", "repository", "branch", "commit_hash", "status", "created_at", "last_access_at", "expires_at"}).
		AddRow("id", "feature", 3000, "work-preview", "main", "abc123", StatusActive, created, created, expires)
	mock.ExpectQuery("FROM previews WHERE status = 'active'").WillReturnRows(rows)
	previews, err := store.Active(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 || previews[0].Port != 3000 || previews[0].Prefix != "feature" {
		t.Fatalf("unexpected previews: %+v", previews)
	}
	accessed := created.Add(10 * time.Minute)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE previews SET last_access_at").WithArgs(accessed, accessed.Add(time.Hour), "id").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO preview_events").WithArgs("id", EventAccessed, accessed, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := store.Touch(context.Background(), "id", accessed, accessed.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStoreReturnsNotFoundForInactiveUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &SQLiteStore{db: db}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE previews SET status").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if err := store.SetStatus(context.Background(), "missing", StatusDeleted, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestSQLiteStoreCreatesPreviewAndEventInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &SQLiteStore{db: db}
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	p := Preview{ID: "id", Prefix: "feature", Port: 3000, Status: StatusActive, CreatedAt: now, LastAccessAt: now, ExpiresAt: now.Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO previews").
		WithArgs(p.ID, p.Prefix, p.Port, p.Status, p.CreatedAt, p.LastAccessAt, p.ExpiresAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO preview_events").
		WithArgs(p.ID, EventCreated, p.CreatedAt, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := store.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStoreRollsBackStateWhenEventInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &SQLiteStore{db: db}
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE previews SET status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO preview_events").WillReturnError(errors.New("event insert failed"))
	mock.ExpectRollback()
	if err := store.SetStatus(context.Background(), "id", StatusExpired, now); err == nil {
		t.Fatal("SetStatus unexpectedly succeeded")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteDSNUsesWALForeignKeysAndBusyTimeout(t *testing.T) {
	dsn, err := url.Parse(sqliteDSN("/var/lib/work-preview/work-preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	pragmas := dsn.Query()["_pragma"]
	for _, expected := range []string{"busy_timeout(5000)", "foreign_keys(1)", "journal_mode(WAL)"} {
		if !slices.Contains(pragmas, expected) {
			t.Fatalf("missing %q in SQLite DSN %q", expected, dsn)
		}
	}
	if dsn.Scheme != "file" || dsn.Path != "/var/lib/work-preview/work-preview.db" {
		t.Fatalf("unexpected SQLite database location: %s", dsn)
	}
}
