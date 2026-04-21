package dynamodb

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
)

type PlanCommentThreadRepository struct {
	db *DB
}

func NewPlanCommentThreadRepository(db *DB) *PlanCommentThreadRepository {
	return &PlanCommentThreadRepository{db: db}
}

type planCommentThreadItem struct {
	ID               string `dynamodbav:"id"`
	PlanDocumentID   string `dynamodbav:"plan_document_id"`
	TargetText       string `dynamodbav:"target_text"`
	ContextBefore    string `dynamodbav:"context_before"`
	ContextAfter     string `dynamodbav:"context_after"`
	OriginalBodyHash string `dynamodbav:"original_body_hash"`
	Status           string `dynamodbav:"status"`
	CreatedAt        string `dynamodbav:"created_at"`
	UpdatedAt        string `dynamodbav:"updated_at"`
}

func (r *PlanCommentThreadRepository) Create(ctx context.Context, thread *domain.PlanCommentThread) error {
	if thread.ID == "" {
		thread.ID = uuid.New().String()
	}
	now := time.Now()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	if thread.UpdatedAt.IsZero() {
		thread.UpdatedAt = now
	}
	if thread.Status == "" {
		thread.Status = domain.PlanCommentThreadStatusActive
	}

	item := planCommentThreadItem{
		ID:               thread.ID,
		PlanDocumentID:   thread.PlanDocumentID,
		TargetText:       thread.TargetText,
		ContextBefore:    thread.ContextBefore,
		ContextAfter:     thread.ContextAfter,
		OriginalBodyHash: thread.OriginalBodyHash,
		Status:           string(thread.Status),
		CreatedAt:        thread.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:        thread.UpdatedAt.Format(time.RFC3339Nano),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}

	_, err = r.db.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.db.TableName("plan_comment_threads")),
		Item:      av,
	})
	return err
}

func (r *PlanCommentThreadRepository) FindByID(ctx context.Context, id string) (*domain.PlanCommentThread, error) {
	result, err := r.db.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName("plan_comment_threads")),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, nil
	}

	var item planCommentThreadItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, err
	}

	return r.itemToThread(&item), nil
}

func (r *PlanCommentThreadRepository) FindByPlanDocumentID(ctx context.Context, planDocumentID string, status *domain.PlanCommentThreadStatus) ([]*domain.PlanCommentThread, error) {
	keyCond := expression.Key("plan_document_id").Equal(expression.Value(planDocumentID))
	builder := expression.NewBuilder().WithKeyCondition(keyCond)

	if status != nil {
		filter := expression.Name("status").Equal(expression.Value(string(*status)))
		builder = builder.WithFilter(filter)
	}

	expr, err := builder.Build()
	if err != nil {
		return nil, err
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(r.db.TableName("plan_comment_threads")),
		IndexName:                 aws.String("plan_document_id-created_at-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(true), // Ascending order by created_at
	}
	if expr.Filter() != nil {
		input.FilterExpression = expr.Filter()
	}

	result, err := r.db.Client.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	var items []planCommentThreadItem
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &items); err != nil {
		return nil, err
	}

	threads := make([]*domain.PlanCommentThread, 0, len(items))
	for _, item := range items {
		threads = append(threads, r.itemToThread(&item))
	}

	return threads, nil
}

func (r *PlanCommentThreadRepository) CountActiveByPlanDocumentID(ctx context.Context, planDocumentID string) (int, error) {
	keyCond := expression.Key("plan_document_id").Equal(expression.Value(planDocumentID))
	filter := expression.Name("status").Equal(expression.Value(string(domain.PlanCommentThreadStatusActive)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).WithFilter(filter).Build()
	if err != nil {
		return 0, err
	}

	result, err := r.db.Client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.db.TableName("plan_comment_threads")),
		IndexName:                 aws.String("plan_document_id-created_at-index"),
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

func (r *PlanCommentThreadRepository) Update(ctx context.Context, thread *domain.PlanCommentThread) error {
	thread.UpdatedAt = time.Now()

	update := expression.Set(expression.Name("status"), expression.Value(string(thread.Status))).
		Set(expression.Name("updated_at"), expression.Value(thread.UpdatedAt.Format(time.RFC3339Nano)))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return err
	}

	_, err = r.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.db.TableName("plan_comment_threads")),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: thread.ID},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	return err
}

func (r *PlanCommentThreadRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.db.TableName("plan_comment_threads")),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	return err
}

func (r *PlanCommentThreadRepository) MarkOutdatedByPlanDocumentID(ctx context.Context, planDocumentID string) error {
	// First, find all active threads for this plan document
	activeStatus := domain.PlanCommentThreadStatusActive
	threads, err := r.FindByPlanDocumentID(ctx, planDocumentID, &activeStatus)
	if err != nil {
		return err
	}

	// Update each thread to outdated
	now := time.Now()
	for _, thread := range threads {
		thread.Status = domain.PlanCommentThreadStatusOutdated
		thread.UpdatedAt = now
		if err := r.Update(ctx, thread); err != nil {
			return err
		}
	}

	return nil
}

func (r *PlanCommentThreadRepository) itemToThread(item *planCommentThreadItem) *domain.PlanCommentThread {
	createdAt, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, item.UpdatedAt)

	return &domain.PlanCommentThread{
		ID:               item.ID,
		PlanDocumentID:   item.PlanDocumentID,
		TargetText:       item.TargetText,
		ContextBefore:    item.ContextBefore,
		ContextAfter:     item.ContextAfter,
		OriginalBodyHash: item.OriginalBodyHash,
		Status:           domain.PlanCommentThreadStatus(item.Status),
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
}
