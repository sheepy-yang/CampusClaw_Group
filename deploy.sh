#!/usr/bin/env bash
# CampusClaw 一键部署（对齐远程 Compose：仅 Nginx 对宿主暴露端口）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

COMPOSE=(docker compose)
if ! docker compose version >/dev/null 2>&1; then
  if command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
  else
    echo "错误：未找到 Docker Compose。" >&2
    exit 1
  fi
fi

ensure_env() {
  if [[ ! -f .env ]]; then
    cp .env.example .env
    echo "已从 .env.example 生成 .env"
  fi
  bash ./scripts/fill-env-defaults.sh
}

cmd="${1:-up}"
APP_PORT="$(grep -E '^APP_PORT=' .env 2>/dev/null | cut -d= -f2- || true)"
APP_PORT="${APP_PORT:-18088}"

case "$cmd" in
  up|start|"")
    ensure_env
    APP_PORT="$(grep -E '^APP_PORT=' .env | cut -d= -f2-)"
    echo "==> 构建并启动 CampusClaw …"
    "${COMPOSE[@]}" up -d --build
    echo
    echo "部署完成。"
    echo "  统一入口:  http://127.0.0.1:${APP_PORT}"
    echo "  健康检查:  curl -i http://127.0.0.1:${APP_PORT}/health"
    echo "演示：打开首页选择「教师登录」或「学生登录」。"
    ;;
  restart)
    ensure_env
    "${COMPOSE[@]}" up -d --build --force-recreate
    ;;
  down|stop)
    "${COMPOSE[@]}" down
    ;;
  status|ps)
    "${COMPOSE[@]}" ps
    ;;
  logs)
    shift || true
    "${COMPOSE[@]}" logs -f "$@"
    ;;
  smoke)
    ensure_env
    APP_PORT="$(grep -E '^APP_PORT=' .env | cut -d= -f2-)"
    echo "==> 冒烟检查"
    curl -sf "http://127.0.0.1:${APP_PORT}/health" && echo
    curl -sf -X POST "http://127.0.0.1:${APP_PORT}/api/v1/auth/login" \
      -H 'Content-Type: application/json' \
      -d '{"role":"teacher"}' && echo
    echo "smoke ok"
    ;;
  *)
    echo "用法: $0 [up|restart|down|status|logs|smoke]" >&2
    exit 1
    ;;
esac
