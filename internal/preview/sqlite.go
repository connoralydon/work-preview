package preview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func sqliteDSN(path string) string {
	return (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_pragma=busy_timeout%285000%29&_pragma=foreign_keys%281%29&_pragma=journal_mode%28WAL%29",
	}).String()
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) Create(ctx context.Context, p Preview) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO previews (id, prefix, port, status, created_at, last_access_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, p.ID, p.Prefix, p.Port, p.Status, p.CreatedAt, p.LastAccessAt, p.ExpiresAt)
	var sqliteErr interface{ Code() int }
	if errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == 19 {
		return ErrPrefixConflict
	}
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, p.ID, EventCreated, p.CreatedAt, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) Active(ctx context.Context) ([]Preview, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, prefix, port, status, created_at, last_access_at, expires_at
FROM previews WHERE status = 'active' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var previews []Preview
	for rows.Next() {
		var p Preview
		if err := rows.Scan(&p.ID, &p.Prefix, &p.Port, &p.Status, &p.CreatedAt, &p.LastAccessAt, &p.ExpiresAt); err != nil {
			return nil, err
		}
		previews = append(previews, p)
	}
	return previews, rows.Err()
}

func (s *SQLiteStore) GetActive(ctx context.Context, id string) (Preview, error) {
	var p Preview
	err := s.db.QueryRowContext(ctx, `
SELECT id, prefix, port, status, created_at, last_access_at, expires_at
FROM previews WHERE id = ? AND status = 'active'`, id).Scan(
		&p.ID, &p.Prefix, &p.Port, &p.Status, &p.CreatedAt, &p.LastAccessAt, &p.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Preview{}, ErrNotFound
	}
	return p, err
}

func (s *SQLiteStore) Touch(ctx context.Context, id string, accessedAt, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
UPDATE previews SET last_access_at = ?, expires_at = ? WHERE id = ? AND status = 'active'`, accessedAt, expiresAt, id)
	if err := checkedUpdate(result, err); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, id, EventAccessed, accessedAt, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) SetStatus(ctx context.Context, id, status string, endedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
UPDATE previews SET status = ?, ended_at = ? WHERE id = ? AND status = 'active'`, status, endedAt, id)
	if err := checkedUpdate(result, err); err != nil {
		return err
	}
	eventType := EventExpired
	if status == StatusDeleted {
		eventType = EventDeleted
	}
	if err := insertEvent(ctx, tx, id, eventType, endedAt, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) RecordEvent(ctx context.Context, id, eventType string, occurredAt time.Time, details string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO preview_events (preview_id, event_type, occurred_at, details)
VALUES (?, ?, ?, ?)`, id, eventType, occurredAt, nullableDetails(details))
	return err
}

type eventExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertEvent(ctx context.Context, execer eventExecer, id, eventType string, occurredAt time.Time, details string) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO preview_events (preview_id, event_type, occurred_at, details)
VALUES (?, ?, ?, ?)`, id, eventType, occurredAt, nullableDetails(details))
	return err
}

func nullableDetails(details string) any {
	if details == "" {
		return nil
	}
	return details
}

func checkedUpdate(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
