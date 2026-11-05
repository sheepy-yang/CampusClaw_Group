.PHONY: verify openspec test web compose-check up down logs env help

CHANGE_ID := 2026-10-28-bootstrap-project-skeleton

help:
	@echo "make verify  - OpenSpec + 测试 + 前端构建 + Compose 校验"
	@echo "make up      - docker compose up -d --build"
	@echo "make down    - docker compose down"
	@echo "make logs    - 跟随 api/mcp/worker/nginx 日志"
	@echo "make env     - 从 .env.example 生成本地 .env（含演示默认值）"

verify: openspec test web compose-check

openspec:
	npx --yes @fission-ai/openspec@1.15.0 validate $(CHANGE_ID) --strict

test:
	cd api && go test ./...
	cd api && go vet ./...
	cd mcp-teaching-tools && go test ./...
	cd mcp-teaching-tools && go vet ./...

web:
	npm --prefix web ci
	npm --prefix web run build
	npm --prefix web audit --audit-level=high

compose-check:
	docker compose config --quiet

env:
	@test -f .env || cp .env.example .env
	@bash ./scripts/fill-env-defaults.sh

up: env
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs --no-log-prefix api mcp worker nginx
