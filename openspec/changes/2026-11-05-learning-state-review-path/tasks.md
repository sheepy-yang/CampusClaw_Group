# Tasks: 2026-11-05-learning-state-review-path

> 每次只执行一项任务。验证通过后才能勾选；环境缺失或验证失败时保持未勾选。
> 骨架 Change 的 6.1～6.3 是独立课程交付待办，保持原状态，不阻塞本地 MVP 实现，也不得伪造完成证据。

## 0. 规约与阶段基线

- [x] 0.1 将项目切换到严格 OpenSpec 驱动的 MVP 阶段，并将本 Change 重整为四项技能、共享学习状态和 MCP 的统一 V1 规约。
  - Verify: `npx --yes @fission-ai/openspec@1.15.0 validate 2026-11-05-learning-state-review-path --strict`

## 1. Learning State 与复习路径

- [x] 1.1 实现 Outcome、14 天学习状态汇总、四级掌握状态和确定性单元测试。
  - Verify: `cd api && go test -race ./internal/teaching/modules/tutoring/... ./internal/store/...`
- [x] 1.2 实现 3～5 步复习计划生成、输出校验、无证据跳过、查询和步骤完成事件回写。
  - Verify: `cd api && go test -race ./internal/tutoring/...`
- [x] 1.3 接入学习状态/复习路径 HTTP 路由、权限、技能开关、审计和集成测试。
  - Verify: `cd api && go test -race ./internal/httpserver/... ./internal/teaching/...`
- [x] 1.4 将学生复习页面接入真实接口并覆盖 loading、ready、empty、error。
  - Verify: `npm --prefix web run build`

## 2. 追问式辅导

- [x] 2.1 实现题目提供器、确定性概念评价器、会话历史和后端答案释放规则。
  - Verify: `cd api && go test -race ./internal/agent/... ./internal/tutoring/...`
- [x] 2.2 接入辅导 HTTP 路由、资源所有者权限、内容安全、审计和学生聊天页面。
  - Verify: `cd api && go test -race ./internal/httpserver/... && npm --prefix web run build`

## 3. 自适应练习

- [x] 3.1 实现演示题库、服务端判题、三档状态机、hard 完成条件、真实前置配置和重复提交保护。
  - Verify: `cd api && go test -race ./internal/tutoring/...`
- [x] 3.2 接入练习 start/submit/hint 路由、权限、审计和学生练习页面。
  - Verify: `cd api && go test -race ./internal/httpserver/... && npm --prefix web run build`

## 4. 课堂互动

- [x] 4.1 实现教师创建/结束、开放互动查询、回答更新、人数门槛和最多四类的确定性聚类。
  - Verify: `cd api && go test -race ./internal/tutoring/...`
- [x] 4.2 接入课堂互动 HTTP 权限与审计，并完成学生提交和教师分析页面。
  - Verify: `cd api && go test -race ./internal/httpserver/... && npm --prefix web run build`

## 5. MCP Teaching Tools

- [x] 5.1 实现 MCP Server 三个工具、参数 Schema 校验、JSON-RPC 错误和单元测试。
  - Verify: `cd mcp-teaching-tools && go test -race ./... && go vet ./...`
- [x] 5.2 修复 API MCP 配置，实现 initialize、发现缓存、白名单调用、Principal 权限、审计和不可达错误。
  - Verify: `cd api && go test -race ./internal/mcpclient/... ./internal/config/... ./internal/teaching/...`

## 6. 契约、评测与验收

- [x] 6.1 更新 OpenAPI、README、REQUIREMENTS、架构文档和 Makefile 三 Change strict 校验入口。
  - Verify: `make openspec && npm --prefix web run build`
- [x] 6.2 建立复习路径、追问辅导、课堂聚类各至少 10 条固定 AI 评测，并补齐全量 Go/HTTP 测试。
  - Verify: `cd api && go test -race ./... && go vet ./...`
- [x] 6.3 执行统一验证、Compose 配置和端到端演示；记录环境缺失与非生产级限制。
  - Verify: `make verify && docker compose config --quiet`
