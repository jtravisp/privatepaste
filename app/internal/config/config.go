// Package config reads runtime settings from environment variables.
//
// Every setting has a default that works for local development, so the binary
// starts with no configuration at all. In Kubernetes the values come from the
// Deployment's env block (see deploy/k8s).
package config

import (
	"os"
	"time"
)

type Config struct {
	// Port the HTTP server listens on.
	Port string
	// Region for the AWS SDK.
	Region string
	// TableName is the DynamoDB table holding pastes.
	TableName string
	// Env is informational ("dev", "prod"); it is logged, never branched on.
	Env string
	// AWSProfile selects a shared-config profile for local runs against real
	// AWS. Empty in Kubernetes, where credentials come from EKS Pod Identity.
	AWSProfile string
	// DynamoEndpoint overrides the DynamoDB endpoint, e.g. DynamoDB Local in
	// kind. When set, the server also creates the table if it is missing --
	// a local-only convenience; the production role cannot create tables.
	DynamoEndpoint string
	// ShutdownDelay is how long the server keeps serving after SIGTERM while
	// reporting not-ready, so endpoints controllers and proxies stop sending
	// traffic before connections are closed.
	ShutdownDelay time.Duration
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Load() *Config {
	delay, err := time.ParseDuration(getEnv("SHUTDOWN_DELAY", "5s"))
	if err != nil {
		delay = 5 * time.Second
	}
	return &Config{
		Port:           getEnv("PORT", "8080"),
		Region:         getEnv("AWS_REGION", "us-east-1"),
		TableName:      getEnv("TABLE_NAME", "privatepaste-local"),
		Env:            getEnv("ENV", "dev"),
		AWSProfile:     getEnv("AWS_PROFILE", ""),
		DynamoEndpoint: getEnv("DYNAMODB_ENDPOINT", ""),
		ShutdownDelay:  delay,
	}
}
