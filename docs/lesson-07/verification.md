# 第 7 课项目骨架验证记录

对应 change：`2026-10-28-bootstrap-project-skeleton`

本记录只接受命令、HTTP 响应、日志和另一名成员的实际运行结果；没有执行的项目保持未勾选，不以说明文字代替证据。

## 一、自动检查

- [x] OpenSpec strict 校验通过
- [x] API Go 测试通过
- [x] MCP Go 测试通过
- [x] 前端 lockfile 安装、TypeScript 与 Vite 构建通过
- [x] 前端高危依赖审计通过
- [x] `docker compose config --quiet` 通过

统一命令：

```bash
make verify
```

实际结果：

```text
make verify 退出码为 0：
- OpenSpec 1.15.0：Change '2026-10-28-bootstrap-project-skeleton' is valid
- API：go test ./... 与 go vet ./... 通过
- MCP：go test ./... 与 go vet ./... 通过
- Web：Vite 8.3.5 生产构建成功，31 个 npm 包审计为 0 个漏洞
- Compose：docker compose config --quiet 通过
```

## 二、运行验收

- [x] API、MCP、Worker 和 Web 镜像实际构建通过
- [x] 九个服务实际启动

启动命令：

```bash
docker compose up -d --build
```

### 1. 容器状态

```bash
docker compose ps
```

实际结果：

```text
九个服务均启动：
api healthy；mcp healthy；mysql healthy；redis healthy；
qdrant、minio、web、worker、nginx 均为 Up。
仅 nginx 发布宿主机 18088 端口，其余服务只暴露在 Compose 网络。
```

### 2. 健康检查

```bash
curl -i http://127.0.0.1:${APP_PORT}/health
```

通过标准：

- HTTP 状态为 200；
- 总状态为 `up`；
- MySQL、Redis、Qdrant、MinIO 和 MCP 均为 `up`；
- 响应不含口令、Token、DSN 或内部堆栈。

实际结果：

```text
HTTP/1.1 200 OK
{"status":"up","checks":{"mcp":{"status":"up"},"minio":{"status":"up"},"mysql":{"status":"up"},"qdrant":{"status":"up"},"redis":{"status":"up"}}}

额外失败路径：停止 Qdrant 后返回 HTTP 503，只有 qdrant=down；
重新启动 Qdrant 后恢复 HTTP 200 和全部 up。
```

### 3. 请求标识与 JSON 日志

```bash
curl -i -X POST -H 'X-Request-ID: lesson-07-manual-check' \
  http://127.0.0.1:${APP_PORT}/api/v1/skills/proposition
docker compose logs --no-log-prefix api
```

通过标准：

- 占位业务入口返回 501 和 `NOT_IMPLEMENTED`；
- 响应头与错误体包含 `lesson-07-manual-check`；
- API 日志一行一条 JSON，并含相同 `request_id`；
- 日志不含 Cookie、Authorization、API Key、MCP Token 或数据库口令。

实际结果：

```text
HTTP/1.1 501 Not Implemented
X-Request-ID: lesson-07-manual-check
{"code":"NOT_IMPLEMENTED","message":"teaching capability is declared but not implemented in the skeleton","request_id":"lesson-07-manual-check"}

API 脱敏日志：
{"level":"INFO","service":"api","environment":"test","event":"http_request","request_id":"lesson-07-manual-check","method":"POST","path":"/api/v1/skills/proposition","status":501,"duration_ms":0}

Nginx 访问日志同样为单行 JSON，并携带相同 request_id。
```

## 三、配置失败路径

从本机 `.env` 临时移除一个必填键，再执行：

```bash
docker compose config
```

通过标准：命令以非零状态失败，只指出缺失变量名，不输出其他配置值。

实际结果：

```text
已在不提供 SESSION_SECRET 的环境中执行 docker compose config --quiet。
命令按预期失败，仅报告：
required variable SESSION_SECRET is missing a value: SESSION_SECRET is required

另以空环境启动 API，进程在监听端口前退出，JSON 日志字段为：
{"time":"2026-10-29T00:14:18.388809+08:00","level":"ERROR","msg":"api stopped","service":"api","event":"startup_failed","code":"CONFIG_INVALID","error":"configuration invalid: APP_ENV is required\nHTTP_PORT is required\nSHUTDOWN_TIMEOUT is required\nHEALTH_TIMEOUT is required\nMYSQL_ADDR is required\nREDIS_ADDR is required\nQDRANT_URL is required\nMINIO_URL is required\nMCP_URL is required\nSESSION_SECRET is required"}
```

## 四、同伴从零复核

- 复核成员：
- 操作系统与 Docker 版本：
- 是否只依据 README：
- `make verify` 结果：
- 一条命令启动结果：
- `/health` 结果：
- 发现的问题及修正：

该部分必须由非本次修改作者的组员在干净目录实际完成，不得预填“通过”。

## 五、需提交人亲自完成

- [ ] 将远程仓库改为仅含小写字母和连字符的课题短名
- [ ] 确认所有成员已有仓库权限
- [ ] 设置主干保护和必要检查
- [ ] 检查完整历史中没有真实密钥
- [ ] 由组员交叉评审本 change
- [ ] 提交修改并创建 `v0.0.1-skeleton` 标签

以上均涉及 Git 或 GitHub，Agent 不代替提交人执行。
