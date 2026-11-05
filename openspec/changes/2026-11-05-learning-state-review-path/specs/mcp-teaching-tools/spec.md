# Spec: mcp-teaching-tools

## ADDED Requirements

### Requirement: Authenticated discovery without fake fallback

API MCP Host MUST 使用 `MCP_URL` 和 `MCP_TOKEN` 完成初始化和工具发现，并只允许调用真实发现且在白名单中的工具。

#### Scenario: Initialize and cache tool schemas

- **Given** MCP Server 可达且 Token 有效
- **When** Host 首次发现工具
- **Then** 依次调用 `initialize`、`tools/list`，缓存工具名称、描述和 `inputSchema`

#### Scenario: MCP unavailable

- **Given** MCP Server 超时、拒绝连接或返回非法 JSON-RPC
- **When** Host 发现或调用工具
- **Then** 返回明确错误，不回退到静态工具列表或伪造成功

#### Scenario: Reject unknown tool or invalid arguments

- **Given** 工具未被发现、不在白名单，或参数不符合发现的 Schema
- **When** API 请求调用工具
- **Then** 在调用前拒绝并记录稳定错误码

### Requirement: Real conservative tool execution

MCP Server MUST 对三个白名单工具执行真实且保守的确定性处理，不得读取不存在的共享 Store 或伪造业务数据。

#### Scenario: Verify a formula conservatively

- **Given** 参数符合 `formula_verify` Schema
- **When** 工具能够确定两个受支持表达式等价
- **Then** 返回 `verified=true`；无法可靠判断时返回 `indeterminate` 或 `verified=false`

#### Scenario: Calculate assignment statistics

- **Given** API 已验证教师和班级，并传入限定范围的匿名作答数据
- **When** 调用 `assignment_stats`
- **Then** 工具只计算该输入的聚合统计，不读取共享内存或扩大数据范围

#### Scenario: Summarize student mistakes

- **Given** API 已验证当前学生本人或本班教师，并传入限定错题摘要
- **When** 调用 `student_mistakes`
- **Then** 工具只整理输入摘要，不接受模型指定其他学生或班级

### Requirement: MCP authorization and audit

API MUST 在调用 MCP 前执行 Principal 权限与数据范围校验，并对成功和失败调用写入脱敏审计。

#### Scenario: Deny unauthorized scope

- **Given** 学生请求他人错题，或学生请求 `assignment_stats`，或教师请求其他班级数据
- **When** API 执行工具权限检查
- **Then** 返回 403 或 404，且不向 MCP Server 发送敏感参数

#### Scenario: Audit every call safely

- **Given** 工具调用成功或失败
- **When** 请求结束
- **Then** 审计记录 actor、role、class、tool、request ID、状态、耗时和错误码，不记录 Token、API Key 或完整敏感正文

#### Scenario: MCP output is untrusted

- **Given** MCP 返回缺字段、超长文本或指令性内容
- **When** API 解析工具结果
- **Then** 服务端校验或拒绝结果，不把它作为系统指令执行
