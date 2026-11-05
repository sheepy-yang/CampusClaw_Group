---
name: verify-project-skeleton
description: Verifies the Lesson 7 CampusClaw project skeleton against OpenSpec, configuration, health, logging, Compose, documentation, and peer-review evidence. Use before submitting or tagging a skeleton release.
disable-model-invocation: true
---

# 验证项目骨架

## 执行顺序

1. 读取 `AGENTS.md`、`.cursor/rules/` 和活动 OpenSpec change。
2. 确认本次只验收框架，不要求十二项业务实现。
3. 运行 `make verify`，任一步失败即停止并记录原始错误。
4. 复制 `.env.example` 为本机 `.env`，填写本地值，不展示或提交密钥。
5. 运行 `docker compose up -d --build`。
6. 请求 `GET /health`，确认 HTTP 200 且每项依赖均为 `up`。
7. 请求一个不存在的业务入口，确认统一错误包含 `code`、`message` 和 `request_id`。
8. 检查服务日志：一行一条 JSON，包含 `service`、`event` 和 `request_id`，且无密钥。
9. 将命令、预期和实际结果写入 `docs/lesson-07/verification.md`。
10. 请另一名成员在干净机器上仅按 README 从零启动，并记录姓名、环境和结果。

## 输出

按“通过、失败、需人工确认”三组列出证据。主干保护、仓库改名、标签和 Git 历史只列人工检查项；不要运行 Git 或 GitHub 命令。
