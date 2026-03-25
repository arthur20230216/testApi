# Model Probe

一个面向 AI 编程中转站的真伪检测项目。当前已重构为前后端分离结构：

- `backend/`
  Go 后端，参考 `sub2api` 的目录思路，按 `cmd` + `internal/{handler,service,repository,model,server}` 分层
- `frontend/`
  独立 Vite 前端，用于提交探测、查看结果和浏览红黑榜

核心目标：

- 输入第三方公开的 `baseUrl + apiKey`
- 探测其 `/v1/models` 或 `/models`
- 分析模型家族与 OpenAI 兼容性
- 输出可信度评分和可疑原因
- 对中转站和分组形成红黑榜
- 使用 PostgreSQL 保存探测记录和排行样本

## 项目结构

```text
.
├─ backend/
│  ├─ cmd/server
│  ├─ internal/config
│  ├─ internal/handler
│  ├─ internal/model
│  ├─ internal/repository
│  ├─ internal/server
│  └─ internal/service
├─ frontend/
│  └─ src/
└─ docs/
```

## 运行

后端：

```bash
cd backend
go mod tidy
go run ./cmd/server
```

前端：

```bash
cd frontend
npm install
npm run dev
```

默认端口：

- 前端：`5173`
- 后端：`8080`

前端通过 `VITE_API_BASE_URL` 指向后端，示例见 [frontend/.env.example](./frontend/.env.example)。

## 数据库

当前后端使用 PostgreSQL。

默认连接串通过 `DATABASE_URL` 注入，示例见 [backend/.env.example](./backend/.env.example)。初始化脚本在 [create_database.sql](./backend/scripts/create_database.sql) 和 [init_postgres.sql](./backend/scripts/init_postgres.sql)。

## API

- `GET /api/health`
- `POST /api/probes`
- `GET /api/probes`
- `GET /api/probes/:id`
- `GET /api/rankings/stations`
- `GET /api/rankings/groups`

## 当前探测边界

- 只做只读元数据探测，优先请求 `GET /v1/models`
- 不保存明文 `apiKey`，只保存哈希和掩码
- 评分是启发式技术判断，不代表官方认证

## 文档

- [产品文档](./docs/PRODUCT.md)
- [开发文档](./docs/DEVELOPMENT.md)
- [Ubuntu 部署文档](./docs/DEPLOY_UBUNTU.md)
