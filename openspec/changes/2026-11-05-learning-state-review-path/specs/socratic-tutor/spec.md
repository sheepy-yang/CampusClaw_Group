# Spec: socratic-tutor

## ADDED Requirements

### Requirement: Server-controlled tutoring session

系统 MUST 为当前学生创建绑定班级和题目的多轮辅导会话，由模型建议追问，由后端规则控制答案释放。

#### Scenario: Start and continue tutoring

- **Given** 已认证学生选择存在的本人错题或明确标记的演示题
- **When** 学生创建会话并连续提交回答
- **Then** 系统保存轮次和历史，返回经过校验的下一轮引导问题

#### Scenario: Release after verified progress

- **Given** 后端确定性评价器连续两轮确认回答命中题目关键概念且未被否定词反转
- **When** 处理第二次有效回答
- **Then** 后端释放该题绑定的完整解析

#### Scenario: Release on limit or explicit request

- **Given** 会话达到最大 5 轮，或学生明确请求讲解
- **When** 服务端推进会话
- **Then** 后端释放解析并记录释放原因

#### Scenario: Model cannot release answer

- **Given** 模型输出包含释放标志、标准答案或越权指令
- **When** 服务端校验模型输出
- **Then** 忽略模型状态指令，并在后端条件未满足时阻止答案泄露

### Requirement: Tutor authorization and safety

辅导接口 MUST 校验会话所有者、班级和输入安全，并在模型不可用时保留已保存状态。

#### Scenario: Deny another student's session

- **Given** 学生请求其他学生或其他班级的会话
- **When** 提交回答或查询会话
- **Then** 返回 404 且会话不变化

#### Scenario: Reject invalid or unsafe answer

- **Given** 回答为空、超长或命中现有 Prompt Injection 检查
- **When** 学生提交回答
- **Then** 返回 400，不调用模型、不增加轮次

#### Scenario: Model unavailable

- **Given** 模型不可用
- **When** 会话需要生成下一轮追问
- **Then** 返回 503 并保留现有会话历史
