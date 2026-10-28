package main

import (
	"log"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpserver"
)

// API 入口：挂载平台服务、教学技能与智能体路由（骨架）。
func main() {
	mux := httpserver.NewMux()
	addr := ":8080"
	log.Printf("CampusClaw API skeleton listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
