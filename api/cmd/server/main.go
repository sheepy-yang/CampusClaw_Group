package main

import (
	"log"
	"net/http"
	"os"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpserver"
)

func main() {
	mux := httpserver.NewMux()
	handler := httpserver.CORSMiddleware(mux)
	addr := ":8080"
	if p := os.Getenv("HTTP_PORT"); p != "" {
		addr = ":" + p
	}
	log.Printf("CampusClaw API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}
