package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/joho/godotenv"
	appconfig "github.com/jtravisp/privatepaste/internal/config"
	"github.com/jtravisp/privatepaste/internal/store"
)


func healthCheck(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

func main() {
    godotenv.Load()
	cfg := appconfig.Load()
	
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
    config.WithRegion(cfg.Region),
	)
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	store := store.NewDynamo(dynamoClient, cfg.TableName)
	_ = store // temporary to avoid unused variable error	

	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	mux := http.NewServeMux()
    mux.HandleFunc("GET /health", healthCheck)

    log.Println("starting server on :" + cfg.Port)
    log.Fatal(http.ListenAndServe(":" + cfg.Port, mux))
}
