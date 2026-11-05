# Spec: learning-state

## ADDED Requirements

### Requirement: Student-scoped recent learning state

系统 MUST 只根据服务端认证的学生和班级，汇总最近 14 天且不晚于当前时刻的学习事件，并使用可解释等级而非伪精确分数。错题可供后续复习路径使用，但本任务不与事件重复折算为掌握率。

#### Scenario: Calculate four-level state

- **Given** 当前学生在本班存在带知识点的近期事件或错题
- **When** 学生请求 `GET /api/v1/learning/state`
- **Then** 系统按知识点返回事件数、有效证据数、正确/错误/理解数、成功率以及 `weak`、`developing`、`solid` 或 `insufficient_data`

#### Scenario: Apply deterministic mastery thresholds

- **Given** 有效证据仅由 `correct`、`understood` 和 `incorrect` 构成
- **When** 系统以 `(correct + understood) / evidenceCount` 计算成功率
- **Then** `evidenceCount < 3` 为 `insufficient_data`；否则成功率小于 0.5 为 `weak`，大于等于 0.5 且小于 0.8 为 `developing`，大于等于 0.8 为 `solid`

#### Scenario: Handle legacy outcomes

- **Given** 旧事件的 `Outcome` 为空
- **When** 系统汇总学习状态
- **Then** 该事件按 `unknown` 计为活动，但不计为正确、错误或已理解

#### Scenario: Enforce time and tenant scope

- **Given** Store 中同时存在窗口外、未来、其他学生或其他班级的数据
- **When** 当前学生查询状态
- **Then** 返回结果不包含这些数据

#### Scenario: Reject unauthenticated or disabled access

- **Given** 请求未认证，或本班已关闭 `review_path`
- **When** 调用学习状态接口
- **Then** 分别返回 401 或 403，且不泄露学习数据

### Requirement: Safe learning event updates

系统 MUST 由服务端确定事件身份和班级，并防止同一业务动作重复计入。

#### Scenario: Record a completed review step

- **Given** 学生完成本人复习计划中的一个待完成步骤
- **When** 服务端更新计划
- **Then** 追加一条 `review_task/completed` 事件，身份、班级、知识点和 UTC 时间均由服务端确定

#### Scenario: Repeat the same completion

- **Given** 同一步骤已经完成并产生事件
- **When** 客户端重复提交
- **Then** 返回当前状态，不追加第二条事件

#### Scenario: Reject invalid outcome

- **Given** 待写事件的 Outcome 不属于 `correct`、`incorrect`、`understood`、`completed`、`unknown`
- **When** 服务端校验事件
- **Then** 返回参数错误且不写入 Store
