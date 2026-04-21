package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
)

type EventRepository struct {
	mu     sync.RWMutex
	events map[string]*domain.Event
	// uuidIndex maps "session_id:uuid" -> event.ID to detect duplicates
	uuidIndex map[string]string
}

func NewEventRepository() *EventRepository {
	return &EventRepository{
		events:    make(map[string]*domain.Event),
		uuidIndex: make(map[string]string),
	}
}

func (r *EventRepository) Create(ctx context.Context, event *domain.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.createLocked(event)
}

func (r *EventRepository) createLocked(event *domain.Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	// Check for duplicate uuid within the same session
	if event.UUID != "" {
		indexKey := event.SessionID + ":" + event.UUID
		if _, exists := r.uuidIndex[indexKey]; exists {
			return repository.ErrDuplicateEvent
		}
		r.uuidIndex[indexKey] = event.ID
	}

	r.events[event.ID] = event
	return nil
}

func (r *EventRepository) CreateBatch(ctx context.Context, events []*domain.Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	inserted := 0
	for _, event := range events {
		err := r.createLocked(event)
		if err == nil {
			inserted++
			continue
		}
		if err == repository.ErrDuplicateEvent {
			continue
		}
		return inserted, err
	}
	return inserted, nil
}

// getTimestampFromPayload extracts timestamp from payload, falls back to CreatedAt
func getTimestampFromPayload(e *domain.Event) time.Time {
	if ts, ok := e.Payload["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return parsed
		}
		// Try parsing without timezone
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", ts); err == nil {
			return parsed
		}
	}
	return e.CreatedAt
}

func (r *EventRepository) FindBySessionID(ctx context.Context, sessionID string) ([]*domain.Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	events := make([]*domain.Event, 0)
	for _, e := range r.events {
		if e.SessionID == sessionID && !domain.IsHiddenEventType(e.EventType) {
			events = append(events, e)
		}
	}

	// Sort by payload.timestamp ascending (oldest first)
	sort.Slice(events, func(i, j int) bool {
		return getTimestampFromPayload(events[i]).Before(getTimestampFromPayload(events[j]))
	})

	return events, nil
}

func (r *EventRepository) CountBySessionID(ctx context.Context, sessionID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, e := range r.events {
		if e.SessionID == sessionID && !domain.IsHiddenEventType(e.EventType) {
			count++
		}
	}

	return count, nil
}

func (r *EventRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Collect IDs of events to delete and their uuid index keys
	for id, e := range r.events {
		if e.SessionID == sessionID {
			// Remove from uuid index if present
			if e.UUID != "" {
				indexKey := e.SessionID + ":" + e.UUID
				delete(r.uuidIndex, indexKey)
			}
			delete(r.events, id)
		}
	}

	return nil
}
