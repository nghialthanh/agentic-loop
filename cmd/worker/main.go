package main

import (
	"log"
	"net/http"

	"github.com/agentic-loop/worker/internal/health"
)

func main() {
	mux := http.NewServeMux()
	health.Register(mux)
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
