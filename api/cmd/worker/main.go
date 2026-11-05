package main

import (
	"log"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
)

func main() {
	w := queue.NewWorker()
	log.Println("CampusClaw worker starting (in-memory queue; share process with API for demo)")
	if err := w.Run(); err != nil {
		log.Fatal(err)
	}
}
