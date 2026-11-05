# Design: Evidence-bound content production and virtual classroom

## Architecture

HTTP Handler 只从服务端会话取得教师、班级和请求标识。Content Service 读取本班不可变讲义切片，调用专业内容智能体，再由独立质量校验器检查 Schema、题型、时长、答案和逐字引用。Repository 将完整证据快照、质量报告、模型元数据、幂等信息和内容状态写入共享持久存储。

虚拟课堂入口只接受已确认或已发布的本班教案，先持久化任务和报告壳，再投递 Redis Streams。Worker 加载冻结教案，调用虚拟学生智能体，校验对话存在后保存合成角色、角色边界、风险时间线、建议及模型版本。任务失败保留输入并允许安全重试。

## Trust Boundaries

- 模型不能决定用户、班级、权限、资源所有者、发布状态或幂等键。
- 引用的来源、版本、切片、字符范围、摘要和哈希必须与服务端证据完全一致。
- 模型输出未通过结构、引用、内容安全或业务规则校验时返回明确失败，不保存为合格内容。
- 虚拟角色和报告必须明确标注为教学演练；结果只用于修改教案。
- 学生只能读取已发布内容，且命题答案和解析在正式提交前不返回。

## State and Failure

内容状态为 `pending_confirmation -> confirmed -> published`；发布后冻结。虚拟演练使用 `queued -> running -> pending -> confirmed -> published`，任务通过 `202 + taskId` 轮询。幂等键相同且负载相同返回原结果，负载不同返回 409。模型、队列或持久化不可用时保留已落库输入并返回可观察错误。
