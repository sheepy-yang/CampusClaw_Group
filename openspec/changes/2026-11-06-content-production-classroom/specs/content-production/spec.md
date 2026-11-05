# Spec: content-production

## ADDED Requirements

### Requirement: Evidence-bound teaching content

系统 MUST 只使用当前教师所在班级的不可变讲义切片生成命题、教案和预习内容，并在保存前完成独立质量校验。

#### Scenario: Generate traceable exercises

- **Given** 当前班级已启用命题技能且教师选择本班讲义
- **When** 教师提交主题、题型、难度和题量
- **Then** 系统返回题目、答案、解析、模型元数据、质量报告和可回溯引用，且每条引用与服务端证据的版本、范围和哈希一致

#### Scenario: Generate a valid lesson plan

- **Given** 教师选择本班讲义、教学目标和课时
- **When** 教师生成教案
- **Then** 系统校验目标、重难点、引用和各阶段分钟数之和等于总课时

#### Scenario: Require effective preview cards

- **Given** 教师从本班讲义生成预习单
- **When** 候选结果少于三张有效知识卡或引用不匹配
- **Then** 系统不保存为合格内容，并返回明确的模型输出错误

#### Scenario: Reject cross-class or unsafe evidence

- **Given** 请求引用其他班级材料，或输入包含提示注入
- **When** 生成服务解析证据
- **Then** 返回与资源不存在一致的 404 或安全错误，不把不可信输入发送给模型

### Requirement: Confirmed and frozen content lifecycle

系统 MUST 要求本班教师确认生成内容后才能发布，并在发布后冻结负载和证据。

#### Scenario: Confirm then publish

- **Given** 内容通过质量校验且处于待确认状态
- **When** 本班教师确认并使用幂等键发布
- **Then** 内容变为已发布、保存冻结副本，重复同一发布安全返回原结果

#### Scenario: Prevent premature or cross-class publication

- **Given** 内容未确认，或请求教师属于其他班级
- **When** 请求发布
- **Then** 返回 409 或不可区分的 404，内容保持不变

#### Scenario: Hide answers from students

- **Given** 学生读取已发布命题内容
- **When** 服务端生成学生视图
- **Then** 不返回答案、解析、质量内部字段或未发布内容

### Requirement: Traceable virtual classroom rehearsal

系统 MUST 使用已确认教案和公开知识边界的合成角色执行异步教学演练，并明确说明其不是对真实学生的评价。

#### Scenario: Queue and complete a rehearsal

- **Given** 本班教师选择已确认教案、目标和 1～10 轮演练
- **When** 教师使用幂等键发起演练
- **Then** API 返回 202 和任务编号，Worker 生成可查看的角色、对话、教案依据、风险时间线和修改建议

#### Scenario: Compare a repeated rehearsal

- **Given** 教师修改并确认教案，且提供同班前次演练编号
- **When** 再次演练
- **Then** 报告基于两个真实报告给出前后风险数量，不声称未被证据支持的教学改进

#### Scenario: Model or worker failure

- **Given** 模型、队列或对象持久化暂时不可用
- **When** 演练失败
- **Then** 已保存的任务和输入不会丢失，状态可轮询，并允许授权用户安全重试

#### Scenario: Never evaluate real students

- **Given** 教师或学生查看演练报告
- **When** 系统展示角色和结论
- **Then** 报告标注“教学演练（非真实学生评价）”，不包含真实学生身份、成绩或个体预测
