# 设计：小组项目可验证骨架

## 1. 设计目标

骨架只回答四个问题：能否按规约协作、能否从零启动、能否观察依赖状态、能否为后续业务提供稳定边界。十二项教学能力保留接口和目录，不在本变更中实现。

## 2. 分层与依赖

```text
Browser
  → Nginx
    → Web static assets
    → Go API Transport
       → Application / teaching pipeline contracts
          → Domain types and ports
             ← MySQL / Redis / Qdrant / MinIO / MCP / LLM adapters
```

- `cmd/` 是组合根，只装配配置、日志、依赖和进程生命周期。
- HTTP Handler 只解析输入、调用 Application 接口并编码统一响应。
- Application 层执行认证后的角色、班级、资源和流程判定。
- Port 不依赖具体 SDK；Adapter 以后实现外部系统接入。
- `map[string]any` 仅允许停留在 JSON Schema、模型原始响应等协议边界。

## 3. 配置与启动

- `.env.example` 只列键名，`.env` 被忽略。
- Compose 对必填变量使用 `${NAME:?message}`；Go 进程再次校验格式。
- API 必填项包括端口、依赖地址、健康检查超时和会话密钥。
- MCP 必填项包括端口与 Bearer Token。
- 服务响应 SIGTERM 并在超时内优雅退出。

## 4. 健康检查

- `GET /health` 不要求认证。
- API 在有限超时内检查 MySQL TCP、Redis TCP、Qdrant `/readyz`、MinIO `/minio/health/live` 和 MCP `/health`。
- 全部可达时返回 200 与 `status=up`；任一失败返回 503、`status=down` 及逐项状态。
- 健康响应不得包含 DSN、Token、口令或内部错误堆栈。

## 5. 日志与错误

- 使用 Go `slog` JSON Handler，一行一条事件。
- API 接受合法 `X-Request-ID`，否则生成随机标识；响应头和 JSON 错误均返回该标识。
- 请求日志记录 `service`、`event`、`request_id`、方法、路径、状态和耗时。
- 占位业务入口返回 501 与稳定错误码 `NOT_IMPLEMENTED`，不触发 panic。

## 6. Compose 拓扑

- 仅 Nginx 向宿主机发布端口。
- API、Web、MCP、Worker 和数据依赖只在 Compose 网络内可见。
- API 健康检查通过后 Nginx 才进入稳定状态；数据卷用于 MySQL、Qdrant 和 MinIO。
- Web 使用 lockfile、多阶段构建和 Nginx 静态服务，支持 SPA 路由回退。

## 7. 接口契约

- Agent、Gateway、MCP Host、Queue 和教学模块继续作为 Port。
- 新增 Retrieval、Citation Snapshot 和 SSE 事件类型，表达 RAG 与流式链路而不实现它们。
- MCP Server 只实现鉴权、JSON-RPC 信封、握手、工具描述和明确的未实现错误，不执行教学工具。

## 8. 取舍

- 不引入第三方配置、路由或探活库，骨架使用标准库降低初始依赖。
- 健康检查验证“可连接”，不执行数据库写入或业务查询。
- Worker 在骨架阶段只维护可启动生命周期，不消费队列；真实消费将在异步任务 change 中实现。

## 9. 失败模式

- 必填配置缺失：进程在监听端口前以非零状态退出，日志只报告变量名。
- 依赖不可达：进程保持运行，`/health` 返回 503 并标明失败依赖。
- 非法请求标识：服务端忽略并生成新标识，防止日志注入。
- 未认证业务访问：后续认证 change 实现前，业务占位入口只返回 501，不伪装为成功。
- MCP Token 错误：`POST /mcp` 返回 401；`GET /health` 仍可访问。
- 未实现方法：返回结构化 501 或 JSON-RPC error，不发生进程崩溃。
