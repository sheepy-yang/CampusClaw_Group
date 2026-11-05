# Tasks: 2026-11-05-learning-state-review-path

> 每次只执行一项任务。任务验证通过后再勾选；未通过时保持未勾选。
> 本 Change 的业务实现受 `proposal.md` 中 Prerequisite 约束：项目阶段未正式切换前，只能完善规约，不得开始业务代码。

## 0. 项目阶段门禁

- [ ] 0.1 确认骨架阶段人工交付项已完成，或由项目提交人记录明确例外、负责人和补齐日期。
  - Verify: 检查 `openspec/changes/2026-10-28-bootstrap-project-skeleton/tasks.md` 第 6 节及对应同伴复核记录；未完成项必须有书面例外记录。
- [ ] 0.2 将项目阶段约束切换为规约驱动的业务实现阶段。
  - Verify: 检查 `AGENTS.md`、`openspec/config.yaml` 和 `REQUIREMENTS.md` 的阶段描述一致；明确只有严格校验通过的 OpenSpec Change 才能实现业务代码。

## 1. 规约与接口契约

- [ ] 1.1 完成本 Change 的 proposal、design、learning-state spec、review-path spec 和 tasks。
  - Verify: `npx --yes @fission-ai/openspec@1.15.0 validate 2026-11-05-learning-state-review-path --strict`
- [ ] 1.2 更新 OpenAPI：学习状态、复习路径生成/查询、步骤完成、幂等头和错误响应。
  - Verify: 使用仓库现有 OpenAPI 校验方式校验 `api/openapi/openapi.yaml`；逐项核对路径、方法、DTO、401/403/404/409/502/503。
- [ ] 1.3 更新 Makefile，使 `make verify` 严格校验骨架 Change、现有评价 Change 和本 Change。
  - Verify: `make openspec`；输出中三个 Change 均执行 strict validation。

## 2. 学习状态领域与 Repository

- [ ] 2.1 定义 typed LearningEvent、LearningState、KnowledgePointState、MistakeEvidence DTO 和合法枚举。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`
- [ ] 2.2 定义 `tutoring.Repository` Port，避免 Application Service 直接访问 `store.Default`。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`；检查 tutoring 包不直接导入 `internal/store`。
- [ ] 2.3 为现有 Memory Store 实现 Repository Adapter，复用 `LearningEvent`、`Mistake` 和 `ContentRecord`。
  - Verify: `cd api && go test ./internal/teaching/adapters/... ./internal/store/...`
- [ ] 2.4 扩展学习事件 Outcome 和幂等追加能力，兼容 Outcome 为空的旧事件。
  - Verify: `cd api && go test ./internal/store/... ./internal/teaching/adapters/...`
- [ ] 2.5 实现 14 天窗口、知识点汇总、掌握等级和稳定排序。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`；测试覆盖窗口边界、未来事件、跨学生/班级、unknown/completed 和四种等级。
- [ ] 2.6 为 Learning State Service 增加固定时钟和 Fake Repository 单元测试。
  - Verify: `cd api && go test -race ./internal/teaching/modules/tutoring/...`

## 3. 复习路径 Application Service

- [ ] 3.1 改造 `GenerateReviewPath`，使用近 14 天学习状态和错题摘要，不再只向模型传错题数量。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`；Fake Orchestrator 断言输入含知识点证据而非仅计数。
- [ ] 3.2 实现无有效证据跳过逻辑，确保不调用模型且不保存 ContentRecord。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`；Fake Orchestrator 调用次数为 0。
- [ ] 3.3 定义 ReviewPlan / ReviewStep typed DTO，并校验 3～5 步、知识点白名单、类型和字段长度。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`
- [ ] 3.4 接入现有 VerifierAgent 和最多一次定向重试；失败时不保存半成品计划。
  - Verify: `cd api && go test ./internal/teaching/modules/tutoring/...`；覆盖首次通过、重试后通过、重试仍失败。
- [ ] 3.5 实现生成幂等、历史计划保留和最新计划查询。
  - Verify: `cd api && go test -race ./internal/teaching/modules/tutoring/...`；相同 key 不重复调用模型，不同 key 创建新版本。
- [ ] 3.6 实现复习步骤完成的原子更新与 `review_task/completed` 事件回写。
  - Verify: `cd api && go test -race ./internal/teaching/modules/tutoring/...`；并发完成同一步骤只产生一条事件。

## 4. HTTP 接入与权限

- [ ] 4.1 在 `cmd/server` 装配 Memory Repository、Tutoring Service、Verifier 和 AuditLogger，并注入 HTTP Server。
  - Verify: `cd api && go test ./cmd/server ./internal/httpserver/...`
- [ ] 4.2 将 `POST /api/v1/skills/review-path` 从 Pipeline 501 占位切换为专用 Handler，其他三个技能保持原行为。
  - Verify: `cd api && go test ./internal/httpserver/... ./internal/teaching/...`；确认 review-path 正常路由，socratic-tutor/classroom-cluster/adaptive-practice 仍为 501。
- [ ] 4.3 实现 `GET /api/v1/learning/state`、复习路径 GET 和步骤完成 POST。
  - Verify: `cd api && go test ./internal/teaching/... ./internal/httpserver/...`
- [ ] 4.4 对所有读写操作执行 Principal、班级、技能开关和资源所有者校验。
  - Verify: `cd api && go test ./internal/teaching/... ./internal/httpserver/...`；覆盖 401、403、同班其他学生 404、跨班 404。
- [ ] 4.5 修复 `GET /api/v1/contents/{id}` 对 `review_path` 的学生所有者校验，避免通过通用内容接口绕过权限。
  - Verify: `cd api && go test ./internal/teaching/...`；学生 A 读取学生 B 的 contentId 返回 404。
- [ ] 4.6 为生成、跳过、读取和步骤完成写入脱敏审计事件。
  - Verify: `cd api && go test ./internal/teaching/...`；断言审计包含 actor/class/action/resource/status，不含完整正文和密钥。

## 5. 学生端页面

- [ ] 5.1 更新 `web/src/pages/student/Home.tsx` 的 ReviewPath 页面，展示学习状态、复习步骤和完成状态。
  - Verify: `npm --prefix web run build`
- [ ] 5.2 为生成请求添加 `Idempotency-Key`，接入查询和步骤完成接口。
  - Verify: `npm --prefix web run build`；浏览器网络面板确认身份字段不由客户端提交，生成请求包含幂等头。
- [ ] 5.3 覆盖 loading、ready、empty、skipped、error 状态，并标明自测不计入正式成绩。
  - Verify: 浏览器手工检查六种页面状态；模型输出以普通文本安全渲染。

## 6. 自动化测试与 AI 评测

- [ ] 6.1 为学习状态与复习路径补齐单元测试、HTTP 集成测试和权限测试。
  - Verify: `cd api && go test -race ./... && go vet ./...`
- [ ] 6.2 建立 10～20 条固定 Review Path AI 评测样本，覆盖无证据、弱知识点、只有活动记录、未知知识点和不合规模型输出。
  - Verify: 执行固定评测脚本；输出每条样本的预期、实际、通过状态和模型/提示词版本。
- [ ] 6.3 确认普通测试使用 Offline/Mock，不访问真实模型或外部网络。
  - Verify: 在无 `LLM_API_KEY`、无外网的环境运行 `cd api && go test ./...`。

## 7. 集成验收

- [ ] 7.1 执行统一验证入口。
  - Verify: `make verify`
- [ ] 7.2 完成本 Change 对应的干净环境启动与端到端冒烟。
  - Verify: `make up` 后用学生账号生成复习路径、查询路径、完成一个步骤并再次查询学习状态；记录请求与响应状态。
- [ ] 7.3 由非作者成员按本 Change 的 Scenario 交叉复核并记录结果。
  - Verify: 提交可复核的交叉评审记录，覆盖正常、失败、越权和依赖不可用路径。
