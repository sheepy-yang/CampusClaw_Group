# 架构说明（项目骨架）

## 1. 当前边界

当前 change 只建立可启动、可观察、可验证的工程基线。十二项教学能力保留路由、类型和 Port，调用时明确返回 `NOT_IMPLEMENTED`；数据库业务表、模型调用、检索算法和工具执行均不在骨架范围。

## 2. 容器拓扑

```text
Browser
  │ 唯一宿主机入口
  ▼
Nginx :${APP_PORT}
  ├─ /              → Web Nginx :80 → React 静态文件
  ├─ /api/*         → Go API :${HTTP_PORT}
  └─ /health        → Go API :${HTTP_PORT}

Go API（内部网络）
  ├─ MySQL :3306
  ├─ Redis :6379
  ├─ Qdrant :6333
  ├─ MinIO :9000
  └─ MCP :${MCP_PORT}

Worker（内部网络）→ Redis
```

数据库、队列、向量库、对象存储、API 和 MCP 均不向宿主机发布端口。

## 3. Go 依赖方向

```text
cmd/server（组合根）
  → httpserver / httpapi（Transport）
    → teaching / platform（Application）
      → agent / retrieval / gateway / mcpclient / queue / security（Port）
        ← 后续 change 中的基础设施 Adapter
```

约束：

- `cmd/` 只装配配置、日志、Port 和进程生命周期。
- Transport 解析协议并统一编码错误，不实现流程规则。
- Application 使用服务端认证上下文判定角色、班级和资源归属。
- Port 不导入具体数据库、模型或 MCP SDK。
- Adapter 可以依赖 Port，Port 不得反向依赖 Adapter。
- 外部调用不得放在数据库事务内。

## 4. 运行与可观察性

- API 和 MCP 从环境变量加载配置，缺失或格式非法时在监听端口前退出。
- `GET /health` 免认证；API 并发检查 MySQL、Redis、Qdrant、MinIO 和 MCP。
- 请求日志使用 `slog` JSON Handler，一行一条，并传播经过白名单校验的 `X-Request-ID`。
- HTTP 错误使用 `code`、`message`、`request_id` 信封。
- API、MCP 和 Worker 响应 SIGTERM；API/MCP 使用有超时的优雅关闭。

## 5. AI 与数据边界

- `gateway` 定义文本、多模态、Embedding 和 SSE 事件，不包含提供商实现。
- `retrieval` 定义班级范围、检索查询和不可变 `ChunkRef`，引用绑定材料版本、切片、字符区间与内容摘要。
- `mcpclient` 的工具结果只作为不可信数据；MCP Server 只提供鉴权和合法 JSON-RPC 协议壳。
- `queue` 的 Worker 在骨架阶段只保持进程生命周期，不消费或伪造任务结果。
- MySQL 是业务事实源；Qdrant 以后只保存向量和最小关联标识；MinIO 保存原文件。

## 6. 成员模块

- 刘恺骐：`api/internal/teaching/modules/content`、`api/internal/agent`、统一 Pipeline、模型与检索集成。
- 杨瑾文：`api/internal/teaching/modules/evaluation`、`api/internal/queue`。
- 刘沛鑫：`api/internal/teaching/modules/tutoring`、`mcp-teaching-tools`。
- 三人共同维护根目录 Compose、OpenSpec、规则、测试、文档和部署。

## 7. 后续增量方式

每项业务先创建独立 OpenSpec change，补齐正常、参数非法、未认证、无权限、资源不存在和依赖不可达 Scenario，再新增 Adapter 与业务实现。历史 change 验收后归档，禁止直接在骨架 change 中扩展业务范围。
