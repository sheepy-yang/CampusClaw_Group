## ADDED Requirements

### Requirement: 可复现启动

系统 SHALL 允许新成员仅依据仓库文件完成配置，并用一条 Compose 命令启动完整项目骨架。

#### Scenario: 从干净环境启动

- **Given** 成员已 clone 仓库、安装 Docker，并按 `.env.example` 填写本机 `.env`
- **When** 成员执行 `docker compose up -d --build`
- **Then** Nginx、Web、API、Worker、MCP、MySQL、Redis、Qdrant 和 MinIO 容器均启动
- **And** 浏览器只能通过 Nginx 发布端口访问项目

#### Scenario: 必填配置缺失

- **Given** `.env` 缺少一个必填变量或该变量为空
- **When** 成员解析 Compose 或启动对应服务
- **Then** 操作以非零状态失败并指出缺失的变量名
- **And** 日志不输出其他配置值或密钥

#### Scenario: 配置值非法

- **Given** 端口、URL、地址或超时配置格式非法
- **When** 服务启动并加载配置
- **Then** 服务在监听端口前退出
- **And** JSON 日志记录稳定错误码 `CONFIG_INVALID`

### Requirement: 健康检查

系统 SHALL 提供免认证的 `GET /health`，以有限超时报告 API 及其依赖的连通状态。

#### Scenario: 全部依赖可用

- **Given** MySQL、Redis、Qdrant、MinIO 和 MCP 均可连接
- **When** 客户端请求 `GET /health`
- **Then** 返回 HTTP 200
- **And** 总状态及每项依赖状态均为 `up`
- **And** 响应不包含口令、Token、DSN 或内部堆栈

#### Scenario: 一个依赖不可达

- **Given** 任一依赖停止、拒绝连接或超过健康检查超时
- **When** 客户端请求 `GET /health`
- **Then** 返回 HTTP 503
- **And** 总状态为 `down`
- **And** 失败依赖为 `down`，其他依赖仍返回各自状态

#### Scenario: 未认证访问健康检查

- **Given** 请求不含 Cookie 或 Authorization
- **When** 客户端请求 `GET /health`
- **Then** 系统执行健康检查而不是返回 401

#### Scenario: 不存在的资源

- **Given** 服务已启动
- **When** 客户端请求未注册路径
- **Then** 返回结构化 404
- **And** 响应包含 `code`、`message` 和 `request_id`

### Requirement: 可追踪日志与错误

系统 SHALL 为每个 HTTP 请求生成或传播安全的请求标识，并输出单行 JSON 日志和统一错误响应。

#### Scenario: 服务端生成请求标识

- **Given** 请求未携带 `X-Request-ID`
- **When** API 处理请求
- **Then** 响应头包含新生成的 `X-Request-ID`
- **And** 请求结束日志包含相同 `request_id`

#### Scenario: 传播合法请求标识

- **Given** 请求携带仅由字母、数字、点、下划线和短横线组成且不超过 128 字符的 `X-Request-ID`
- **When** API 处理请求
- **Then** 响应与日志使用该标识

#### Scenario: 拒绝日志注入标识

- **Given** `X-Request-ID` 含换行、控制字符或超过 128 字符
- **When** API 处理请求
- **Then** 服务端忽略该值并生成新标识
- **And** 日志仍保持一行一条合法 JSON

#### Scenario: 占位业务入口

- **Given** 对应业务尚未进入实现 change
- **When** 客户端调用已声明的业务路由
- **Then** 返回 HTTP 501 和错误码 `NOT_IMPLEMENTED`
- **And** 服务继续运行且不发生 panic

### Requirement: 规约与工程规则

系统 SHALL 将规约、工程约束和验收流程保存到 Harness 可读取且可版本化的位置。

#### Scenario: OpenSpec 四件套齐全

- **Given** 本次骨架变更处于活动状态
- **When** 执行严格 OpenSpec 校验
- **Then** proposal、design、spec 和 tasks 均存在且通过校验
- **And** 每项 Requirement 至少有一个独立可验证 Scenario

#### Scenario: 工程规则可发现

- **Given** 新的 Cursor 会话只获得仓库路径
- **When** 会话准备修改项目文件
- **Then** 自动读取适用的 `.cursor/rules/*.mdc`
- **And** 能说明产品、后端、安全、测试、部署、AI、数据和完成定义约束

#### Scenario: 验证流程可重复

- **Given** 成员准备提交骨架
- **When** 显式调用 `verify-project-skeleton` 技能
- **Then** 技能按规约校验、构建、Compose、健康、日志和同伴复核顺序执行
- **And** 不运行 Git 或 GitHub 命令

### Requirement: MCP 协议骨架

MCP 服务 SHALL 提供受 Bearer Token 保护的 JSON-RPC 入口和免认证健康检查，但不执行教学工具业务。

#### Scenario: 未携带 Token

- **Given** 请求没有有效 `Authorization: Bearer` 头
- **When** 客户端请求 `POST /mcp`
- **Then** 返回 HTTP 401
- **And** 不解析或执行请求体中的工具参数

#### Scenario: 工具发现

- **Given** 请求携带有效 Token
- **When** 客户端发送 `tools/list`
- **Then** 返回 JSON-RPC 2.0 响应
- **And** 每个工具声明包含名称、说明和 `inputSchema`

#### Scenario: 工具执行尚未实现

- **Given** 请求携带有效 Token
- **When** 客户端发送 `tools/call`
- **Then** 返回带原请求 ID 的 JSON-RPC error
- **And** 不调用数据库、模型或教学业务

#### Scenario: 未知协议方法

- **Given** 请求携带有效 Token
- **When** 客户端发送未声明的方法
- **Then** 返回 JSON-RPC `-32601 Method not found`
