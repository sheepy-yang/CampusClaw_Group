# Spec: diagnosis

## ADDED Requirements

### Requirement: Evidence-bound learning diagnosis

系统 MUST 以可追溯学习事件生成诊断，并在教师确认前对学生隐藏。

#### Scenario: Intercept insufficient evidence

- **Given** 当前学生的有效学习事件少于 3 条
- **When** 教师发起学习诊断
- **Then** 仅返回“样本不足”、`intercepted=true` 和 `studentVisible=false`，且不调用模型

#### Scenario: Generate an evidence-bound diagnosis

- **Given** 当前学生在本班存在至少 3 条有效事件
- **When** Worker 完成诊断任务
- **Then** 每项掌握结论均包含对应 `evidence` 事件标识，默认 `studentVisible=false`

#### Scenario: Release a confirmed diagnosis

- **Given** 教师已核对本班诊断内容
- **When** 调用 `POST /api/v1/skills/diagnosis/{id}/release`
- **Then** 内容状态变为 `published` 且 `studentVisible=true`

#### Scenario: Hide a cross-class diagnosis

- **Given** 当前用户请求其他班级的诊断标识
- **When** 服务端执行班级归属校验
- **Then** 返回 404 且不泄露诊断是否存在
