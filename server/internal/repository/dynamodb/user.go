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

type UserRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

type userItem struct {
	ID          string `dynamodbav:"id"`
	Email       string `dynamodbav:"email"`
	DisplayName string `dynamodbav:"display_name,omitempty"`
	CreatedAt   string `dynamodbav:"created_at"`
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}

	item := userItem{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		CreatedAt:   user.CreatedAt.Format(time.RFC3339Nano),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}

	_, err = r.db.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.db.TableName("users")),
		Item:      av,
	})
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	result, err := r.db.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName("users")),
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

	var item userItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, err
	}

	return r.itemToUser(&item), nil
}

func (r *UserRepository) FindByIDs(ctx context.Context, ids []string) (map[string]*domain.User, error) {
	result := make(map[string]*domain.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	tableName := r.db.TableName("users")

	const batchLimit = 100
	for start := 0; start < len(unique); start += batchLimit {
		end := start + batchLimit
		if end > len(unique) {
			end = len(unique)
		}
		keys := make([]map[string]types.AttributeValue, 0, end-start)
		for _, id := range unique[start:end] {
			keys = append(keys, map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: id},
			})
		}

		req := map[string]types.KeysAndAttributes{
			tableName: {Keys: keys},
		}

		for len(req) > 0 {
			output, err := r.db.Client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: req,
			})
			if err != nil {
				return nil, err
			}

			for _, raw := range output.Responses[tableName] {
				var item userItem
				if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
					return nil, err
				}
				u := r.itemToUser(&item)
				result[u.ID] = u
			}

			if unprocessed, ok := output.UnprocessedKeys[tableName]; ok && len(unprocessed.Keys) > 0 {
				req = map[string]types.KeysAndAttributes{tableName: unprocessed}
			} else {
				req = nil
			}
		}
	}
	return result, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	keyCond := expression.Key("email").Equal(expression.Value(email))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, err
	}

	result, err := r.db.Client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.db.TableName("users")),
		IndexName:                 aws.String("email-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, nil
	}

	var item userItem
	if err := attributevalue.UnmarshalMap(result.Items[0], &item); err != nil {
		return nil, err
	}

	return r.itemToUser(&item), nil
}

func (r *UserRepository) FindAll(ctx context.Context) ([]*domain.User, error) {
	result, err := r.db.Client.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(r.db.TableName("users")),
	})
	if err != nil {
		return nil, err
	}

	var items []userItem
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &items); err != nil {
		return nil, err
	}

	users := make([]*domain.User, len(items))
	for i, item := range items {
		users[i] = r.itemToUser(&item)
	}

	return users, nil
}

func (r *UserRepository) UpdateDisplayName(ctx context.Context, id string, displayName string) error {
	update := expression.Set(expression.Name("display_name"), expression.Value(displayName))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return err
	}

	_, err = r.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.db.TableName("users")),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	return err
}

func (r *UserRepository) itemToUser(item *userItem) *domain.User {
	createdAt, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
	return &domain.User{
		ID:          item.ID,
		Email:       item.Email,
		DisplayName: item.DisplayName,
		CreatedAt:   createdAt,
	}
}
