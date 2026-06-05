package dynamodb

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

type eventItem struct {
	SessionID string `dynamodbav:"session_id"`
	SortKey   string `dynamodbav:"sort_key"` // "e#<uuid>" for uuid-based, "t#<created_at>#<id>" for timestamp-based
	ID        string `dynamodbav:"id"`
	EventType string `dynamodbav:"event_type"`
	Payload   string `dynamodbav:"payload"` // JSON string
	UUID      string `dynamodbav:"uuid"`
	CreatedAt string `dynamodbav:"created_at"`
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

	createdAtStr := event.CreatedAt.Format(time.RFC3339Nano)

	// Use uuid-based sort_key for uniqueness, fall back to timestamp-based for events without uuid
	var sortKey string
	if event.UUID != "" {
		sortKey = "e#" + event.UUID // "e#" prefix for uuid-based events
	} else {
		sortKey = "t#" + createdAtStr + "#" + event.ID // "t#" prefix for timestamp-based events
	}

	item := eventItem{
		SessionID: event.SessionID,
		SortKey:   sortKey,
		ID:        event.ID,
		EventType: event.EventType,
		Payload:   string(payloadJSON),
		UUID:      event.UUID,
		CreatedAt: createdAtStr,
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}

	// Use conditional PutItem to atomically check for duplicates
	// attribute_not_exists(session_id) ensures this is a new item (session_id + sort_key combination)
	_, err = r.db.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.db.TableName("events")),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(session_id)"),
	})
	if err != nil {
		// Check if it's a conditional check failure (duplicate)
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return repository.ErrDuplicateEvent
		}
		return err
	}
	return nil
}

func (r *EventRepository) FindBySessionID(ctx context.Context, sessionID string) ([]*domain.Event, error) {
	keyCond := expression.Key("session_id").Equal(expression.Value(sessionID))

	// Build filter condition from domain.HiddenEventTypes
	var filterCond expression.ConditionBuilder
	for i, eventType := range domain.HiddenEventTypes {
		cond := expression.Name("event_type").NotEqual(expression.Value(eventType))
		if i == 0 {
			filterCond = cond
		} else {
			filterCond = expression.And(filterCond, cond)
		}
	}

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCond).
		WithFilter(filterCond).
		Build()
	if err != nil {
		return nil, err
	}

	result, err := r.db.Client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.db.TableName("events")),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(true), // Ascending order (chronological)
	})
	if err != nil {
		return nil, err
	}

	var items []eventItem
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &items); err != nil {
		return nil, err
	}

	events := make([]*domain.Event, len(items))
	for i, item := range items {
		events[i] = r.itemToEvent(&item)
	}

	// sort_key is uuid-based, so chronological order must be reconstructed here.
	// Match the other backends: sort by payload.timestamp, not created_at.
	sort.SliceStable(events, func(i, j int) bool {
		return eventTimestamp(events[i]).Before(eventTimestamp(events[j]))
	})

	return events, nil
}

func eventTimestamp(e *domain.Event) time.Time {
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

func (r *EventRepository) CountBySessionID(ctx context.Context, sessionID string) (int, error) {
	keyCond := expression.Key("session_id").Equal(expression.Value(sessionID))

	// Build filter condition from domain.HiddenEventTypes
	var filterCond expression.ConditionBuilder
	for i, eventType := range domain.HiddenEventTypes {
		cond := expression.Name("event_type").NotEqual(expression.Value(eventType))
		if i == 0 {
			filterCond = cond
		} else {
			filterCond = expression.And(filterCond, cond)
		}
	}

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCond).
		WithFilter(filterCond).
		Build()
	if err != nil {
		return 0, err
	}

	result, err := r.db.Client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.db.TableName("events")),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Select:                    types.SelectCount,
	})
	if err != nil {
		return 0, err
	}

	return int(result.Count), nil
}

func (r *EventRepository) itemToEvent(item *eventItem) *domain.Event {
	createdAt, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)

	var payload map[string]interface{}
	json.Unmarshal([]byte(item.Payload), &payload)

	return &domain.Event{
		ID:        item.ID,
		SessionID: item.SessionID,
		EventType: item.EventType,
		Payload:   payload,
		UUID:      item.UUID,
		CreatedAt: createdAt,
	}
}

func (r *EventRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	// Query all events for this session to get their sort keys
	keyCond := expression.Key("session_id").Equal(expression.Value(sessionID))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return err
	}

	result, err := r.db.Client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.db.TableName("events")),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ProjectionExpression:      aws.String("session_id, sort_key"),
	})
	if err != nil {
		return err
	}

	// Delete each event
	for _, item := range result.Items {
		_, err := r.db.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(r.db.TableName("events")),
			Key: map[string]types.AttributeValue{
				"session_id": item["session_id"],
				"sort_key":   item["sort_key"],
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}
