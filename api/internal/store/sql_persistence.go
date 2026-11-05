package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type SQLPersistence struct {
	db        *sql.DB
	once      sync.Once
	initError error
}

func (p *SQLPersistence) enabled() bool { return p != nil }

func NewSQLPersistenceFromEnv() *SQLPersistence {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	if dsn == "" {
		return nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &SQLPersistence{db: db}
}

func (p *SQLPersistence) ensure(ctx context.Context) error {
	p.once.Do(func() {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS app_documents (
				kind VARCHAR(64) NOT NULL,
				id VARCHAR(191) NOT NULL,
				payload MEDIUMBLOB NOT NULL,
				updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
				PRIMARY KEY (kind, id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS app_document_indexes (
				kind VARCHAR(64) NOT NULL,
				index_key VARCHAR(255) NOT NULL,
				id VARCHAR(191) NOT NULL,
				PRIMARY KEY (kind, index_key, id),
				INDEX idx_document (kind, id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS app_idempotency (
				key_hash CHAR(64) NOT NULL PRIMARY KEY,
				task_id VARCHAR(191) NOT NULL,
				payload_hash CHAR(64) NOT NULL,
				expires_at DATETIME(6) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS app_completed_jobs (
				key_hash CHAR(64) NOT NULL PRIMARY KEY,
				expires_at DATETIME(6) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := p.db.ExecContext(ctx, statement); err != nil {
				p.initError = err
				return
			}
		}
	})
	return p.initError
}

func sqlContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func (p *SQLPersistence) save(kind, id string, value any, indexes ...string) error {
	if p == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return err
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO app_documents (kind, id, payload) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE payload=VALUES(payload), updated_at=CURRENT_TIMESTAMP(6)`,
		kind, id, encoded,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_document_indexes WHERE kind=? AND id=?`, kind, id); err != nil {
		return err
	}
	for _, index := range indexes {
		if index == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT IGNORE INTO app_document_indexes (kind, index_key, id) VALUES (?, ?, ?)`,
			kind, index, id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *SQLPersistence) load(kind, id string, dst any) error {
	if p == nil {
		return ErrNotFound
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return err
	}
	var encoded []byte
	err := p.db.QueryRowContext(ctx, `SELECT payload FROM app_documents WHERE kind=? AND id=?`, kind, id).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, dst)
}

func (p *SQLPersistence) list(kind, index string, factory func() any) ([]any, error) {
	if p == nil {
		return nil, nil
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT d.payload
		   FROM app_documents d
		   JOIN app_document_indexes i ON i.kind=d.kind AND i.id=d.id
		  WHERE i.kind=? AND i.index_key=?
		  ORDER BY d.updated_at DESC`,
		kind, index,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []any
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		value := factory()
		if err := json.Unmarshal(encoded, value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (p *SQLPersistence) markJobDone(key string) error {
	if p == nil || key == "" {
		return nil
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO app_completed_jobs (key_hash, expires_at)
		 VALUES (?, DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 7 DAY))
		 ON DUPLICATE KEY UPDATE expires_at=VALUES(expires_at)`,
		Digest(key),
	)
	return err
}

func (p *SQLPersistence) jobDone(key string) (bool, error) {
	if p == nil || key == "" {
		return false, nil
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return false, err
	}
	var count int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM app_completed_jobs WHERE key_hash=? AND expires_at>UTC_TIMESTAMP(6)`,
		Digest(key),
	).Scan(&count)
	return count > 0, err
}

func (p *SQLPersistence) claimIdempotency(composite, payloadHash, taskID string) (string, bool, error) {
	if p == nil {
		return "", false, nil
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if err := p.ensure(ctx); err != nil {
		return "", false, err
	}
	keyHash := Digest(composite)
	_, _ = p.db.ExecContext(ctx, `DELETE FROM app_idempotency WHERE key_hash=? AND expires_at<=UTC_TIMESTAMP(6)`, keyHash)
	result, err := p.db.ExecContext(ctx,
		`INSERT IGNORE INTO app_idempotency (key_hash, task_id, payload_hash, expires_at)
		 VALUES (?, ?, ?, DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 7 DAY))`,
		keyHash, taskID, payloadHash,
	)
	if err != nil {
		return "", false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if affected == 1 {
		return taskID, false, nil
	}
	var existing idempotencyRecord
	if err := p.db.QueryRowContext(ctx,
		`SELECT task_id, payload_hash FROM app_idempotency WHERE key_hash=?`,
		keyHash,
	).Scan(&existing.TaskID, &existing.PayloadHash); err != nil {
		return "", true, err
	}
	if existing.PayloadHash != payloadHash {
		return existing.TaskID, true, ErrConflict
	}
	return existing.TaskID, true, nil
}

func (p *SQLPersistence) releaseIdempotency(composite string) {
	if p == nil {
		return
	}
	ctx, cancel := sqlContext()
	defer cancel()
	if p.ensure(ctx) != nil {
		return
	}
	_, _ = p.db.ExecContext(ctx, `DELETE FROM app_idempotency WHERE key_hash=?`, Digest(composite))
}
