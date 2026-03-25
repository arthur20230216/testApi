# Backend

Go 后端，负责：

- 接收探测请求
- 请求第三方中转站模型列表接口
- 解析模型家族与兼容性
- 生成可信度评分
- 持久化探测记录
- 输出站点与分组红黑榜

## 运行

```bash
cd backend
go mod tidy
go run ./cmd/server
```

默认端口 `8080`。

当前数据库使用 PostgreSQL。

## 初始化数据库

先创建数据库：

```bash
psql -U postgres -h 127.0.0.1 -f backend/scripts/create_database.sql
```

再初始化表结构：

```bash
psql -U postgres -h 127.0.0.1 -d modelprobe -f backend/scripts/init_postgres.sql
```

也可以手动创建 `modelprobe` 数据库，然后再执行 `init_postgres.sql`。

## 环境变量

- `PORT`
- `DATABASE_URL`
- `PROBE_TIMEOUT_MS`
- `ALLOW_ORIGIN`
