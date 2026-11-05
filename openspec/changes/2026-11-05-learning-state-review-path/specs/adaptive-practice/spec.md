# Spec: adaptive-practice

## ADDED Requirements

### Requirement: Server-evaluated adaptive practice

系统 MUST 从已校验题库选择题目，并只接受 `answerText`，不得接受客户端声明答题 outcome。

#### Scenario: Start at medium and grade on server

- **Given** 已认证学生选择有题目的知识点
- **When** 创建练习并提交答案文本
- **Then** 首题难度为 `medium`，服务端根据隐藏标准答案产生判定并更新状态

#### Scenario: Increase and finish difficulty

- **Given** 学生连续答对两题，随后连续答对三道 `hard` 题
- **When** 服务端推进状态机
- **Then** 难度按 `easy -> medium -> hard` 提升，并以掌握条件结束练习

#### Scenario: Handle conceptual and calculation errors

- **Given** 判题器识别概念错误或计算错误
- **When** 选择下一题
- **Then** 概念错误回退到配置中真实存在的前置知识点；没有前置时使用当前知识点 easy 题；计算错误保持同层

#### Scenario: Finish at question limit or quit

- **Given** 练习达到 10 题或学生主动退出
- **When** 服务端结束会话
- **Then** 返回练习总结并明确不计入正式成绩

#### Scenario: Repeat a submission

- **Given** 某次提交 ID 已处理
- **When** 客户端重复提交
- **Then** 返回原结果，不重复计题或调整难度

### Requirement: Practice authorization and secrecy

练习接口 MUST 保护标准答案并限制为会话所属学生访问，非法输入不得触发状态迁移。

#### Scenario: Hide standard answer

- **Given** 服务端向学生返回当前题目
- **When** 编码响应
- **Then** 不包含标准答案、可接受答案集合或内部判题规则

#### Scenario: Deny invalid owner or missing session

- **Given** 学生访问他人、跨班或不存在的练习会话
- **When** 提交答案或请求提示
- **Then** 返回 404 且状态不变化

#### Scenario: Reject unsafe answer

- **Given** 答案为空、超长或内容安全检查失败
- **When** 学生提交答案
- **Then** 返回 400，不执行判题和状态迁移
