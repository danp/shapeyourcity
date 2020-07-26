package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/danp/shapeyourcity"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

func NewDB(filename string) (*DB, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, fmt.Errorf("opening database file %q: %w", filename, err)
	}

	st := &DB{db: db}
	if err := st.init(); err != nil {
		db.Close()
		return nil, err
	}

	return st, nil
}

func (s *DB) init() error {
	var err error

	_, err = s.db.Exec("create table if not exists markers (id text not null primary key, url text, address text, category text, created_at datetime, editable bool, lat text, lng text, user text)")
	if err != nil {
		return fmt.Errorf("creating markers table: %w", err)
	}
	_, err = s.db.Exec("create table if not exists responses (marker_id integer not null, mode text not null, question_type text not null, question text not null, answer text, foreign key(marker_id) references markers(id))")
	if err != nil {
		return fmt.Errorf("creating responses table: %w", err)
	}

	return nil
}

func (s *DB) Markers(ctx context.Context) ([]shapeyourcity.Marker, error) {
	rows, err := s.db.QueryContext(ctx, "select id, url, address, category, created_at, editable, lat, lng, user, mode, question_type, question, answer from markers left outer join responses on (marker_id=id)")
	if err != nil {
		return nil, fmt.Errorf("loading stored markers: %w", err)
	}

	var (
		out []shapeyourcity.Marker
		cm  shapeyourcity.Marker
	)
	for rows.Next() {
		var (
			m shapeyourcity.Marker
			r shapeyourcity.MarkerResponse
		)

		if err := rows.Scan(
			&m.ID,
			&m.URL,
			&m.Address,
			&m.Category,
			&m.CreatedAt,
			&m.Editable,
			&m.Latitude,
			&m.Longitude,
			&m.User,
			&r.Mode,
			&r.QuestionType,
			&r.Question,
			&r.Answer,
		); err != nil {
			return nil, fmt.Errorf("loading stored markers: %w", err)
		}

		if m.ID != cm.ID {
			if cm.User != "" {
				out = append(out, cm)
			}
			cm = m
		}

		cm.Responses = append(cm.Responses, r)
	}

	if cm.User != "" {
		out = append(out, cm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("loading stored markers: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("loading stored markers: %w", err)
	}

	return out, nil
}

func (s *DB) Sync(ctx context.Context, marker shapeyourcity.Marker) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning sync tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "delete from responses where marker_id=?", marker.ID); err != nil {
		return fmt.Errorf("syncing marker: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "delete from markers where id=?", marker.ID); err != nil {
		return fmt.Errorf("syncing marker: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		"insert into markers (id, url, address, category, created_at, editable, lat, lng, user) values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		marker.ID,
		marker.URL,
		marker.Address,
		marker.Category,
		marker.CreatedAt.UTC(),
		marker.Editable,
		marker.Latitude,
		marker.Longitude,
		marker.User,
	)
	if err != nil {
		return fmt.Errorf("adding marker %d: %w", marker.ID, err)
	}

	for _, r := range marker.Responses {
		_, err := tx.ExecContext(ctx, "insert into responses (marker_id, mode, question_type, question, answer) values (?, ?, ?, ?, ?)", marker.ID, r.Mode, r.QuestionType, r.Question, r.Answer)

		if err != nil {
			return fmt.Errorf("syncing marker %d response: %w", marker.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("syncing marker %d: %w", marker.ID, err)
	}

	return nil
}

func (s *DB) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
