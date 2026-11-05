# Spec: review-path

## ADDED Requirements

### Requirement: Generate a personalized review path from recent evidence

系统 MUST 使用当前学生最近 14 天的错题、薄弱知识点和有效学习活动生成 3～5 步个性化复习计划。模型只能组织已有证据，不得创造不存在的知识点或学生记录。

#### Scenario: Generate a review path

- **GIVEN** 已认证学生在当前班级有最近 14 天的有效错题或学习事件
- **WHEN** 学生携带 `Idempotency-Key` 调用 `POST /api/v1/skills/review-path`
- **THEN** 系统调用现有 Orchestrator，生成并校验 3～5 步计划，保存为当前学生的已发布个人计划，并返回 `contentId`、步骤和学习状态摘要

#### Scenario: Use existing learning evidence

- **GIVEN** 学生在近 14 天有多个知识点的错题和学习事件
- **WHEN** 系统生成复习路径
- **THEN** 优先覆盖状态为 `weak` 或 `developing` 的知识点，并使用稳定、可解释的排序；不要求模型自行推断知识点依赖关系

#### Scenario: Generate three to five valid steps

- **GIVEN** 模型返回复习路径步骤
- **WHEN** 服务端校验模型结果
- **THEN** 步骤数量为 3～5，每步类型仅为 `read`、`example`、`practice`、`self_test`，知识点必须来自本次证据集合，标题和说明符合长度限制

#### Scenario: No recent evidence

- **GIVEN** 学生近 14 天没有错题，也没有有效学习事件
- **WHEN** 学生生成复习路径
- **THEN** 返回 `skipped=true` 和 `reason=no_recent_learning_evidence`，不调用模型、不保存 `ContentRecord`

#### Scenario: Only activity records without outcomes

- **GIVEN** 学生只有合法的近期活动事件，事件结果为 `unknown` 或 `completed`
- **WHEN** 学生生成复习路径
- **THEN** 系统可以生成用于巩固或自测的计划，但不得将这些活动解释为答题正确或知识点已掌握

#### Scenario: Invalid model output

- **GIVEN** 模型返回少于 3 步、多于 5 步、未知知识点、非法类型或超长字段
- **WHEN** 服务端校验结果
- **THEN** 最多执行一次定向重试；仍不合格时返回 HTTP 502，不保存计划，也不向学生展示不合格内容

#### Scenario: Model or repository unavailable

- **GIVEN** Orchestrator 或 Repository 不可用
- **WHEN** 学生生成复习路径
- **THEN** 返回可识别的 HTTP 503 错误，不创建半成品计划，不使用虚构学生数据兜底

### Requirement: Idempotent generation and plan history

复习路径生成 MUST 支持幂等请求，并保留历史计划版本。

#### Scenario: Retry the same generation request

- **GIVEN** 同一学生、班级和技能已使用某个 `Idempotency-Key` 成功生成计划
- **WHEN** 客户端使用相同 key 重试相同请求
- **THEN** 返回原计划结果，不重复调用模型、不创建第二条计划

#### Scenario: Reuse an idempotency key for a different request

- **GIVEN** 某个 `Idempotency-Key` 已绑定到已有生成请求
- **WHEN** 同一作用域使用该 key 提交不同请求内容
- **THEN** 返回 HTTP 409，且不覆盖原请求结果

#### Scenario: Regenerate after learning evidence changes

- **GIVEN** 学生已有一份复习计划，之后又产生新的学习证据
- **WHEN** 学生使用新的 `Idempotency-Key` 再次生成计划
- **THEN** 系统创建新的计划记录，旧计划保留，最新计划作为当前计划返回

### Requirement: Review path visibility

复习路径 MUST 仅对计划所属学生和该学生所在班级的教师可见。

#### Scenario: Student reads own latest plan

- **GIVEN** 学生已有已发布复习计划
- **WHEN** 学生调用 `GET /api/v1/skills/review-path`
- **THEN** 返回该学生当前班级下最新的个人计划

#### Scenario: Same-class teacher reads a student plan

- **GIVEN** 教师属于该计划所在班级，且请求提供目标 `studentId`
- **WHEN** 教师调用 `GET /api/v1/skills/review-path?studentId=...`
- **THEN** 返回该学生在本班的最新计划

#### Scenario: Student attempts to read another student's plan

- **GIVEN** 学生 A 请求学生 B 的 `contentId` 或通过 `studentId` 查询学生 B
- **WHEN** 系统执行资源授权
- **THEN** 返回 HTTP 404，与资源不存在的响应一致

#### Scenario: Cross-class teacher access

- **GIVEN** 教师请求其他班级学生的计划
- **WHEN** 系统执行资源授权
- **THEN** 返回 HTTP 404

#### Scenario: Generic content endpoint cannot bypass owner check

- **GIVEN** 学生 A 知道同班学生 B 的复习路径 `contentId`
- **WHEN** 学生 A 调用 `GET /api/v1/contents/{id}`
- **THEN** 返回 HTTP 404，不得仅凭同班和 `published` 状态返回 B 的计划

### Requirement: Review step completion updates learning activity

学生完成复习步骤 MUST 更新计划进度并追加一条可追溯学习事件；该操作必须幂等。

#### Scenario: Complete a pending step

- **GIVEN** 学生访问自己的已发布计划，且目标步骤状态为 `pending`
- **WHEN** 学生调用 `POST /api/v1/skills/review-path/{contentID}/steps/{stepID}/complete`
- **THEN** 步骤状态变为 `completed`，写入一条 `kind=review_task`、`outcome=completed` 的事件，并返回更新后的计划进度

#### Scenario: Complete the same step more than once

- **GIVEN** 某步骤已经完成，且对应学习事件已写入
- **WHEN** 学生重复或并发提交完成请求
- **THEN** 返回当前完成状态，不重复追加事件，不重复增加活动计数

#### Scenario: Self-test is not formal grading

- **GIVEN** 学生完成类型为 `self_test` 的复习步骤
- **WHEN** 系统更新计划和学习事件
- **THEN** 只记录 `completed` 活动，不创建正式成绩、不将其自动标记为 `correct` 或 `understood`

#### Scenario: Complete another student's step

- **GIVEN** 学生 A 请求学生 B 的计划或步骤
- **WHEN** 系统处理完成请求
- **THEN** 返回 HTTP 404，计划和学习事件均不发生变化

### Requirement: Review path security and content validation

复习路径 MUST 对输入证据和模型输出执行校验，并遵守现有身份、技能开关和错误响应约束。

#### Scenario: Unauthenticated generation

- **GIVEN** 请求没有有效 Bearer Token
- **WHEN** 调用复习路径生成接口
- **THEN** 返回 HTTP 401，错误码为 `UNAUTHORIZED`

#### Scenario: Student requests teacher-only view

- **GIVEN** 学生尝试使用教师查询方式访问其他学生的路径
- **WHEN** 调用复习路径查询接口
- **THEN** 返回 HTTP 404，不泄露目标学生或资源是否存在

#### Scenario: Disabled skill

- **GIVEN** 当前班级已关闭 `review_path`
- **WHEN** 学生生成或查询复习路径
- **THEN** 返回 HTTP 403，错误码为 `FORBIDDEN`

#### Scenario: Prompt injection in evidence

- **GIVEN** 学习事件摘要或错题文本包含现有 `security.CheckInput` 识别的提示注入内容
- **WHEN** 系统准备模型上下文
- **THEN** 不将该文本作为模型输入；过滤后没有有效证据时按无证据路径处理

#### Scenario: Invalid request or idempotency key

- **GIVEN** 请求体包含 `studentId`/`classId` 等身份覆盖字段，或 `Idempotency-Key` 缺失/格式非法
- **WHEN** Transport 校验请求
- **THEN** 返回 HTTP 400，且不调用模型、不写入计划

#### Scenario: Audit generation and completion

- **GIVEN** 学生生成路径、查询路径或完成步骤
- **WHEN** 对应请求处理结束
- **THEN** AuditLogger 记录 actor、class、操作、资源 ID、证据数量和结果状态，不记录密钥或完整学生正文
