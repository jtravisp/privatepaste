package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/joho/godotenv"
	appconfig "github.com/jtravisp/privatepaste/internal/config"
	"github.com/jtravisp/privatepaste/internal/handler"
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

	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	mux := http.NewServeMux()
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	store := store.NewDynamo(dynamoClient, cfg.TableName)
	pasteHandler := handler.NewPasteHandler(store)
	mux.HandleFunc("POST /pastes", pasteHandler.CreatePaste)
	mux.HandleFunc("DELETE /pastes/{id}", pasteHandler.DeletePaste)
	mux.HandleFunc("GET /pastes/{id}", pasteHandler.GetPaste)
	mux.HandleFunc("GET /health", healthCheck)

	log.Println("starting server on :" + cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
