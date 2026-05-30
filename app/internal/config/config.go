package config

import (
	"os"
)

type Config struct {
	Port      	string
	Region    	string
	TableName 	string
	Env       	string
	AWSProfile	string
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Load() *Config {
	return &Config{
		Port:      	getEnv("PORT", "8081"),
		Region:    	getEnv("AWS_REGION", "us-east-1"),
		TableName: 	getEnv("TABLE_NAME", "private-paste"),
		Env:       	getEnv("ENV", "dev"),
		AWSProfile: getEnv("AWS_PROFILE", ""),
	}
}
