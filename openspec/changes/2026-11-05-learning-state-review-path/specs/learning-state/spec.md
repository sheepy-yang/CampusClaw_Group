# Spec: learning-state

## ADDED Requirements

### Requirement: Student-scoped learning state

系统 MUST 基于服务端认证身份，为学生计算近 14 天、限定当前班级的学习状态。查询不得接受客户端指定的 `studentId` 或 `classId` 来扩大数据范围。

#### Scenario: Read own learning state

- **GIVEN** 已认证学生，且当前班级存在该学生近 14 天的学习事件或错题
- **WHEN** 学生请求 `GET /api/v1/learning/state`
- **THEN** 系统只返回该学生、该班级、最近 14 天内的知识点汇总，并返回窗口起止时间

#### Scenario: Exclude events outside the time window

- **GIVEN** 学生存在早于 14 天窗口或晚于当前时刻的事件
- **WHEN** 系统计算学习状态
- **THEN** 这些事件不参与知识点计数、掌握等级或复习优先级

#### Scenario: Isolate student and class evidence

- **GIVEN** 同一 Store 中存在其他学生或其他班级的事件和错题
- **WHEN** 当前学生查询学习状态
- **THEN** 返回结果不包含其他学生或其他班级的数据

#### Scenario: Do not infer outcomes from legacy events

- **GIVEN** 旧学习事件没有 `Outcome` 字段或其值为空
- **WHEN** 系统计算学习状态
- **THEN** 将其按 `unknown` 处理，只计入有效学习活动，不计为正确、错误或已理解

### Requirement: Explainable mastery levels

系统 MUST 使用显式学习证据计算知识点等级，不得在证据不足时生成伪精确掌握度分数。

#### Scenario: Mark a knowledge point weak

- **GIVEN** 某知识点近 14 天至少有一条错题，或至少两条明确错误事件
- **WHEN** 系统计算该知识点状态
- **THEN** `masteryLevel` 为 `weak`，并返回对应错题数、错误数和最近活动时间

#### Scenario: Mark a knowledge point solid

- **GIVEN** 某知识点近 14 天至少有三条明确 `correct` 或 `understood` 事件，且没有错题或明确错误事件
- **WHEN** 系统计算该知识点状态
- **THEN** `masteryLevel` 为 `solid`

#### Scenario: Mark a knowledge point developing

- **GIVEN** 某知识点有明确正确、错误或已理解证据，但不满足 `weak` 或 `solid` 条件
- **WHEN** 系统计算该知识点状态
- **THEN** `masteryLevel` 为 `developing`

#### Scenario: Mark insufficient data

- **GIVEN** 某知识点只有 `unknown` 或 `completed` 活动，没有明确正确/错误/已理解证据和错题
- **WHEN** 系统计算该知识点状态
- **THEN** `masteryLevel` 为 `insufficient_data`，不得将任务完成视为掌握

#### Scenario: Stable ordering

- **GIVEN** 两次计算使用完全相同的学习证据
- **WHEN** 系统生成知识点列表
- **THEN** 按 `weak`、`developing`、`insufficient_data`、`solid` 优先级及约定的计数、时间和知识点名称规则返回相同顺序

### Requirement: Append-only and idempotent learning events

系统 MUST 通过服务端 Application Service 追加学习事件。事件身份和班级必须来自可信服务端上下文；同一事件 ID 不得重复写入。

#### Scenario: Append a valid learning event

- **GIVEN** 服务端已验证学生身份，且事件包含合法类型、知识点、结果和来源
- **WHEN** Application Service 追加事件
- **THEN** 事件写入当前学生和班级范围，时间以 UTC 保存，并可被后续学习状态查询读取

#### Scenario: Idempotent event append

- **GIVEN** 同一事件 ID 已经写入
- **WHEN** 上游因重试再次追加该事件
- **THEN** 系统不产生第二条事件，查询结果中的计数不增加

#### Scenario: Reject invalid event outcome

- **GIVEN** 事件结果不是 `correct`、`incorrect`、`understood`、`completed` 或 `unknown`
- **WHEN** Application Service 追加事件
- **THEN** 返回参数错误且不写入事件

#### Scenario: Reject client-supplied identity

- **GIVEN** 学生请求中包含其他 `studentId` 或 `classId`
- **WHEN** Transport 处理请求
- **THEN** 不使用请求体身份字段；学习状态仍严格限定为 Principal 中的学生和班级

### Requirement: Learning state authorization and failure handling

学习状态接口 MUST 执行认证、技能开关和资源范围校验，并按统一错误格式返回失败。

#### Scenario: Unauthenticated request

- **GIVEN** 请求没有有效 Bearer Token
- **WHEN** 调用 `GET /api/v1/learning/state`
- **THEN** 返回 HTTP 401，错误码为 `UNAUTHORIZED`

#### Scenario: Disabled review capability

- **GIVEN** 当前班级已关闭 `review_path` 技能
- **WHEN** 学生查询学习状态或生成复习路径
- **THEN** 返回 HTTP 403，错误码为 `FORBIDDEN`

#### Scenario: Repository unavailable

- **GIVEN** Learning Repository 查询或追加事件时发生依赖错误
- **WHEN** 学生查询学习状态
- **THEN** 返回可识别的 HTTP 503 错误，不返回伪造的空掌握状态
