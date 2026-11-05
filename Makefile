.PHONY: check check-security verify openspec openapi test web e2e load-smoke compose-check up down logs smoke env help

CHANGE_IDS := 2026-10-28-bootstrap-project-skeleton yang-evaluation-core 2026-11-05-learning-state-review-path 2026-11-06-campusclaw-core-paths 2026-11-06-content-production-classroom

help:
	@echo "make verify  - OpenSpec + 测试 + 前端构建 + Compose 校验"
	@echo "make check-security - 密钥、依赖与安全回归检查"
	@echo "make up      - docker compose up -d --build"
	@echo "make down    - docker compose down"
	@echo "make logs    - 跟随 api/mcp/worker/nginx 日志"
	@echo "make smoke   - 检查健康端点与教师登录"
	@echo "make e2e     - 对已启动的 Compose 环境运行浏览器端到端测试"
	@echo "make load-smoke - 对已启动环境执行有界并发冒烟压测"
	@echo "make env     - 从 .env.example 生成本地 .env（含演示默认值）"

check: verify

check-security:
	bash ./scripts/check-secrets.sh
	cd api && go test ./internal/auth ./internal/security ./internal/platform ./internal/httpserver
	cd api && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	cd mcp-teaching-tools && go test ./...
	cd mcp-teaching-tools && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	npm --prefix web audit --audit-level=high

verify: openspec openapi test web compose-check

openspec:
	@for change in $(CHANGE_IDS); do \
		npx --yes @fission-ai/openspec@1.15.0 validate $$change --strict || exit 1; \
	done

openapi:
	npx --yes @apidevtools/swagger-cli@4.0.4 validate docs/openapi.yaml

test:
	cd api && go test ./...
	cd api && go vet ./...
	cd mcp-teaching-tools && go test ./...
	cd mcp-teaching-tools && go vet ./...

web:
	npm --prefix web ci
	npm --prefix web run build
	npm --prefix web audit --audit-level=high

e2e:
	npm --prefix web run e2e

load-smoke:
	python3 scripts/load-smoke.py

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

smoke:
	./deploy.sh smoke
