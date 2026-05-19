package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/jtravisp/privatepaste/internal/config"
)


func healthCheck(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

func main() {
    godotenv.Load()
	cfg := config.Load()
	
	mux := http.NewServeMux()
    mux.HandleFunc("GET /health", healthCheck)

    log.Println("starting server on :" + cfg.Port)
    log.Fatal(http.ListenAndServe(":" + cfg.Port, mux))

}
