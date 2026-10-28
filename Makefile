.PHONY: verify openspec test web compose-check up down logs

CHANGE_ID := 2026-10-28-bootstrap-project-skeleton

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

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs --no-log-prefix api mcp worker nginx
