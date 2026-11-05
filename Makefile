# CampusClaw 常用命令
.PHONY: up down restart status logs smoke env help

help:
	@echo "make up       - 一键部署（./deploy.sh）"
	@echo "make down     - 停止全部容器"
	@echo "make restart  - 强制重建并启动"
	@echo "make status   - 查看容器状态"
	@echo "make logs     - 跟随全部日志"
	@echo "make smoke    - 部署后冒烟检查"
	@echo "make env      - 从示例生成 .env"

env:
	@test -f .env || cp .env.example .env
	@echo ".env ready"

up: env
	./deploy.sh up

down:
	./deploy.sh down

restart:
	./deploy.sh restart

status:
	./deploy.sh status

logs:
	./deploy.sh logs

smoke:
	./deploy.sh smoke
