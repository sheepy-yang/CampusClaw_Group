# CampusClaw 环境与依赖要求（Requirements）

本文档说明一键部署与本地开发所需的**系统环境、软件依赖、端口与配置**。

## 当前开发阶段

项目已进入完整联调阶段：核心平台路径、十二项教学技能、MCP、持久异步任务和前端入口均已实现。所有业务变更仍须先通过对应 OpenSpec strict validation；认证、班级隔离、不可信模型/MCP 输出校验和密钥保护是持续约束。

分支保护、交叉评审、干净环境复验、真实用户反馈和部署演示由小组成员按课程最终验收要求实际完成，不得由程序生成或伪造。

---

## 1. 一键部署最低要求

| 项目 | 要求 |
| --- | --- |
| 操作系统 | macOS 12+ / Ubuntu 22.04+ / Windows 10+（建议 WSL2） |
| CPU / 内存 | 建议 2 核及以上，内存 ≥ 4 GB（推荐 8 GB） |
| 磁盘 | ≥ 5 GB 可用空间（镜像与卷） |
| 必装软件 | [Docker Desktop](https://www.docker.com/products/docker-desktop/) 或 Docker Engine **20.10+** |
| Compose | Docker Compose **v2**（`docker compose` 子命令） |
| 网络 | 首次构建需能拉取 Docker Hub 镜像（`golang`、`node`、`mysql`、`redis`、`nginx` 等） |

> **一键部署不要求本机安装 Go / Node.js**。业务进程全部跑在容器内。

### 验证环境

```bash
docker --version
docker compose version
```

两条命令均有输出即可进入一键部署。

---

## 2. 一键部署指令

在仓库根目录执行：

```bash
./deploy.sh
```

等价于：

```bash
make env                  # 从 .env.example 填充演示默认值
docker compose up -d --build
# 或：./deploy.sh
```

部署成功后访问（默认 `APP_PORT=18088`，仅 Nginx 对宿主机暴露）：

| 入口 | 地址 |
| --- | --- |
| 统一入口（Nginx） | http://127.0.0.1:18088 |
| 健康检查 | http://127.0.0.1:18088/health |
| 本地前端开发 | http://127.0.0.1:5173（Vite，`/api` 与 `/health` 代理到 API） |

演示登录（详见 README「测试账号」）：

| 账号 | 密码 | 角色 |
| --- | --- | --- |
| `teacher` | `demo123` | 教师 |
| `student` | `demo123` | 学生 |
| `student2` | `demo123` | 学生 |
| `student3` | `demo123` | 学生 |
| `student4` | `demo123` | 学生 |
| `student5` | `demo123` | 学生 |
| `teacher_other` | `demo123` | 其他班教师 |
| `student_other` | `demo123` | 其他班学生 |

常用运维：

```bash
./deploy.sh status    # 查看容器状态
./deploy.sh logs      # 跟随日志
./deploy.sh down      # 停止并移除容器
./deploy.sh restart   # 重新构建并启动
```

或使用 Make：

```bash
make verify   # OpenSpec + 测试 + 前端构建 + Compose 校验
make up       # 一键部署
make down     # 停止
make logs     # 日志
```

---

## 3. 运行时服务与端口

对齐远程骨架：`docker-compose.yml` **只把 Nginx 映射到宿主机**；API / MCP / 数据依赖仅在 Compose 网络内可达。

| 服务 | 说明 | 宿主机 |
| --- | --- | --- |
| nginx | 反向代理（`/health`、`/api/`、静态前端） | `${APP_PORT}:80`（默认 18088） |
| web | React 静态资源 | 不暴露 |
| api | Go API（鉴权、平台主路径与十二项技能） | 不暴露 |
| worker | Redis Streams 独立消费者（异步批改、演练、讲评等） | 不暴露 |
| mcp | MCP JSON-RPC 鉴权、发现与工具执行 | 不暴露 |
| mysql / redis / qdrant / minio | 依赖服务 | 不暴露 |

本地调试可直接 `go run` API（`HTTP_PORT=8080`）并用 Vite 代理。

---

## 4. 环境变量要求（`.env`）

从 `.env.example` 复制生成 `.env`。一键脚本会自动完成复制。

| 变量 | 是否必填 | 说明 |
| --- | --- | --- |
| `APP_ENV` / `APP_PORT` / `HTTP_PORT` / `MCP_PORT` | 必填 | Compose 使用 `${NAME:?}`；`make env` 会写入演示值 |
| `SHUTDOWN_TIMEOUT` / `HEALTH_TIMEOUT` | 必填 | 优雅退出与探活超时 |
| `SESSION_SECRET` | 必填 | ≥16 字符；演示默认 `dev-session-secret` |
| `AUTH_USERS_JSON` | 非演示环境必填 | 用户数组；密码只能使用 bcrypt 哈希，禁止明文 |
| `MCP_TOKEN` | 必填 | MCP Bearer；演示默认 `dev-mcp-token` |
| `MYSQL_ADDR` / `REDIS_ADDR` / `QDRANT_URL` / `MINIO_URL` / `MINIO_ENDPOINT` / `MCP_URL` | 必填 | API、Worker 与依赖地址 |
| `MYSQL_*` / `MINIO_ROOT_*` / `MINIO_BUCKET` | 必填 | 数据服务启动凭证及附件桶 |
| `LLM_*` / `EMBEDDING_*` | 演示环境可选，其他环境必填 | 课程网关配置；对话模型使用 `course-chat`，向量模型使用 `course-embedding`。同一课程网关密钥可分别填入两个 Key 变量 |

**安全约束（与 README 一致）：**

- 模型 API Key、MCP Token、数据库口令、对象存储密钥**只放服务端环境变量**，禁止写入前端或 Git。
- `.env` 已在 `.gitignore` 中，勿提交真实密钥。

---

## 5. 软件依赖清单

### 5.1 后端 API（`api/`）

| 依赖 | 版本 | 备注 |
| --- | --- | --- |
| Go | 1.27.3（镜像 `golang:1.27.3-alpine`） | 包含当前标准库安全修复；MySQL、Redis 与 MinIO 客户端见 `api/go.mod` |
| 模块路径 | `github.com/sheepy-yang/CampusClaw_Group/api` | 见 `api/go.mod` |

本地开发（非 Docker）另需：Go 1.27.3+。

### 5.2 MCP 服务（`mcp-teaching-tools/`）

| 依赖 | 版本 | 备注 |
| --- | --- | --- |
| Go | 1.27.3+ | 标准库 HTTP JSON-RPC 风格接口 |

工具：`formula_verify`、`assignment_stats`、`student_mistakes`。

### 5.3 前端（`web/`）

| 依赖 | 版本 | 来源 |
| --- | --- | --- |
| Node.js | 20+（镜像 `node:20-alpine`） | 构建与开发服务器 |
| react / react-dom | ^18.3.1 | `web/package.json` |
| react-router-dom | 7.18.5 | 路由 |
| vite | 8.3.6 | 开发与构建 |
| typescript | ^5.6.3 | 类型检查 |
| @vitejs/plugin-react | 6.2.1 | React 插件 |

安装：

```bash
cd web && npm install
```

依赖由已提交的 `package-lock.json` 锁定；CI 与本地验收使用 `npm ci`。

### 5.4 基础设施镜像

| 镜像 | 用途 |
| --- | --- |
| `mysql:8.0` | 关系库 |
| `redis:7` | 队列 / 缓存 |
| `qdrant/qdrant:v1.12.1` | 向量库 |
| `ghcr.io/coollabsio/minio:RELEASE.2025-10-15T17-29-55Z` | S3 兼容对象存储（固定上游 release，来源说明见 `docs/dependencies.md`） |
| `nginx:1.27` | 反向代理 |
| `golang:1.27.3-alpine` | 构建 API / MCP |
| `node:20-alpine` | 前端容器 |
| `alpine:3.20` | API / MCP 运行时 |

---

## 6. 本地开发模式要求（可选）

不使用 Docker 跑业务容器时，需额外安装：

| 软件 | 版本 |
| --- | --- |
| Go | ≥ 1.27.3 |
| Node.js | ≥ 20，含 npm |
| （可选）Docker | 仅拉起 mysql / redis / qdrant / minio |

```bash
# 依赖服务
docker compose up -d mysql redis qdrant minio

# 三个进程
cd mcp-teaching-tools && go run ./cmd/server   # :8100
cd api && go run ./cmd/server                 # :8080
cd web && npm install && npm run dev          # :5173
```

---

## 7. 验收冒烟（部署后）

```bash
curl -s http://127.0.0.1:18088/health
# 期望：HTTP 200，status=up，checks 含 mysql/redis/qdrant/minio/mcp

curl -s -X POST http://127.0.0.1:18088/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher","password":"demo123"}'
# 期望：返回随机、限时 token

TOKEN="$(curl -s -X POST http://127.0.0.1:18088/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher","password":"demo123"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
curl -s -X POST http://127.0.0.1:18088/api/v1/skills/proposition \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"高斯消元","materialId":"mat-1","count":1,"exerciseTypes":["short_answer"],"idempotencyKey":"smoke-proposition-001"}'
# 期望：HTTP 201（首次）或 200（同幂等键重放），返回 contentId、质量报告和用量信息
```

浏览器打开 http://127.0.0.1:18088，可分别从教师工作台和学生学习台验收十二项技能、RAG 助手、作业异步批改、附件上传和审计链路。

---

## 8. 常见问题

| 现象 | 处理 |
| --- | --- |
| `docker compose` 不可用 | 升级 Docker Desktop，或安装 Compose v2 插件 |
| `APP_PORT` 被占用 | 修改 `.env` 中 `APP_PORT` 后 `./deploy.sh restart` |
| 构建拉镜像失败 | 配置镜像加速 / 代理后重试 `./deploy.sh restart` |
| API 报 MCP 失败 | 确认 `mcp` 容器 healthy，且 `.env` 中 `MCP_TOKEN` 与 mcp 服务一致 |
| 未配置 LLM Key | 仅 `local/development/test/demo` 使用明确标记的本地演示适配器；其他环境启动失败。真实图片转写始终要求课程网关，失败时保留提交 |
