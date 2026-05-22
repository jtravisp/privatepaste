package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jtravisp/privatepaste/internal/model"
)

var ErrNotFound = errors.New("paste not found")

type DynamoStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamo(client *dynamodb.Client, tableName string) *DynamoStore {
	return &DynamoStore{
		client:    client,
		tableName: tableName,
	}
}

func (s *DynamoStore) CreatePaste(paste *model.Paste) error {
	av, err := attributevalue.MarshalMap(paste)
	if err != nil {
		return fmt.Errorf("failed to marshal Record, %w", err)
	}

	_, err = s.client.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to put Record, %w", err)
	}

	return nil
}

func (s *DynamoStore) GetPaste(id string) (*model.Paste, error) {
	result, err := s.client.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"ID": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item, %w", err)
	}

	if result.Item == nil {
		return nil, ErrNotFound
	}

	var paste model.Paste
	err = attributevalue.UnmarshalMap(result.Item, &paste)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal record, %w", err)
	}

	return &paste, nil
}

func (s *DynamoStore) DeletePaste(id string) error {
	_, err := s.client.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"ID": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete item, %w", err)
	}

	return nil
}
