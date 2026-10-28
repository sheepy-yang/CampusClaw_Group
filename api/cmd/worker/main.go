package main

import (
	"log"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
)

// Worker 入口：消费 Redis 队列中的异步教学任务（骨架）。
func main() {
	w := queue.NewWorker()
	log.Println("CampusClaw worker skeleton starting")
	if err := w.Run(); err != nil {
		log.Fatal(err)
	}
}
