# 架构速览（骨架）

详见根目录 [README.md](../README.md) 第四节。

```text
Browser → Nginx → Go API
                ├─ platform / teaching / agent / gateway / mcpclient
                └─ Redis Worker
依赖：MySQL · Qdrant · Redis · MinIO · mcp-teaching-tools · LLM Gateway
```

分工模块代码位置：

| 成员 | 包路径 |
| --- | --- |
| 刘恺骐 | `api/internal/teaching/modules/content` + `api/internal/agent`（编排/内容/验证） |
| 杨瑾文 | `api/internal/teaching/modules/evaluation` + `api/internal/queue` |
| 刘沛鑫 | `api/internal/teaching/modules/tutoring` + `mcp-teaching-tools` |
