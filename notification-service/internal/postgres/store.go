package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Store records processed event IDs for idempotent consumption.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// WasProcessed reports whether this event_id was already handled (duplicate delivery).
func (s *Store) WasProcessed(ctx context.Context, eventID string) (bool, error) {
	if eventID == "" {
		return false, fmt.Errorf("empty event_id")
	}
	var stub int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM processed_events WHERE event_id = $1`, eventID).Scan(&stub)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RecordProcessed persists event_id after the side effect (log) succeeds.
func (s *Store) RecordProcessed(ctx context.Context, eventID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO processed_events (event_id) VALUES ($1)`, eventID)
	return err
}
