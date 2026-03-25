# Frontend

前端用于：

- 提交中转站探测请求
- 展示单次检测结果
- 展示站点红黑榜
- 展示分组红黑榜
- 展示最近探测记录

## 运行

```bash
cd frontend
npm install
npm run dev
```

## 环境变量

复制 `.env.example` 后配置：

```bash
VITE_API_BASE_URL=http://localhost:8080
```

## 构建

```bash
npm run build
```
