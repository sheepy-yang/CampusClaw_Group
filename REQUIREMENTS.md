# CampusClaw 环境与依赖要求（Requirements）

本文档说明一键部署与本地开发所需的**系统环境、软件依赖、端口与配置**。

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
| api | Go API（评价异步任务由进程内嵌 Worker 消费） | 不暴露 |
| worker | 生命周期骨架进程（不与 API 共享内存队列） | 不暴露 |
| mcp | MCP JSON-RPC 鉴权与工具声明 | 不暴露 |
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
| `MCP_TOKEN` | 必填 | MCP Bearer；演示默认 `dev-mcp-token` |
| `MYSQL_ADDR` / `REDIS_ADDR` / `QDRANT_URL` / `MINIO_URL` / `MCP_URL` | 必填 | API 健康检查与依赖地址 |
| `MYSQL_*` / `MINIO_ROOT_*` | 必填 | 数据服务启动凭证 |
| `LLM_*` / `EMBEDDING_*` | 可选 | 为空则离线演示合成 |

**安全约束（与 README 一致）：**

- 模型 API Key、MCP Token、数据库口令、对象存储密钥**只放服务端环境变量**，禁止写入前端或 Git。
- `.env` 已在 `.gitignore` 中，勿提交真实密钥。

---

## 5. 软件依赖清单

### 5.1 后端 API（`api/`）

| 依赖 | 版本 | 备注 |
| --- | --- | --- |
| Go | 1.22+（镜像 `golang:1.22-alpine`） | 标准库为主，无第三方强制依赖 |
| 模块路径 | `github.com/sheepy-yang/CampusClaw_Group/api` | 见 `api/go.mod` |

本地开发（非 Docker）另需：Go 1.22+。

### 5.2 MCP 服务（`mcp-teaching-tools/`）

| 依赖 | 版本 | 备注 |
| --- | --- | --- |
| Go | 1.22+ | 标准库 HTTP JSON-RPC 风格接口 |

工具：`formula_verify`、`assignment_stats`、`student_mistakes`。

### 5.3 前端（`web/`）

| 依赖 | 版本 | 来源 |
| --- | --- | --- |
| Node.js | 20+（镜像 `node:20-alpine`） | 构建与开发服务器 |
| react / react-dom | ^18.3.1 | `web/package.json` |
| react-router-dom | ^6.28.0 | 路由 |
| vite | ^5.4.10 | 开发与构建 |
| typescript | ^5.6.3 | 类型检查 |
| @vitejs/plugin-react | ^4.3.3 | React 插件 |

安装：

```bash
cd web && npm install
```

完整锁文件可在首次 `npm install` 后生成并提交 `package-lock.json`（可选）。

### 5.4 基础设施镜像

| 镜像 | 用途 |
| --- | --- |
| `mysql:8.0` | 关系库 |
| `redis:7` | 队列 / 缓存 |
| `qdrant/qdrant:v1.12.1` | 向量库 |
| `minio/minio:latest` | 对象存储 |
| `nginx:1.27` | 反向代理 |
| `golang:1.22-alpine` | 构建 API / MCP |
| `node:20-alpine` | 前端容器 |
| `alpine:3.20` | API / MCP 运行时 |

---

## 6. 本地开发模式要求（可选）

不使用 Docker 跑业务容器时，需额外安装：

| 软件 | 版本 |
| --- | --- |
| Go | ≥ 1.22（推荐 1.24，兼容较新 macOS） |
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
  -d '{"role":"teacher"}'
# 期望：返回 token

curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18088/api/v1/skills/proposition
# 期望：501（刘恺骐内容生产模块仍为骨架占位；刘沛鑫辅导模块同理）
```

浏览器打开 http://127.0.0.1:18088 ，教师登录后可验收杨瑾文四项评价技能；
刘恺骐（命题/备课/预习/虚拟课堂）与刘沛鑫（复习路径/追问辅导/课堂聚类/自适应闯关）入口保留 501。

---

## 8. 常见问题

| 现象 | 处理 |
| --- | --- |
| `docker compose` 不可用 | 升级 Docker Desktop，或安装 Compose v2 插件 |
| `APP_PORT` 被占用 | 修改 `.env` 中 `APP_PORT` 后 `./deploy.sh restart` |
| 构建拉镜像失败 | 配置镜像加速 / 代理后重试 `./deploy.sh restart` |
| API 报 MCP 失败 | 确认 `mcp` 容器 healthy，且 `.env` 中 `MCP_TOKEN` 与 mcp 服务一致 |
| 未配置 LLM Key | 正常；系统使用离线演示合成，主路径仍可验收 |
