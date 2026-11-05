#!/usr/bin/env bash
# 为远程骨架式 .env.example（仅键名）填入本地演示默认值；不覆盖已有非空值。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ROOT}/.env"
test -f "$ENV_FILE" || cp "${ROOT}/.env.example" "$ENV_FILE"

set_default() {
  local key="$1"
  local value="$2"
  if grep -q "^${key}=$" "$ENV_FILE" 2>/dev/null || ! grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    if grep -q "^${key}=" "$ENV_FILE"; then
      sed -i.bak "s|^${key}=.*|${key}=${value}|" "$ENV_FILE" && rm -f "${ENV_FILE}.bak"
    else
      printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
    fi
  fi
}

set_default APP_ENV local
set_default APP_PORT 18088
set_default HTTP_PORT 8080
set_default MCP_PORT 8100
set_default SHUTDOWN_TIMEOUT 10s
set_default HEALTH_TIMEOUT 2s
set_default SESSION_SECRET dev-session-secret
set_default MCP_TOKEN dev-mcp-token
set_default MYSQL_ADDR mysql:3306
set_default MYSQL_ROOT_PASSWORD root
set_default MYSQL_DATABASE campusclaw
set_default MYSQL_USER campusclaw
set_default MYSQL_PASSWORD campusclaw
set_default MYSQL_DSN 'campusclaw:campusclaw@tcp(mysql:3306)/campusclaw?parseTime=true&charset=utf8mb4'
set_default REDIS_ADDR redis:6379
set_default REDIS_URL redis://redis:6379/0
set_default QDRANT_URL http://qdrant:6333
set_default QDRANT_COLLECTION campusclaw
set_default MINIO_URL http://minio:9000
set_default MINIO_ENDPOINT minio:9000
set_default MINIO_ROOT_USER minioadmin
set_default MINIO_ROOT_PASSWORD minioadmin
set_default MINIO_BUCKET campusclaw
set_default MCP_URL http://mcp:8100

echo "filled demo defaults into .env"
