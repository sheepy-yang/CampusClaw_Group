# 依赖与镜像说明

## 代码依赖

- Go API 与 MCP 骨架只使用 Go 标准库。
- Web 依赖由 `web/package-lock.json` 固定，使用 `npm ci` 安装。
- `make verify` 执行 `npm audit --audit-level=high`；发现高危或严重漏洞即失败。

## 容器镜像

- MySQL：`mysql:8.0`
- Redis：`redis:7`
- Qdrant：`qdrant/qdrant:v1.12.1`
- Nginx：`nginx:1.27-alpine`
- Go 构建：`golang:1.22-alpine`
- Node 构建：`node:20.19-alpine`
- 运行基础：`alpine:3.20`

MinIO 已停止在 Docker Hub 发布官方社区镜像。Compose 使用
`ghcr.io/coollabsio/minio:RELEASE.2025-10-15T17-29-55Z`，该固定标签由公开构建流程基于同名上游安全修复版本构建。采用原因是课程架构要求 S3 兼容对象存储，且原 `minio/minio` 地址已经无法拉取。

在生产部署前应完成镜像来源、摘要、AGPL-3.0 义务和课程部署环境白名单复核；若学校镜像源不接受该地址，应基于上游同一 release 源码自行构建并固定摘要，而不是退回存在已知高危问题的旧镜像。
