package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jtravisp/privatepaste/internal/model"
)

type DynamoStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamo(client *dynamodb.Client, tableName string) *DynamoStore {
	return &DynamoStore{client: client, tableName: tableName}
}

func key(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: id}}
}

func (s *DynamoStore) CreatePaste(ctx context.Context, paste *model.Paste) error {
	av, err := attributevalue.MarshalMap(paste)
	if err != nil {
		return fmt.Errorf("marshal paste: %w", err)
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
		// IDs are random, so a collision is astronomically unlikely -- but
		// never silently overwrite someone else's paste if it happens.
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err != nil {
		return fmt.Errorf("put paste: %w", err)
	}
	return nil
}

func (s *DynamoStore) GetPaste(ctx context.Context, id string) (*model.Paste, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key(id),
		// Strongly consistent so a paste deleted a moment ago is not served
		// from a stale replica.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("get paste: %w", err)
	}
	if out.Item == nil {
		return nil, ErrNotFound
	}
	var p model.Paste
	if err := attributevalue.UnmarshalMap(out.Item, &p); err != nil {
		return nil, fmt.Errorf("unmarshal paste: %w", err)
	}
	return &p, nil
}

func (s *DynamoStore) ConsumePaste(ctx context.Context, id string) (*model.Paste, error) {
	out, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(s.tableName),
		Key:                 key(id),
		ConditionExpression: aws.String("attribute_exists(id)"),
		ReturnValues:        types.ReturnValueAllOld,
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("consume paste: %w", err)
	}
	var p model.Paste
	if err := attributevalue.UnmarshalMap(out.Attributes, &p); err != nil {
		return nil, fmt.Errorf("unmarshal paste: %w", err)
	}
	return &p, nil
}

func (s *DynamoStore) DeletePaste(ctx context.Context, id string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key:       key(id),
	})
	if err != nil {
		return fmt.Errorf("delete paste: %w", err)
	}
	return nil
}

func (s *DynamoStore) Ping(ctx context.Context) error {
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(s.tableName)})
	if err != nil {
		return fmt.Errorf("describe table: %w", err)
	}
	return nil
}

// EnsureTable creates the table if it does not exist. Only called when a
// local endpoint (DynamoDB Local) is configured; in AWS the table is owned by
// Terraform and the app's role has no CreateTable permission. Retries for a
// while because DynamoDB Local may still be starting.
func (s *DynamoStore) EnsureTable(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := s.createIfMissing(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *DynamoStore) createIfMissing(ctx context.Context) error {
	if err := s.Ping(ctx); err == nil {
		return nil
	}
	_, err := s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            aws.String(s.tableName),
		BillingMode:          types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash}},
	})
	var inUse *types.ResourceInUseException
	if err != nil && !errors.As(err, &inUse) {
		return fmt.Errorf("create table: %w", err)
	}
	return nil
}
