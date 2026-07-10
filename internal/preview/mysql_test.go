package preview

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestMySQLStoreCreateMapsDuplicatePrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &MySQLStore{db: db}
	now := time.Now().UTC()
	p := Preview{ID: "id", Prefix: "same", Port: 3000, Status: StatusActive, CreatedAt: now, LastAccessAt: now, ExpiresAt: now.Add(time.Hour)}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO previews (id, prefix, port, status, created_at, last_access_at, expires_at)\nVALUES (?, ?, ?, ?, ?, ?, ?)")).
		WithArgs(p.ID, p.Prefix, p.Port, p.Status, p.CreatedAt, p.LastAccessAt, p.ExpiresAt).
		WillReturnError(&mysqlDriver.MySQLError{Number: 1062, Message: "duplicate active_prefix"})
	if err := store.Create(context.Background(), p); !errors.Is(err, ErrPrefixConflict) {
		t.Fatalf("got %v, want ErrPrefixConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLStoreReadsAndUpdatesActivePreview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &MySQLStore{db: db}
	created := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	expires := created.Add(time.Hour)
	rows := sqlmock.NewRows([]string{"id", "prefix", "port", "status", "created_at", "last_access_at", "expires_at"}).
		AddRow("id", "feature", 3000, StatusActive, created, created, expires)
	mock.ExpectQuery("FROM previews WHERE status = 'active'").WillReturnRows(rows)
	previews, err := store.Active(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 || previews[0].Port != 3000 || previews[0].Prefix != "feature" {
		t.Fatalf("unexpected previews: %+v", previews)
	}
	accessed := created.Add(10 * time.Minute)
	mock.ExpectExec("UPDATE previews SET last_access_at").WithArgs(accessed, accessed.Add(time.Hour), "id").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.Touch(context.Background(), "id", accessed, accessed.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLStoreReturnsNotFoundForInactiveUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &MySQLStore{db: db}
	mock.ExpectExec("UPDATE previews SET status").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := store.SetStatus(context.Background(), "missing", StatusDeleted, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
