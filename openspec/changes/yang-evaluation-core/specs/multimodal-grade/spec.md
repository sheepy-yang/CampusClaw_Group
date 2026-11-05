# Spec: multimodal-grade

## ADDED Requirements

### Requirement: Traceable multimodal grading

系统 MUST 在保存学生提交后异步执行转写与主观题评价，并由教师保留最终评分权。

#### Scenario: Grade a subjective submission

- **Given** 学生提交主观题文本或手写图片摘要
- **When** 调用 `POST /api/v1/skills/multimodal-grade`
- **Then** 系统先保存提交再入队，完成后返回转写、步骤划分、首错定位和建议分

#### Scenario: Grade an objective answer deterministically

- **Given** 请求包含学生答案与标准答案
- **When** 调用 `POST /api/v1/skills/multimodal-grade/objective`
- **Then** 系统使用确定性规则判分并返回 `usedLLM=false`

#### Scenario: Confirm a low-confidence transcription

- **Given** 学生确认的转写置信度低于自动采纳阈值
- **When** 调用转写确认接口
- **Then** 提交进入 `needs_teacher_review`，不自动确定最终成绩

#### Scenario: Override a suggested score

- **Given** 教师已认证且提交属于本班
- **When** 教师提交最终分与说明
- **Then** 最终分以教师结果为准并追加 `scoreHistory`

### Requirement: Grading isolation and failure retention

系统 MUST 隔离其他学生与其他班级的数据，并在模型失败时保留原始提交。

#### Scenario: Read another student's submission

- **Given** 当前学生请求其他学生的 submission
- **When** 服务端执行资源归属校验
- **Then** 返回 404 且不泄露提交是否存在

#### Scenario: Model processing fails

- **Given** 模型或离线处理器返回失败
- **When** Worker 处理批改任务
- **Then** 原始提交仍保留且状态为 `failed_submission_kept`
