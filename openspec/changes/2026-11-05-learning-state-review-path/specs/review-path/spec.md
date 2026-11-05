# Spec: review-path

## ADDED Requirements

### Requirement: Evidence-based review plan

系统 MUST 使用当前学生最近 14 天的真实学习证据生成并校验 3～5 步复习计划。

#### Scenario: Generate a valid plan

- **Given** 已认证学生在当前班级有近期学习事件或错题
- **When** 学生调用 `POST /api/v1/skills/review-path`
- **Then** 服务端向现有 Orchestrator 提供去标识化证据，并只保存包含 3～5 个合法步骤的计划

#### Scenario: No recent evidence

- **Given** 当前学生没有有效近期证据
- **When** 学生生成复习路径
- **Then** 返回 `skipped=true`，不调用模型、不创建计划，也不使用演示数据填充

#### Scenario: Reject unsafe or invalid model output

- **Given** 证据命中内容安全规则，或模型返回未知知识点、非法步骤类型、错误数量或超长字段
- **When** 服务端校验输入和候选结果
- **Then** 不保存不合格计划，并返回明确的 400 或 502 错误

#### Scenario: Model unavailable

- **Given** Orchestrator 或模型不可用
- **When** 学生生成复习路径
- **Then** 返回 503，不伪造计划成功

### Requirement: Private plan progress

复习路径 MUST 仅对所属学生和本班教师可见，并支持重复安全的步骤完成。

#### Scenario: Read and complete own plan

- **Given** 学生已有本人计划
- **When** 学生查询最新计划并完成一个步骤
- **Then** 返回更新后的结构化计划和进度，并写入一条完成事件

#### Scenario: Self-test does not affect formal grade

- **Given** 学生完成 `self_test` 步骤
- **When** 服务端记录完成
- **Then** 只记录活动完成，不创建正式成绩，也不自动判定掌握

#### Scenario: Deny another student's or cross-class plan

- **Given** 学生请求他人计划，或教师请求其他班级计划
- **When** 服务端执行资源授权
- **Then** 返回与资源不存在一致的 404

#### Scenario: Plan not found

- **Given** 当前学生尚无计划或 content/step ID 不存在
- **When** 查询或完成步骤
- **Then** 查询返回空状态或资源操作返回 404，不创建替代数据
