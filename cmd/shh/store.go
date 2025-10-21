package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Host struct {
	ID         int64
	Host       string
	Comment    string
	LastUsedAt sql.NullTime
	UseCount   int
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,host,comment,last_used_at,use_count
		FROM hosts
		ORDER BY CASE WHEN last_used_at IS NULL THEN 1 ELSE 0 END,
		         last_used_at DESC,
		         host ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.ID, &h.Host, &h.Comment, &h.LastUsedAt, &h.UseCount); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (s *Store) AddHost(ctx context.Context, host, comment string) error {
	normHost, err := normalizeHost(host)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO hosts(host,comment) VALUES(?,?)`,
		normHost, strings.TrimSpace(comment))
	return err
}

func (s *Store) UpdateHost(ctx context.Context, id int64, host, comment string) error {
	normHost, err := normalizeHost(host)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE hosts SET host=?, comment=? WHERE id=?`,
		normHost, strings.TrimSpace(comment), id)
	return err
}

func (s *Store) DeleteHost(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM hosts WHERE id=?`, id)
	return err
}

func (s *Store) MarkUsed(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosts SET use_count=use_count+1, last_used_at=? WHERE id=?`,
		time.Now().UTC(), id)
	return err
}

func (s *Store) GetMeta(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) ImportHost(ctx context.Context, host, comment string) error {
	err := s.AddHost(ctx, host, comment)
	if err == nil {
		return nil
	}
	if isUniqueConstraintError(err) {
		return nil
	}
	return fmt.Errorf("add host %q: %w", host, err)
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	const uniquePrefix = "UNIQUE constraint failed"
	return strings.Contains(err.Error(), uniquePrefix)
}
