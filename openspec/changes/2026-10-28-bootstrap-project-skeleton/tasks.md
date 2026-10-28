# 任务清单

## 1. 规约与 Harness

- [x] 1.1 完成本变更四件套并执行严格校验
  Verify: `openspec validate 2026-10-28-bootstrap-project-skeleton --strict`
- [x] 1.2 建立产品、后端、安全、测试、部署、AI、数据与完成定义规则
  Verify: 新会话只给仓库路径，能够列出八类规则及其检查方式
- [x] 1.3 建立 `verify-project-skeleton` 项目技能
  Verify: 技能元数据合法，步骤覆盖规约、启动、健康、日志和同伴复核

## 2. 可启动骨架

- [x] 2.1 实现环境配置加载与缺失/非法值 fail-fast
  Verify: `go test ./internal/config/...`
- [x] 2.2 实现 API/MCP 优雅启动与 Worker 空壳生命周期
  Verify: 两个 Go 模块运行 `go test ./...` 均通过
- [x] 2.3 启用完整 Compose 拓扑且仅 Nginx 发布端口
  Verify: `docker compose config --quiet`

## 3. 可观察性

- [x] 3.1 实现 API `GET /health` 及逐依赖状态
  Verify: 健康单元测试覆盖全成功与单依赖失败
- [x] 3.2 实现 JSON 请求日志与安全 `request_id`
  Verify: 中间件测试覆盖生成、传播和非法标识替换
- [x] 3.3 统一结构化错误并移除可达路径中的占位 panic
  Verify: 已声明业务路由返回 501 且进程保持运行

## 4. 框架接口

- [x] 4.1 增加 Retrieval、引用冻结和 SSE 事件类型
  Verify: `go test ./internal/retrieval/... ./internal/agent/... ./internal/gateway/...` 编译接口契约
- [x] 4.2 建立 Handler 到 Pipeline/Service 的依赖装配边界
  Verify: Transport 包不被 Domain/Port 包反向导入
- [x] 4.3 使 MCP 信封与工具描述符合 JSON-RPC 2.0 骨架
  Verify: MCP 测试覆盖鉴权、initialize、tools/list、tools/call 和未知方法

## 5. Web 与文档

- [x] 5.1 使用 lockfile 和多阶段生产构建 Web
  Verify: `npm --prefix web ci && npm --prefix web run build`
- [x] 5.2 更新 README、`.env.example`、架构与第 7 课验证记录
  Verify: 新成员无需口头说明即可定位配置、启动、检查和故障排查步骤
- [x] 5.3 增加统一验证入口
  Verify: `make verify`

## 6. 人工交付

- [ ] 6.1 仓库管理员将仓库名改为小写连字符形式并设置主干保护
- [ ] 6.2 一名非作者成员在干净机器上从零启动并填写复核记录
- [ ] 6.3 提交人执行 Git 提交并创建 `v0.0.1-skeleton` 标签
