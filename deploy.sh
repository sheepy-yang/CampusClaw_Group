#!/usr/bin/env bash
# CampusClaw 一键部署脚本
# 用法：
#   ./deploy.sh           # 构建并启动全部服务
#   ./deploy.sh status    # 查看状态
#   ./deploy.sh logs      # 跟随日志
#   ./deploy.sh down      # 停止
#   ./deploy.sh restart   # 重建并启动
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

COMPOSE=(docker compose)
if ! docker compose version >/dev/null 2>&1; then
  if command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
  else
    echo "错误：未找到 Docker Compose（需要 docker compose 或 docker-compose）。" >&2
    echo "请先安装 Docker Desktop / Docker Engine，并确认 compose 可用。" >&2
    echo "详见 REQUIREMENTS.md" >&2
    exit 1
  fi
fi

ensure_env() {
  if [[ ! -f .env ]]; then
    cp .env.example .env
    echo "已从 .env.example 生成 .env（可按需修改密钥）。"
  fi
  # 演示默认值，避免空 SESSION_SECRET / MCP_TOKEN 导致困惑
  if ! grep -q '^SESSION_SECRET=.\+' .env 2>/dev/null; then
    if grep -q '^SESSION_SECRET=$' .env 2>/dev/null; then
      # macOS / GNU sed 兼容写法
      sed -i.bak 's/^SESSION_SECRET=$/SESSION_SECRET=dev-session-secret/' .env && rm -f .env.bak
    fi
  fi
}

cmd="${1:-up}"

case "$cmd" in
  up|start|"")
    if ! command -v docker >/dev/null 2>&1; then
      echo "错误：未安装 docker。详见 REQUIREMENTS.md" >&2
      exit 1
    fi
    ensure_env
    echo "==> 构建并启动 CampusClaw …"
    "${COMPOSE[@]}" up -d --build
    echo
    echo "部署完成。"
    echo "  统一入口:  http://localhost"
    echo "  前端:      http://localhost:5173"
    echo "  API 健康:  http://localhost:8080/healthz"
    echo "  MCP 健康:  http://localhost:8100/healthz"
    echo
    echo "演示：打开首页选择「教师登录」或「学生登录」。"
    echo "更多要求见 REQUIREMENTS.md"
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
    echo "==> 冒烟检查"
    curl -sf http://127.0.0.1:8080/healthz && echo
    curl -sf -X POST http://127.0.0.1:8080/api/v1/auth/login \
      -H 'Content-Type: application/json' \
      -d '{"role":"teacher"}' && echo
    echo "smoke ok"
    ;;
  *)
    echo "用法: $0 [up|restart|down|status|logs|smoke]" >&2
    exit 1
    ;;
esac
