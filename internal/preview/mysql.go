package preview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

const schema = `
CREATE TABLE IF NOT EXISTS previews (
  id VARCHAR(32) PRIMARY KEY,
  prefix VARCHAR(63) NOT NULL,
  port INT UNSIGNED NOT NULL,
  status ENUM('active', 'deleted', 'expired') NOT NULL,
  created_at DATETIME(6) NOT NULL,
  last_access_at DATETIME(6) NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  ended_at DATETIME(6) NULL,
  active_prefix VARCHAR(63) GENERATED ALWAYS AS (
    CASE WHEN status = 'active' THEN prefix ELSE NULL END
  ) STORED UNIQUE,
  INDEX previews_active_expiry (status, expires_at)
) ENGINE=InnoDB;
`

type MySQLStore struct {
	db *sql.DB
}

func OpenMySQL(ctx context.Context, dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to mysql: %w", err)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize mysql schema: %w", err)
	}
	return &MySQLStore{db: db}, nil
}

func (s *MySQLStore) Close() error { return s.db.Close() }

func (s *MySQLStore) Create(ctx context.Context, p Preview) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO previews (id, prefix, port, status, created_at, last_access_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, p.ID, p.Prefix, p.Port, p.Status, p.CreatedAt, p.LastAccessAt, p.ExpiresAt)
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return ErrPrefixConflict
	}
	return err
}

func (s *MySQLStore) Active(ctx context.Context) ([]Preview, error) {
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

func (s *MySQLStore) GetActive(ctx context.Context, id string) (Preview, error) {
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

func (s *MySQLStore) Touch(ctx context.Context, id string, accessedAt, expiresAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE previews SET last_access_at = ?, expires_at = ? WHERE id = ? AND status = 'active'`, accessedAt, expiresAt, id)
	return checkedUpdate(result, err)
}

func (s *MySQLStore) SetStatus(ctx context.Context, id, status string, endedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE previews SET status = ?, ended_at = ? WHERE id = ? AND status = 'active'`, status, endedAt, id)
	return checkedUpdate(result, err)
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
