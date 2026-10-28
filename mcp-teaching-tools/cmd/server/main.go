package main

import (
	"log"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/mcp-teaching-tools/internal/mcp"
)

func main() {
	mux := mcp.NewHandler()
	log.Println("mcp-teaching-tools skeleton on :8100")
	log.Fatal(http.ListenAndServe(":8100", mux))
}
