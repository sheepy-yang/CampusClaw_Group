# Design: 2026-11-05-learning-state-review-path

## Architecture

沿用 `Transport -> Application -> Domain/Port`。四项技能通过独立 tutoring Service 和专用 HTTP Handler 接入，不塞入通用 `teaching.Pipeline`；`cmd/server` 只负责装配 Memory Store、Agent、Gateway、MCP Host、AuditLogger 和 Handler。

```text
React pages
  -> /api/v1 tutoring handlers
    -> tutoring services
      -> Memory Store / Orchestrator / Gateway / MCP Host / AuditLogger
```

身份、角色、班级和资源所有者只来自 `auth.Principal`。模型输出、学生文本和 MCP 返回均在服务端校验。跨班资源和他人私有资源统一返回 404；学生调用教师专属操作返回 403。

## Learning State and Review Path

`LearningEvent.Outcome` 可为空以兼容旧数据；合法值为 `correct`、`incorrect`、`understood`、`completed`、`unknown`，空值按 `unknown`。只汇总当前学生、当前班级、最近 14 天且不晚于当前时刻的事件。现有错题继续保留给后续复习路径作为独立上下文，本任务不把错题与事件重复折算为掌握率。

有效证据仅包括 `correct`、`understood` 和 `incorrect`；成功率为 `(correct + understood) / evidenceCount`。`completed`、`unknown` 和旧事件空值只计入活动总数，不进入分母。MVP 启发式等级为：

- `insufficient_data`：有效证据少于 3 条；
- `weak`：有效证据至少 3 条且成功率小于 0.5；
- `developing`：成功率大于等于 0.5 且小于 0.8；
- `solid`：成功率大于等于 0.8。

无证据时不调用模型、不创建计划。有证据时 Orchestrator 只接收去标识化摘要；服务端校验 3～5 个步骤、知识点白名单、步骤类型和字段长度。计划保存在 `ContentRecord`，仅本人和本班教师可见。步骤重复完成不重复追加 `review_task/completed` 事件，自测不计入成绩。

## Socratic Tutor

会话绑定 `studentID`、`classID` 和题目。小型题目提供器保存题干、解析、关键概念组合和否定词；缺少课程题库时可返回明确标记的演示题，不能冒充真实课程数据。模型只生成下一轮追问，后端评价器根据概念组合、否定词和尝试次数更新命中状态。

完整解析只在连续两轮命中、达到最大 5 轮或学生明确请求讲解时由后端释放。模型不能写入 `Released`，其他学生或跨班用户不能读取、推进会话。

## Adaptive Practice

题库条目包含知识点、难度、题干、标准答案、可接受答案、错误类型和可选前置知识点。服务端校验题目完整性后再展示，并移除标准答案。提交只接受 `answerText`，由确定性判题器产生 outcome。

练习从 `medium` 开始；连续答对两题升级；连续三道 `hard` 正确结束；概念错误回退到配置中真实存在的前置知识点，否则留在当前知识点的 `easy`；计算错误保持同级；最多 10 题或主动退出结束。提交 ID 防止重复计数，所有状态更新在 Store 临界区内完成。

## Classroom Interaction

教师创建本班互动并生成 session ID，状态为 `open`。学生只读取本班开放互动；同一学生重复提交更新原回答。教师结束收集后才能聚类。少于 5 份有效回答返回 `skipped` 且不调用模型。

V1 先使用匿名文本的字符 n-gram/词项相似度形成最多 4 个确定性分组；若 Gateway Embedding 可用可替换相似度来源。LLM 只能总结已形成组及其真实匿名成员，服务端校验成员集合并计算人数，不允许模型增加回答。

## MCP Teaching Tools

统一使用 `MCP_URL` 和 `MCP_TOKEN`。Host 依次执行 `initialize`、`tools/list`，缓存完整工具描述和 `inputSchema`；连接失败返回明确错误，不回退到静态假列表。调用前检查真实发现结果、白名单、Schema、Principal 权限和数据范围。

- `formula_verify`：保守处理简单数值/规范化表达式；不能确定时返回 `indeterminate`。
- `assignment_stats`：仅教师；API 授权并取得本班最小化统计输入后交给工具计算。
- `student_mistakes`：学生仅本人，教师仅本班；API 提供已限定范围的错题摘要。

每次调用审计 actor、role、class、tool、request ID、状态、耗时和错误码，不记录 Token、密钥或完整敏感正文。

## Frontend and API

复用现有学生/教师工作台、`api.ts`、`SkillPanel` 和样式。页面显示结构化内容而非原始 JSON，并覆盖 loading、ready、empty、error。React 只按文本渲染模型内容。

专用路由覆盖学习状态、复习路径、辅导、练习和课堂互动；另外两组成员的路由保持原样。OpenAPI 与 DTO 同步更新。

## Failure Semantics

- 参数非法或内容安全失败：400。
- 未认证：401；角色或技能关闭：403；跨班、他人资源或不存在：404。
- 重复但内容冲突的提交：409；相同提交返回原状态。
- 模型结构无效：502；模型、MCP 或依赖不可达：503。
- 失败不伪造成功、不创建虚假学生回答、不泄露标准答案或密钥。

## Validation Strategy

普通测试使用 Offline/Mock，不访问真实网络。Go 单元测试覆盖规则、状态机、权限、重复提交和 MCP；HTTP 测试覆盖错误路径；固定 AI 评测分别为复习路径、追问辅导和课堂聚类各至少 10 条。最终执行 OpenSpec strict、Go test/vet、Web build、`make verify` 和 Compose 配置校验；环境缺失的项目保持未完成。
