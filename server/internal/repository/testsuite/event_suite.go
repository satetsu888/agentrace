package testsuite

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
	"github.com/stretchr/testify/suite"
)

// EventRepositorySuite tests EventRepository implementations
type EventRepositorySuite struct {
	suite.Suite
	Repo        repository.EventRepository
	SessionRepo repository.SessionRepository // Optional: for FK constraint support
	Cleanup     func()
}

// createTestSession creates a session for FK constraint tests and returns the auto-generated ID
func (s *EventRepositorySuite) createTestSession(claudeSessionSuffix string) string {
	if s.SessionRepo == nil {
		return ""
	}
	ctx := context.Background()
	session := &domain.Session{
		ClaudeSessionID: "claude-" + claudeSessionSuffix,
	}
	err := s.SessionRepo.Create(ctx, session)
	if err != nil {
		return ""
	}
	return session.ID
}

func (s *EventRepositorySuite) TearDownTest() {
	if s.Cleanup != nil {
		s.Cleanup()
	}
}

func (s *EventRepositorySuite) TestCreate() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-create")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	event := &domain.Event{
		SessionID: sessionID,
		UUID:      uuid.New().String(),
		EventType: "message",
		Payload: map[string]interface{}{
			"content": "Hello, world!",
		},
	}

	err := s.Repo.Create(ctx, event)
	s.Require().NoError(err)

	// ID should be auto-generated
	s.NotEmpty(event.ID)

	// CreatedAt should be set
	s.False(event.CreatedAt.IsZero())
}

func (s *EventRepositorySuite) TestCreate_DuplicateUUID() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-dup")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	duplicateUUID := uuid.New().String()

	// Create first event
	event1 := &domain.Event{
		SessionID: sessionID,
		UUID:      duplicateUUID,
		EventType: "message",
		Payload:   map[string]interface{}{},
	}
	err := s.Repo.Create(ctx, event1)
	s.Require().NoError(err)

	// Try to create second event with same UUID in same session
	event2 := &domain.Event{
		SessionID: sessionID,
		UUID:      duplicateUUID,
		EventType: "message",
		Payload:   map[string]interface{}{},
	}
	err = s.Repo.Create(ctx, event2)
	s.Require().Error(err)
	s.ErrorIs(err, repository.ErrDuplicateEvent)
}

func (s *EventRepositorySuite) TestCreate_SameUUIDDifferentSession() {
	ctx := context.Background()

	sessionA := s.createTestSession("event-a")
	sessionB := s.createTestSession("event-b")
	if sessionA == "" || sessionB == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	sameUUID := uuid.New().String()

	// Create event in first session
	event1 := &domain.Event{
		SessionID: sessionA,
		UUID:      sameUUID,
		EventType: "message",
		Payload:   map[string]interface{}{},
	}
	err := s.Repo.Create(ctx, event1)
	s.Require().NoError(err)

	// Create event with same UUID in different session - should succeed
	event2 := &domain.Event{
		SessionID: sessionB,
		UUID:      sameUUID,
		EventType: "message",
		Payload:   map[string]interface{}{},
	}
	err = s.Repo.Create(ctx, event2)
	s.Require().NoError(err)
}

func (s *EventRepositorySuite) TestFindBySessionID() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-find")
	otherSessionID := s.createTestSession("event-other")
	if sessionID == "" || otherSessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Create multiple events
	for i := 0; i < 5; i++ {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: "message",
			Payload:   map[string]interface{}{"index": i},
			CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Create event for different session
	otherEvent := &domain.Event{
		SessionID: otherSessionID,
		UUID:      uuid.New().String(),
		EventType: "message",
		Payload:   map[string]interface{}{},
	}
	err := s.Repo.Create(ctx, otherEvent)
	s.Require().NoError(err)

	// Find events for session
	events, err := s.Repo.FindBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Len(events, 5)

	// All events should belong to the session
	for _, e := range events {
		s.Equal(sessionID, e.SessionID)
	}
}

func (s *EventRepositorySuite) TestFindBySessionID_ChronologicalOrder() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-chrono")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	baseTime := time.Now()

	// Create events in non-sequential order but with specific timestamps
	// We'll create them out of order to ensure sorting is by CreatedAt, not insertion order
	timestamps := []time.Duration{
		300 * time.Millisecond, // third
		100 * time.Millisecond, // first
		500 * time.Millisecond, // fifth
		200 * time.Millisecond, // second
		400 * time.Millisecond, // fourth
	}

	for i, offset := range timestamps {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: "message",
			Payload:   map[string]interface{}{"order": i},
			CreatedAt: baseTime.Add(offset),
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Find events
	events, err := s.Repo.FindBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Require().Len(events, 5)

	// Verify events are in chronological order (ascending by CreatedAt)
	for i := 1; i < len(events); i++ {
		s.True(
			events[i-1].CreatedAt.Before(events[i].CreatedAt) || events[i-1].CreatedAt.Equal(events[i].CreatedAt),
			"Events should be in chronological order: event[%d].CreatedAt=%v should be <= event[%d].CreatedAt=%v",
			i-1, events[i-1].CreatedAt, i, events[i].CreatedAt,
		)
	}

	// Also verify the first and last events have the expected timestamps
	// Use WithinDuration to allow for database precision differences (PostgreSQL has microsecond precision)
	s.WithinDuration(baseTime.Add(100*time.Millisecond), events[0].CreatedAt, time.Microsecond, "First event should have earliest timestamp")
	s.WithinDuration(baseTime.Add(500*time.Millisecond), events[4].CreatedAt, time.Microsecond, "Last event should have latest timestamp")
}

// TestFindBySessionID_OrdersByPayloadTimestamp verifies that events are ordered by
// the transcript line's own payload.timestamp, NOT by created_at (server receive time).
//
// This guards against backend drift: SQLite/Postgres/turso/memory already sort by
// payload.timestamp, while DynamoDB used to sort by created_at. Under asynchronous /
// out-of-order delivery the two disagree, so we deliberately insert events whose
// created_at order is the REVERSE of their payload.timestamp order.
func (s *EventRepositorySuite) TestFindBySessionID_OrdersByPayloadTimestamp() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-payload-ts-order")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Second precision + UTC "Z" keeps the timestamp string lexically ordered,
	// which Postgres relies on (ORDER BY payload->>'timestamp'), while Go-based
	// backends parse it with RFC3339Nano. Fixed width avoids fractional-trim pitfalls.
	baseTime := time.Now().UTC().Truncate(time.Second)

	const n = 5
	expectedTimestamps := make([]string, n)
	for k := 0; k < n; k++ {
		// payload.timestamp ascends with k; created_at descends with k (reverse order).
		ts := baseTime.Add(time.Duration(k) * time.Second).Format(time.RFC3339Nano)
		expectedTimestamps[k] = ts
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: "message",
			Payload: map[string]interface{}{
				"timestamp": ts,
				"marker":    k,
			},
			CreatedAt: baseTime.Add(time.Duration(n-k) * time.Hour),
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	events, err := s.Repo.FindBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Require().Len(events, n)

	// Returned order must follow payload.timestamp ascending, i.e. the insertion (created_at)
	// order reversed. If a backend sorted by created_at, this would come back descending and fail.
	for i := 0; i < n; i++ {
		ts, ok := events[i].Payload["timestamp"].(string)
		s.Require().True(ok, "event[%d] should carry a string payload.timestamp", i)
		s.Equal(expectedTimestamps[i], ts,
			"events should be ordered by payload.timestamp ascending (position %d)", i)
	}
}

func (s *EventRepositorySuite) TestFindBySessionID_Empty() {
	ctx := context.Background()

	// Use a valid UUID format for non-existing session
	nonExistingID := uuid.New().String()
	events, err := s.Repo.FindBySessionID(ctx, nonExistingID)
	s.Require().NoError(err)
	s.Empty(events)
}

func (s *EventRepositorySuite) TestCountBySessionID() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-count")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Create multiple events
	for i := 0; i < 7; i++ {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: "message",
			Payload:   map[string]interface{}{},
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	count, err := s.Repo.CountBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Equal(7, count)
}

func (s *EventRepositorySuite) TestCountBySessionID_Empty() {
	ctx := context.Background()

	// Use a valid UUID format for non-existing session
	nonExistingID := uuid.New().String()
	count, err := s.Repo.CountBySessionID(ctx, nonExistingID)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *EventRepositorySuite) TestDeleteBySessionID() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-delete")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Create multiple events
	for i := 0; i < 5; i++ {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: "message",
			Payload:   map[string]interface{}{"index": i},
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Verify events exist
	count, err := s.Repo.CountBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Equal(5, count)

	// Delete all events for the session
	err = s.Repo.DeleteBySessionID(ctx, sessionID)
	s.Require().NoError(err)

	// Verify events no longer exist
	count, err = s.Repo.CountBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *EventRepositorySuite) TestDeleteBySessionID_NoEvents() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-delete-empty")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Delete events for session with no events should not error
	err := s.Repo.DeleteBySessionID(ctx, sessionID)
	s.NoError(err)
}

func (s *EventRepositorySuite) TestFindBySessionID_FiltersHiddenEventTypes() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-hidden")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Create visible events
	visibleTypes := []string{"message", "user", "assistant", "tool_use", "tool_result"}
	for i, eventType := range visibleTypes {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: eventType,
			Payload:   map[string]interface{}{"index": i},
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Create hidden events (should be filtered out)
	for i, eventType := range domain.HiddenEventTypes {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: eventType,
			Payload:   map[string]interface{}{"hidden_index": i},
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Find events - should only return visible events
	events, err := s.Repo.FindBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Len(events, len(visibleTypes), "Should only return visible events, not hidden ones")

	// Verify no hidden event types in results
	for _, e := range events {
		s.False(domain.IsHiddenEventType(e.EventType), "Event type %s should not be in results", e.EventType)
	}
}

func (s *EventRepositorySuite) TestCountBySessionID_ExcludesHiddenEventTypes() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-count-hidden")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	// Create visible events
	visibleCount := 3
	for i := 0; i < visibleCount; i++ {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: "message",
			Payload:   map[string]interface{}{},
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Create hidden events (should not be counted)
	for _, eventType := range domain.HiddenEventTypes {
		event := &domain.Event{
			SessionID: sessionID,
			UUID:      uuid.New().String(),
			EventType: eventType,
			Payload:   map[string]interface{}{},
		}
		err := s.Repo.Create(ctx, event)
		s.Require().NoError(err)
	}

	// Count should only include visible events
	count, err := s.Repo.CountBySessionID(ctx, sessionID)
	s.Require().NoError(err)
	s.Equal(visibleCount, count, "Count should only include visible events, not hidden ones")
}

func (s *EventRepositorySuite) TestCreate_DuplicateUUID_Concurrent() {
	ctx := context.Background()

	sessionID := s.createTestSession("event-concurrent")
	if sessionID == "" {
		s.T().Skip("SessionRepo not available, skipping test")
	}

	duplicateUUID := uuid.New().String()
	concurrency := 10

	var wg sync.WaitGroup
	results := make(chan error, concurrency)

	// Launch concurrent event creations with same UUID
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := &domain.Event{
				SessionID: sessionID,
				UUID:      duplicateUUID,
				EventType: "message",
				Payload:   map[string]interface{}{},
			}
			err := s.Repo.Create(ctx, event)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	// Count successes, duplicate errors, and busy errors (SQLite lock contention)
	successCount := 0
	duplicateCount := 0
	busyCount := 0
	var otherErrors []error

	for err := range results {
		if err == nil {
			successCount++
		} else if err == repository.ErrDuplicateEvent {
			duplicateCount++
		} else if isSQLiteBusyError(err) {
			// SQLite/Turso may return SQLITE_BUSY under heavy concurrent writes
			// This is expected behavior for file-based SQLite databases
			busyCount++
		} else {
			otherErrors = append(otherErrors, err)
		}
	}

	s.T().Logf("Results: success=%d, duplicate=%d, busy=%d, other=%d", successCount, duplicateCount, busyCount, len(otherErrors))

	// Exactly 1 should succeed, rest should be duplicates or busy errors
	s.Equal(1, successCount, "exactly 1 concurrent create should succeed")
	s.Equal(concurrency-1, duplicateCount+busyCount, "all other concurrent creates should return ErrDuplicateEvent or SQLITE_BUSY")
	s.Empty(otherErrors, "no unexpected errors should occur")
}

// isSQLiteBusyError checks if the error is a SQLite SQLITE_BUSY error
func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "SQLITE_BUSY") || strings.Contains(errStr, "database is locked")
}
