package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/joho/godotenv"
	app "github.com/jtravisp/privatepaste"
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
	log.Printf("config: table=%s region=%s profile=%s", cfg.TableName, cfg.Region, cfg.AWSProfile)

	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.Region),
		config.WithSharedConfigProfile(cfg.AWSProfile),
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

	staticFS, err := fs.Sub(app.FrontendFS, "frontend")
	if err != nil {
		log.Fatalf("failed to create static FS: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, err := app.FrontendFS.ReadFile("frontend/index.html")
		if err != nil {
			http.Error(w, "Failed to read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	mux.HandleFunc("GET /health", healthCheck)

	log.Println("starting server on :" + cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
