# 架构速览

详见根目录 [README.md](../README.md) 第四节。  
一键部署与环境要求见 [REQUIREMENTS.md](../REQUIREMENTS.md)、[deployment.md](./deployment.md)。

```text
Browser → Nginx → Go API
                ├─ platform / content / tutoring / evaluation
                ├─ agent / gateway / retrieval / mcp host
                └─ Redis Streams → independent Worker
依赖：MySQL · Qdrant · Redis · MinIO · mcp-teaching-tools · LLM Gateway
```

分工模块代码位置：

| 成员 | 包路径 |
| --- | --- |
| 刘恺骐 | `api/internal/content` + `api/internal/agent` + `api/internal/teaching/pipeline.go`（内容生成、验证与虚拟课堂编排） |
| 杨瑾文 | `api/internal/teaching/modules/evaluation` + `api/internal/queue` |
| 刘沛鑫 | `api/internal/tutoring` + `api/internal/httpserver/tutoring_handlers.go` + `mcp-teaching-tools` |
