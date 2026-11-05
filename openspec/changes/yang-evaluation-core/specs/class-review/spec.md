# Spec: class-review

## ADDED Requirements

### Requirement: Anonymous teacher-only class review

系统 MUST 只允许本班教师基于足够的有效提交生成匿名化讲评。

#### Scenario: Generate a class review

- **Given** 本班存在至少 5 条有效提交且教师已认证
- **When** 教师发起班级讲评
- **Then** 系统先匿名化输入，再异步生成提纲、例题和补救题，并标记 `teacherOnly`

#### Scenario: Enforce the minimum sample size

- **Given** 本班有效提交少于 5 条
- **When** 教师发起班级讲评
- **Then** 返回 `skipped=true` 且不调用模型

#### Scenario: Remove direct student names

- **Given** 输入包含“李同学”等直接身份称谓
- **When** 数据进入模型边界
- **Then** 模型输入不再包含原称谓，结果也不暴露学生身份

#### Scenario: Reject a student caller

- **Given** 当前身份为学生
- **When** 调用班级讲评接口
- **Then** 返回 403 且不创建任务或内容
