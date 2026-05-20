package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/jtravisp/privatepaste/internal/model"
)

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
	return nil, nil
}

func (s *DynamoStore) DeletePaste(id string, ownerToken string) error {
	return nil
}
