# 变更：建立小组项目可验证骨架

## 为什么做

小组项目需要一个所有成员都能从零启动、持续增量开发并按同一标准验收的基线。当前仓库已经描述十二项教学能力并建立包目录，但尚缺真实 OpenSpec change、Harness 工程规范、完整 Compose、外置配置、依赖健康检查、结构化日志和验收证据，无法满足第 7 课“落骨架”的可观察标准。

## 交付内容

- 建立本变更的 proposal、design、spec 和 tasks，并提供严格校验配置。
- 将产品、后端、安全、测试、部署运维、AI、数据和完成定义写入 Cursor 可读取的项目规则。
- 保留 Web、API、Worker、MCP、Nginx、MySQL、Redis、Qdrant 和 MinIO 的分层目录，并由 Docker Compose 一条命令启动。
- 所有配置从环境变量读取；必填配置缺失或非法时服务启动失败。
- API 提供免认证 `GET /health`，返回 MySQL、Redis、Qdrant、MinIO 和 MCP 的连通状态。
- API 与 MCP 输出单行 JSON 日志，为请求生成或传播 `request_id`。
- 业务路由仅保留类型契约和明确的“尚未实现”响应，不实现十二项教学业务。
- 在 `docs/lesson-07/` 保存验证命令、结果和同伴复核位置。

## 影响范围

- 根目录协作规则、README、环境变量模板、Makefile 与 Docker Compose。
- Go API/MCP 的配置、生命周期、健康检查、日志和错误响应骨架。
- React 生产构建容器与 Nginx 代理配置。
- OpenSpec、Cursor rules/skill、框架测试和第 7 课验证记录。

## 不在本次范围

- 不实现登录、材料入库、检索、对话、作业或十二项教学技能。
- 不调用真实 LLM、Embedding、Qdrant 检索或 MCP 工具。
- 不创建业务数据库表或写入演示业务数据。
- 不部署公网环境，不伪造运行截图、同伴复核或版本标签。
- 不由 Agent 执行仓库改名、分支保护、提交、标签、推送等 Git/GitHub 操作。
