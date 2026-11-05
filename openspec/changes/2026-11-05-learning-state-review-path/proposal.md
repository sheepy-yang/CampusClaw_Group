# Proposal: 2026-11-05-learning-state-review-path

## Why

仓库已具备个性化辅导相关的数据结构、Agent/Gateway 契约、内存 Store、部分状态机和前端入口，但四项技能仍未形成真实可运行的主路径：公开路由仍返回 501，模型结果和 MCP 返回缺少完整校验，练习由客户端提交判定结果，课堂聚类会自动制造演示回答，MCP `tools/call` 尚未实现。

本 Change 在现有工程上完成「个性化学习与课堂互动 V1」，复用当前 Go 1.22 `net/http`、React + TypeScript、Memory Store、Agent/Orchestrator、认证、审计与安全工具，不重建技术栈，也不修改其他成员负责的业务逻辑。

## What Changes

- 建立共享 Learning State：汇总当前学生、当前班级最近 14 天的学习事件和错题，使用 `weak`、`developing`、`solid`、`insufficient_data` 四级可解释状态。
- 打通个性化复习路径：无证据跳过，有证据时生成并校验 3～5 步计划，支持查询和幂等完成步骤。
- 打通追问式辅导：会话绑定学生与班级，模型生成追问，后端确定性规则控制答案释放。
- 打通自适应练习：服务端判题，三档难度调整，真实前置知识回退，最多 10 题并生成总结。
- 打通课堂互动：教师创建与结束收集，学生每人一份有效回答，至少 5 人后匿名聚类，最多 4 类。
- 完成 MCP Teaching Tools 的初始化、发现、Schema 校验、白名单调用、权限和审计；实现三个现有工具。
- 将四项能力接入独立 tutoring Service/Handler、现有学生与教师页面、OpenAPI、测试和 AI 评测。

## Delivery Boundary

V1 使用现有 Memory Store，进程重启后业务数据会丢失；不新增 MySQL 迁移。规则算法和离线模型仅用于可解释演示，必须明确标注，不得描述为生产级模型能力。MCP Server 不与 API 共享内存：需要业务数据的工具由 API 先完成授权和范围查询，再把最小化数据作为工具参数传入。

骨架 Change 的 6.1 仓库管理、6.2 非作者干净环境复核、6.3 提交与标签仍保持未完成，是独立课程交付待办；本 Change 不修改其状态，也不虚构证据或例外记录。它们不阻塞本地 MVP 实现。

## Out of Scope

- MySQL 业务持久化、复杂 Repository Adapter 体系和生产级分布式幂等。
- 强化学习、复杂知识图谱、Embedding + K-means 和实时课堂通信。
- 多 Agent 协作框架、模型微调和生产级数学符号系统。
- 其他成员负责的内容生产、作业评价与班级分析业务。
- Git/GitHub 操作、远程仓库设置和课程最终发布动作。
