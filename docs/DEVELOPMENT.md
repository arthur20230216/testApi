# 中转站真伪检测平台开发文档

## 1. 当前技术方向

项目已按新的要求重构为前后端分离：

- 前端单独放在 `frontend/`
- 后端单独放在 `backend/`
- 后端由原来的 Node/TypeScript 改为 Go
- 目录分层参考 `Wei-Shaw/sub2api` 的思路，采用 `cmd + internal` 结构

当前目标不是复制 `sub2api` 的全部业务，而是参考其工程组织方式，承载本项目自己的“探测、存证、排行”能力。

## 2. 当前目录结构

```text
testAPI/
├─ backend/
│  ├─ cmd/
│  │  └─ server/
│  ├─ data/
│  ├─ internal/
│  │  ├─ config/
│  │  ├─ handler/
│  │  ├─ model/
│  │  ├─ repository/
│  │  ├─ server/
│  │  └─ service/
│  ├─ go.mod
│  └─ README.md
├─ frontend/
│  ├─ src/
│  ├─ package.json
│  └─ .env.example
├─ docs/
│  ├─ DEVELOPMENT.md
│  └─ PRODUCT.md
└─ README.md
```

## 3. 参考 `sub2api` 的点

本项目对 `sub2api` 的参考主要体现在工程结构，而不是照搬业务模型。

### 3.1 目录组织

参考点：

- 后端入口放到 `backend/cmd/server`
- 业务代码放到 `backend/internal`
- 按 `handler / service / repository / model / server / config` 分层
- 前端作为独立项目放在 `frontend/`

### 3.2 技术路线

参考点：

- Go 作为后端主语言
- 单独前端工程
- 前后端通过 HTTP API 解耦

### 3.3 当前没有照搬的部分

当前没有引入：

- Ent ORM
- Redis
- `sub2api` 的账户、分组、订阅和管理后台模型

原因是这个项目的核心矛盾不是复杂运营后台，而是先把“检测、解释、排行”的业务闭环跑稳。

## 4. 后端技术栈

当前后端使用：

- Go 1.25
- Gin
- PostgreSQL
- `database/sql`
- `github.com/jackc/pgx/v5`

## 5. 前端技术栈

当前前端使用：

- Vite
- React
- TypeScript

说明：

- 这里没有跟着 `sub2api` 用 Vue，而是先保留更快起量的 React Vite 组合
- 对这个项目来说，前端框架不是关键约束，关键约束是前后端必须解耦

## 6. 后端分层职责

### `backend/cmd/server`

- 程序入口
- 加载配置
- 初始化 repository 和 service
- 启动 HTTP 服务

### `backend/internal/config`

- 读取环境变量
- 输出统一配置对象

### `backend/internal/model`

- 定义请求、响应和排行的数据结构
- 放置基础校验逻辑

### `backend/internal/handler`

- 接收 HTTP 请求
- 解析参数
- 调用 service 和 repository
- 返回统一 JSON 响应

### `backend/internal/service`

- 生成候选探测 endpoint
- 请求模型列表
- 解析响应
- 检测模型家族
- 判断兼容性
- 生成评分、结论和证据

### `backend/internal/repository`

- 初始化 PostgreSQL
- 创建表和索引
- 保存探测结果
- 查询单次探测
- 查询最近探测
- 聚合红黑榜

### `backend/internal/server`

- 组装 Gin Router
- 注册中间件
- 注册 API 路由

## 7. 当前请求流程

以 `POST /api/probes` 为例：

1. 前端或调用方提交 `stationName + groupName + baseUrl + apiKey + claimedChannel + expectedModelFamily`
2. `handler` 负责绑定 JSON 和校验参数
3. `service` 负责探测第三方站点
4. `service` 生成 `trustScore / verdict / suspicionReasons / notes`
5. `repository` 将完整结果写入 PostgreSQL
6. `handler` 返回结构化 JSON 给前端
7. 榜单接口直接从数据库聚合

## 8. 当前数据库设计

当前数据库默认在：

通过 `DATABASE_URL` 连接 PostgreSQL。

当前表：

- `probes`

字段仍然围绕探测结果设计，核心包括：

- 站点名
- 分组名
- 中转地址
- API Key 哈希和掩码
- 探测状态
- 可信度分数
- 总结结论
- HTTP 状态
- 命中的 endpoint
- 响应耗时
- 模型家族
- 模型 ID
- 可疑原因
- 正向证据
- 原始节选

## 9. 当前评分逻辑

评分逻辑仍然采用启发式规则，后端实现在 `backend/internal/service/probe_service.go`。

主要信号：

- `/models` 是否返回 2xx
- 响应体是否是 JSON
- 是否能提取模型 ID
- 是否符合 OpenAI 标准模型列表结构
- 是否识别出模型家族
- 站点宣称和检测结果是否一致
- 用户期望家族和检测结果是否一致

当前结论分层：

- `trusted`
- `needs_review`
- `high_risk`

## 10. 前端职责

当前前端是单页控制台，负责：

- 提交探测表单
- 展示单次检测结果
- 展示站点红黑榜
- 展示分组红黑榜
- 展示最近探测

当前前端通过 `VITE_API_BASE_URL` 调用后端 API。

## 11. 运行方式

### 后端

```bash
cd backend
go mod tidy
go run ./cmd/server
```

### 前端

```bash
cd frontend
npm install
npm run dev
```

初始化脚本位于：

```text
backend/scripts/create_database.sql
backend/scripts/init_postgres.sql
```

## 12. 当前实现的优点

- 工程结构比之前清楚得多
- 后端和前端已经彻底解耦
- 后端从 Node 切到 Go，后续更适合做并发探测和持续扩展
- 目录结构更接近成熟项目，不会继续演化成根目录堆文件

## 13. 当前实现的不足

### 13.1 还没有任务队列

现在仍然是同步探测。第三方站点慢时，接口响应会跟着慢。

### 13.2 还没有更强的指纹识别

当前主要靠 `/models` 和关键词规则，离“更准地识别假冒和套壳”还有距离。

### 13.3 前端还是单页 MVP

现在更像操作台，还不是完整的信息站点。

### 13.4 当前还没有 migration 管理器

现在是初始化 SQL 和服务端自动建表并存，足够先跑通，但后续最好补正式 migration 体系。

## 14. 后续开发建议

### 后端优先事项

1. SSRF 防护和 URL 白名单策略
2. 异步任务队列
3. 定时复检
4. 更细的协议指纹
5. 单元测试和集成测试

### 前端优先事项

1. 单站点详情页
2. 单分组详情页
3. 搜索和筛选
4. 结果分享页
5. 风险解释模板优化

### 数据层演进

现在已经切到 PostgreSQL。下一步建议补：

- migration 管理
- 更细的索引策略
- 排行聚合表
- 定时清理和归档策略

## 15. 一句话总结

现在的结构已经从“单个 Node 后端原型”升级为“前后端分离、Go 后端主导、参考成熟项目分层”的可继续迭代版本。后面真正要拉开差距的，不是再换框架，而是把识别能力、异步任务和样本体系做深。
