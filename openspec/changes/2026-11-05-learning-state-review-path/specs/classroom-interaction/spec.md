# Spec: classroom-interaction

## ADDED Requirements

### Requirement: Real classroom interaction lifecycle

系统 MUST 使用本班真实互动和回答完成创建、收集、结束与分析，不得在资源不存在时制造回答。

#### Scenario: Teacher creates and closes an interaction

- **Given** 已认证教师属于当前班级且技能已启用
- **When** 教师创建问题并结束收集
- **Then** 系统生成本班 session ID，按 `open -> closed` 更新状态并写入审计

#### Scenario: Student reads active interaction and submits

- **Given** 当前班级存在开放互动
- **When** 学生查询并提交安全的非空回答
- **Then** 回答绑定服务端学生身份；同一学生再次提交时更新原回答而非增加人数

#### Scenario: Missing interaction

- **Given** session 不存在或当前班级没有开放互动
- **When** 查询、回答或聚类
- **Then** 返回 404 或明确空状态，不创建虚拟回答

### Requirement: Anonymous bounded clustering

课堂聚类 MUST 仅使用真实匿名回答形成最多四组，并由后端校验成员、人数和教师访问范围。

#### Scenario: Skip a small sample

- **Given** 关闭的互动少于 5 份有效学生回答
- **When** 教师触发聚类
- **Then** 返回 `skipped=true`，不调用模型

#### Scenario: Cluster at least five real answers

- **Given** 互动至少有 5 份真实有效回答
- **When** 教师触发聚类
- **Then** 服务端先匿名化并形成最多 4 个组，模型只总结现有组，人数由后端按真实成员计算

#### Scenario: Reject invented or duplicate members

- **Given** 模型总结包含不存在或重复的匿名回答 ID
- **When** 服务端校验结果
- **Then** 拒绝无效成员并记录错误，不把模型人数作为统计事实

#### Scenario: Teacher authorization

- **Given** 学生调用聚类，或教师访问其他班级互动
- **When** 请求分析或结果
- **Then** 分别返回 403 或 404

#### Scenario: Model unavailable

- **Given** 分组已形成但模型总结不可用
- **When** 教师触发分析
- **Then** 返回明确 503 或带算法模式的确定性基础结果，不伪造 AI 建议
