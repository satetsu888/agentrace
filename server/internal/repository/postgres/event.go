package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
)

type EventRepository struct {
	db *DB
}

func NewEventRepository(db *DB) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Create(ctx context.Context, event *domain.Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}

	// Use sql.NullString for optional uuid
	var uuidValue sql.NullString
	if event.UUID != "" {
		uuidValue = sql.NullString{String: event.UUID, Valid: true}
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO events (id, session_id, uuid, event_type, payload, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		event.ID, event.SessionID, uuidValue, event.EventType, payloadJSON, event.CreatedAt,
	)
	if err != nil {
		// Check for UNIQUE constraint violation (duplicate uuid)
		// PostgreSQL error code 23505 = unique_violation
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") {
			return repository.ErrDuplicateEvent
		}
		return err
	}
	return nil
}

func (r *EventRepository) CreateBatch(ctx context.Context, events []*domain.Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	// Build multi-row INSERT with ON CONFLICT DO NOTHING so duplicate uuids are
	// silently skipped. Use RETURNING id to count successful inserts.
	valueGroups := make([]string, 0, len(events))
	args := make([]any, 0, len(events)*6)
	argIdx := 1

	for _, event := range events {
		if event.ID == "" {
			event.ID = uuid.New().String()
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now()
		}

		payloadJSON, err := json.Marshal(event.Payload)
		if err != nil {
			return 0, err
		}

		var uuidValue sql.NullString
		if event.UUID != "" {
			uuidValue = sql.NullString{String: event.UUID, Valid: true}
		}

		placeholders := make([]string, 6)
		for i := 0; i < 6; i++ {
			placeholders[i] = "$" + strconv.Itoa(argIdx)
			argIdx++
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ",")+")")
		args = append(args, event.ID, event.SessionID, uuidValue, event.EventType, payloadJSON, event.CreatedAt)
	}

	query := `INSERT INTO events (id, session_id, uuid, event_type, payload, created_at)
		 VALUES ` + strings.Join(valueGroups, ",") + `
		 ON CONFLICT (session_id, uuid) DO NOTHING
		 RETURNING id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	inserted := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, rows.Err()
}

func (r *EventRepository) FindBySessionID(ctx context.Context, sessionID string) ([]*domain.Event, error) {
	// Order by payload->>'timestamp' if available, otherwise by created_at
	query := fmt.Sprintf(`SELECT id, session_id, uuid, event_type, payload, created_at
		 FROM events WHERE session_id = $1
		 AND (event_type IS NULL OR event_type NOT IN (%s))
		 ORDER BY COALESCE(payload->>'timestamp', created_at::text) ASC`, domain.HiddenEventTypesSQL())
	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.Event
	for rows.Next() {
		event, err := r.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (r *EventRepository) scanEvent(rows *sql.Rows) (*domain.Event, error) {
	var event domain.Event
	var uuidValue sql.NullString
	var payloadBytes []byte

	err := rows.Scan(&event.ID, &event.SessionID, &uuidValue, &event.EventType, &payloadBytes, &event.CreatedAt)
	if err != nil {
		return nil, err
	}

	if uuidValue.Valid {
		event.UUID = uuidValue.String
	}

	if err := json.Unmarshal(payloadBytes, &event.Payload); err != nil {
		// If unmarshal fails, use empty map
		event.Payload = make(map[string]interface{})
	}

	return &event, nil
}

func (r *EventRepository) CountBySessionID(ctx context.Context, sessionID string) (int, error) {
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM events
		 WHERE session_id = $1
		 AND (event_type IS NULL OR event_type NOT IN (%s))`, domain.HiddenEventTypesSQL())
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *EventRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM events WHERE session_id = $1`, sessionID)
	return err
}
