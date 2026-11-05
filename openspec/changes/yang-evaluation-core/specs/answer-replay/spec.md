# Spec: answer-replay

## ADDED Requirements

### Requirement: Privacy-preserving answer replay

系统 MUST 只保存明确的定时或主动快照，并按服务端资源归属返回作答时间线。

#### Scenario: Save an allowed snapshot

- **Given** 快照触发类型为 `manual` 或 `interval`
- **When** 学生保存当前作答
- **Then** 系统追加带 UTC 时间的版本并返回完整时间线

#### Scenario: Reject keystroke capture

- **Given** 快照触发类型为 `keydown`
- **When** 客户端尝试保存
- **Then** 系统返回参数错误且不记录逐键行为

#### Scenario: Read another student's attempt

- **Given** 当前学生请求其他学生的 attempt
- **When** 服务端执行资源归属校验
- **Then** 返回 404 且不泄露时间线

### Requirement: Evidence-only originality review

系统 MUST 仅向教师提供可复核的相似证据，不自动生成学术不端结论。

#### Scenario: Review similarity evidence

- **Given** 作答存在历史版本、同次答案或教师提供的参考资料
- **When** 教师查询 similarity
- **Then** 返回相似片段、来源与定位，并保持 `verdict` 为 null
