# Design: 2026-11-05-learning-state-review-path

## Approach

### 1. 变更边界

本 Change 只交付 Learning State 与 Review Path。继续使用现有 Go 1.22 `net/http`、React 18 + TypeScript、`domain.SkillReviewPath`、`store.ContentRecord`、`store.LearningEvent`、`store.Mistake`、`agent.Orchestrator`、`agent.VerifierAgent`、`security.CheckInput` 和统一错误响应。

不把该功能塞入通用 `teaching.Pipeline`。通用 Pipeline 仍由后续统一编排变更负责；本功能通过独立 Application Service 由 HTTP Handler 调用。其他三个技能在本 Change 中继续保持原有骨架状态。

### 2. 依赖方向与装配

依赖方向遵循 `Transport -> Application -> Domain/Port`，基础设施 Adapter 实现 Port：

```text
cmd/server
  ├── config / gateway / agent
  ├── Memory Store
  ├── MemoryLearningRepository (implements tutoring.Repository)
  ├── tutoring.Services
  └── httpserver.Dependencies
        └── teaching.RegisterReviewPathRoutes
              └── tutoring.Services
```

- `teaching/modules/tutoring` 定义学习状态、复习计划 DTO 和 Repository Port；业务 Service 不直接访问 `store.Default`。
- 新增内存适配器，将现有 `store.Memory` 的 `LearningEvent`、`Mistake`、`ContentRecord` 转换为应用层类型。
- `cmd/server` 负责构造 Repository、Orchestrator、Verifier、AuditLogger 和 `tutoring.Services`，并注入 HTTP Server。
- `httpserver` 只负责把依赖交给教学路由注册函数，不承载业务判断。
- 不改动 `teaching.Pipeline` 的其他技能入口；只将 `review-path` 路由从 501 占位切换到独立 Handler。

### 3. 学习事件与学习状态

#### 3.1 事件模型

在现有 `store.LearningEvent` 兼容扩展可选 `Outcome` 字段，取值限定为：

- `correct`：有明确依据的正确作答；
- `incorrect`：有明确依据的错误作答；
- `understood`：辅导中经规则或评价确认已理解；
- `completed`：完成阅读、例题或复习任务，仅代表活动完成；
- `unknown`：旧数据或无法判定结果的事件。

旧事件的 `Outcome` 为空时按 `unknown` 处理，不回填、不推断正确或错误。`completed` 不等同于 `correct`，不能单独提升掌握等级。

事件仍保留现有 `ID`、`StudentID`、`ClassID`、`Kind`、`Summary`、`KP`、`At` 字段。新增事件必须由服务端补齐身份和班级信息；调用方不能通过请求体指定其他学生或班级。追加操作按事件 ID 幂等，审计只记录必要元数据，不记录完整学生正文。

#### 3.2 有效证据窗口

- 时间统一使用 UTC。
- 时间窗口为请求时刻向前 14 天，包含窗口起点，不包含未来时间。
- `LearningEvent` 必须同时匹配当前 `studentId`、`classId`，且 `KP` 非空、`Kind` 属于当前支持的学习活动类型，才参与知识点汇总。
- `Mistake` 必须匹配当前学生和班级，且 `KnowledgePoint` 非空、`CreatedAt` 在窗口内。
- 当前支持的事件类型至少兼容仓库已有的 `assignment`、`practice`、`help`、`tutoring`，并新增 `review_task`。
- 旧事件即使有 `Kind` 和 `KP`，但没有 `Outcome`，仍可作为“近期学习活动”证明复习路径存在有效输入；它们不能被当作正确或错误证据。
- 旧错题没有稳定 ID 时，作为错题摘要证据使用，不伪造事件 ID。

#### 3.3 知识点状态

每个知识点返回 `masteryLevel`、`mistakeCount`、`correctCount`、`incorrectCount`、`activityCount`、`lastActivityAt`。V1 不返回 0～100 的伪精确分数。

确定性状态规则：

- `weak`：最近 14 天至少有 1 条错题，或至少 2 条明确 `incorrect` 事件；
- `solid`：至少 3 条明确 `correct`/`understood` 事件，且没有最近错题或明确 `incorrect` 事件；
- `developing`：存在明确正确/错误/理解证据，但不满足 `weak` 或 `solid`；
- `insufficient_data`：只有未知或已完成活动记录，没有可用于判断掌握程度的正负证据。

状态优先级与推荐排序固定为 `weak`、`developing`、`insufficient_data`、`solid`。同级按错题数降序、明确错误数降序、最近负向证据时间降序、知识点名称升序排序，确保相同输入产生稳定结果。

学习状态按请求实时计算，不另存一份可能过期的掌握度快照。后续追问辅导和自适应练习可复用同一个 Application Service 与事件写入 Port。

### 4. 复习路径生成

#### 4.1 输入

Service 从 Repository 读取当前学生最近 14 天的错题和学习事件，生成最小化的证据摘要。只向 Orchestrator 提供知识点名称、证据计数、掌握等级、最近活动时间和必要的去标识化摘要；不提供其他学生信息。

对 `Summary`、`QuestionStem`、`ErrorCause` 等不可信文本先使用现有 `security.CheckInput` 检查提示注入。检查失败的文本不进入模型上下文；若过滤后没有可用证据，则按无有效证据处理。

#### 4.2 无证据行为

当最近 14 天没有错题，也没有有效学习事件时：

- 不调用 Orchestrator / LLM；
- 不创建 `ContentRecord`；
- 返回 `skipped=true` 和稳定原因码 `no_recent_learning_evidence`；
- 记录不含学生正文的审计事件。

不能用演示错题、固定学生回答或模型合成数据填充真实学生证据。

#### 4.3 结构化结果

Orchestrator 使用现有 `agent.Orchestrator.Run`，技能标识沿用现有离线合成可识别的“复习路径”。模型只提出候选步骤，不能自行决定学生身份、班级、权限、计划 ID、步骤 ID 或成绩状态。

结果必须包含 3～5 个步骤。每步字段：

- `knowledgePoint`：必须来自本次学习状态候选集合；
- `type`：仅允许 `read`、`example`、`practice`、`self_test`；
- `title`：非空，最多 80 个 Unicode 字符；
- `description`：非空，最多 300 个 Unicode 字符。

服务端生成稳定的步骤 ID 和顺序，初始化 `status=pending`。不接受模型返回的 `studentId`、`classId`、`contentId`、`status` 或 `countsTowardGrade` 覆盖服务端值。

调用现有 `VerifierAgent` 并执行本地 DTO 校验：步骤数量、字段长度、步骤类型、知识点归属、重复项和安全检查。验证失败最多进行 1 次携带问题清单的定向重试；仍失败则返回上游结构化结果错误，不保存计划。

#### 4.4 持久化和可见性

- 使用现有 `store.ContentRecord`，`Skill=review_path`、`ClassID=Principal.ClassID`、`Status=published`。
- `Payload` 保存 `studentId`、`windowDays=14`、生成时间、学习状态摘要、步骤和 `countsTowardGrade=false`。
- 复习路径是学生个人学习建议，不是正式成绩或正式教学内容；生成并校验通过后即可对该学生发布，不走教师内容审核发布流程。
- 学生只能查看自己的路径；本班教师可按 `studentId` 查看本班学生路径；其他学生和跨班教师均返回 404。
- 通用 `GET /api/v1/contents/{id}` 也必须执行复习路径所有者校验，不能绕过专用接口。
- 重新生成创建新的计划记录，旧计划保留，不覆盖历史计划。

### 5. 复习进度与事件回写

学生完成某个复习步骤时，服务端检查计划属于当前学生且步骤存在。完成操作原子地更新步骤状态并追加一条 `review_task`、`Outcome=completed` 的学习事件。

- 同一步骤重复完成返回当前完成状态，不重复追加事件。
- 自测步骤只记录完成活动，不写正式成绩，不自动视为答对。
- 事件的 `StudentID`、`ClassID`、`KP`、`At` 均由已认证身份和计划内容确定。
- 内存适配器通过单次临界区保证计划状态和事件追加的一致性；未来持久化 Adapter 必须使用数据库事务实现同一语义。

### 6. HTTP 契约

| Method | Path | 身份与用途 |
|---|---|---|
| `GET` | `/api/v1/learning/state` | 学生查询自己的近 14 天学习状态；不接受 `studentId` / `classId` 参数 |
| `POST` | `/api/v1/skills/review-path` | 学生为自己生成路径；要求 `Idempotency-Key` 请求头；请求体不接受身份字段 |
| `GET` | `/api/v1/skills/review-path` | 学生查询本人最新路径；教师必须提供 `studentId`，且只能查询本班 |
| `POST` | `/api/v1/skills/review-path/{contentID}/steps/{stepID}/complete` | 学生完成本人计划中的指定步骤；重复请求幂等 |

通用约束：

- 认证使用现有 Bearer Token 和 `auth.Principal`。
- 班级、学生和角色只从服务端 Principal 获取。
- 班级技能开关关闭时返回 403。
- 未认证返回 401；学生越权或跨班资源返回 404；学生访问教师专属读取能力返回 403。
- 请求 DTO 在 Transport 层校验；错误使用现有 `code`、`message`、`request_id` 格式。
- 生成请求的 `Idempotency-Key` 在同一用户、班级和技能范围内唯一；相同 key 重试返回原结果，不重复调用模型或创建计划。
- 模型或 Repository 不可用时返回可识别的 503；模型结果结构无效且定向重试失败时返回 502；均不保存半成品计划。

### 7. 前端

复用 `web/src/pages/student/Home.tsx` 现有 `ReviewPath` 页面和 `web/src/api.ts`，不新增导航路由。页面从 JSON 原始输出调整为：

- 显示近两周学习状态摘要；
- 显示 3～5 个复习步骤、类型、知识点和完成状态；
- 支持逐步标记完成；
- 展示无证据、加载、错误和正常状态；
- 明确标注自测不计入正式成绩。

React 默认文本渲染，不将模型输出作为 HTML 注入。

### 8. 评测与可观察性

- 生成、跳过、查询和步骤完成均写入现有 AuditLogger；审计只记录 actor、class、skill、content/step ID、证据数量、状态和耗时，不记录完整学生答案或模型上下文。
- 为复习路径建立 10～20 条固定 AI 评测样本，覆盖无证据、单一薄弱点、多薄弱点、只有活动记录、未知知识点、越权文本和模型结构错误。
- 普通 `make verify` 使用 Offline/Mock 模型和内存 Repository，不访问真实网络或消耗真实模型额度。

## ADRs

### ADR-1：保留独立 tutoring Application Service，不使用通用 Pipeline

原因：现有通用 Pipeline 在骨架阶段保持 `ErrNotImplemented`，且复习路径是学生个人的同步建议生成，不需要教师审核式任务链路。独立 Service 能避免把个人学习状态和其他成员的内容生产 Pipeline 耦合。

### ADR-2：V1 使用现有 Memory Store，不引入 MySQL 迁移

原因：仓库当前业务模块仍以内存 Store 为演示适配器，MySQL 只在健康检查和 Compose 中就绪。先定义 Repository Port 和内存 Adapter，后续替换存储不改变 Application Service。

### ADR-3：掌握状态使用可解释等级，不使用伪精确分数

原因：现有事件缺少标准化作答结果，旧事件不能被推断为正确或错误。先使用显式错题和带 Outcome 的事件形成弱、发展中、稳固、证据不足四级状态。

## Failure Modes

| Failure | Behavior |
|---|---|
| 无近 14 天有效证据 | 不调用模型、不保存计划，返回 `skipped` |
| 学生输入/历史摘要命中提示注入规则 | 不把该文本送入模型；无剩余证据则跳过 |
| Orchestrator / Repository 不可用 | 返回可恢复错误，不创建半成品计划 |
| 模型步骤数量、知识点或类型不合法 | 最多一次定向重试；仍失败返回 502，不保存 |
| 同一生成请求重试 | 按 Idempotency-Key 返回原结果 |
| 同一步骤并发或重复完成 | 只完成一次、只追加一条事件 |
| 学生访问他人计划 | 返回与资源不存在相同的 404 |
| 教师访问其他班级计划 | 返回 404 |
| 班级禁用 review_path | 返回 403 |
| 进程重启 | V1 内存数据丢失；明确标注演示存储限制，不伪称持久化 |

## Risks

- 当前 `gateway.HTTPClient` 的 `Chat` / `Embed` 仍是离线合成实现；本 Change 只使用现有 Orchestrator 契约，不宣称已经接入真实外部模型。后续真实模型接入需单独完成 Gateway 变更。
- 当前评价模块读取 `LearningEvent` 用于诊断，但没有在本 Change 范围内统一写入作业评价事件。需向其他模块提供内部 `RecordLearningEvent` 契约，后续由模块负责人协商接入。
- `ContentRecord.Payload` 当前是 `map[string]any`；应用层和 HTTP 层仍使用 typed DTO，只有内存适配器负责转换，避免动态类型扩散。
- 仓库仍有骨架阶段规则和未完成的人工交付项；开始实现前必须完成或正式记录阶段切换，不得绕过 `AGENTS.md`。
