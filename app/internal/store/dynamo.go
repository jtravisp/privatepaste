package store

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/jtravisp/privatepaste/internal/model"
)

type DynamoStore struct {
    client     *dynamodb.Client
    tableName   string
}

func NewDynamo(client *dynamodb.Client, tableName string) *DynamoStore {
	return &DynamoStore{
		client:   client,
		tableName: tableName,
	}	
}

func (s *DynamoStore) CreatePaste(paste *model.Paste) error {
	return nil
}

func (s *DynamoStore) GetPaste(id string) (*model.Paste, error) {
	return nil, nil
}

func (s *DynamoStore) DeletePaste(id string, ownerToken string) error {
	return nil
}
