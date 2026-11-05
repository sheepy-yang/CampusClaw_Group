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
cp -n .env.example .env   # 已有 .env 则跳过
docker compose up -d --build
```

部署成功后访问：

| 入口 | 地址 |
| --- | --- |
| 统一入口（Nginx） | http://localhost |
| 前端开发服务 | http://localhost:5173 |
| API 健康检查 | http://localhost:8080/healthz |
| MCP 健康检查 | http://localhost:8100/healthz |

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
make up       # 一键部署
make down     # 停止
make logs     # 日志
make status   # 状态
```

---

## 3. 运行时服务与端口

| 服务 | 容器名意图 | 宿主机端口 | 说明 |
| --- | --- | --- | --- |
| nginx | 反向代理 | **80** | 浏览器统一入口 |
| web | React + Vite | 5173 | 前端；`/api` 代理到 API |
| api | Go API（内嵌 Worker） | 8080 | 教学技能与平台接口 |
| mcp | mcp-teaching-tools | 8100 | 教学 MCP 工具 |
| mysql | MySQL 8.0 | 3306 | 业务库（主路径可先用内存，库已就绪） |
| redis | Redis 7 | 6379 | 异步任务扩展预留 |
| qdrant | Qdrant | 6333 | 向量检索扩展预留 |
| minio | MinIO | 9000 / 9001 | 对象存储扩展预留 |

端口冲突时，修改 `docker-compose.yml` 的 `ports` 映射或释放占用端口。

---

## 4. 环境变量要求（`.env`）

从 `.env.example` 复制生成 `.env`。一键脚本会自动完成复制。

| 变量 | 是否必填 | 说明 |
| --- | --- | --- |
| `SESSION_SECRET` | 建议填写 | 会话签名密钥；演示可留空或使用示例值 |
| `MCP_TOKEN` | 建议填写 | API 调用 MCP 的 Bearer Token，默认 `dev-mcp-token` |
| `MCP_BASE_URL` | 容器内默认 | Compose 中为 `http://mcp:8100` |
| `MYSQL_DSN` | 有默认 | MySQL 连接串 |
| `REDIS_URL` | 有默认 | Redis 地址 |
| `QDRANT_URL` | 有默认 | Qdrant 地址 |
| `MINIO_*` | 有默认 | 对象存储凭证（演示账号，生产必须更换） |
| `LLM_API_KEY` | 可选 | 为空则使用离线演示合成，不调外部大模型 |
| `LLM_BASE_URL` | 可选 | 真实模型网关地址 |
| `EMBEDDING_API_KEY` / `EMBEDDING_BASE_URL` | 可选 | 向量模型 |
| `HTTP_PORT` | 可选 | API 监听端口，默认 `8080` |

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
curl -s http://localhost:8080/healthz
# 期望：{"status":"ok"}

curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"role":"teacher"}'
# 期望：返回 token
```

浏览器打开 http://localhost ，教师登录后可运行「智能命题」等技能。

---

## 8. 常见问题

| 现象 | 处理 |
| --- | --- |
| `docker compose` 不可用 | 升级 Docker Desktop，或安装 Compose v2 插件 |
| 80 端口被占用 | 改 `docker-compose.yml` 中 nginx 端口，或关闭占用进程 |
| 构建拉镜像失败 | 配置镜像加速 / 代理后重试 `./deploy.sh restart` |
| API 报 MCP 失败 | 确认 `mcp` 容器 healthy，且 `.env` 中 `MCP_TOKEN` 与 mcp 服务一致 |
| 未配置 LLM Key | 正常；系统使用离线演示合成，主路径仍可验收 |
