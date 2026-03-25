# Ubuntu 部署文档

本文档说明如何在 Ubuntu 服务器上，从 Git 拉取项目并完成完整部署。

当前推荐部署原则：

- 项目统一放在 `/opt/projects/`
- PostgreSQL 使用 Docker 部署
- PostgreSQL 只初始化一次
- 后续更多是执行前后端部署
- Go 后端在主机上编译并通过 systemd 常驻
- 前端构建为静态文件，由 Nginx 提供访问并反向代理 `/api`

## 1. 推荐脚本方案

这个项目最适合用两类脚本，而不是一个把所有事情都塞进去的总脚本。

### 一次性脚本

脚本：

- `deploy/scripts/init_postgres_once.sh`

用途：

- 第一次启动 PostgreSQL Docker 容器
- 等待 PostgreSQL ready
- 初始化表结构

为什么单独拆开：

- PostgreSQL 属于基础设施，不是每次发版都要碰的东西
- 数据库初始化和应用发布混在一起，后续容易误操作
- 这个项目后面主要变化在前后端代码，不在数据库容器

### 高频脚本

脚本：

- `deploy/scripts/deploy_app.sh`

用途：

- 拉取最新代码
- 编译 Go 后端
- 构建前端静态资源
- 重启后端 systemd 服务
- 重载 Nginx

为什么这样设计：

- 这才是后续最常用的动作
- 每次更新基本都只跑这一个脚本
- 第一次部署时可以增加 `--first-time`，顺便安装 systemd 和 Nginx 配置

### 最终建议

第一次部署：

1. 安装环境
2. `git clone`
3. 跑一次 `init_postgres_once.sh`
4. 配置后端 `.env`
5. 跑一次 `deploy_app.sh --first-time`

后续更新：

1. `git pull`
2. 跑 `deploy_app.sh`

## 2. 目录约定

统一放在：

```text
/opt/projects/modelprobe
```

部署完成后的主要结构：

```text
/opt/projects/modelprobe/
├─ backend/
│  ├─ .env
│  └─ modelprobe-server
├─ frontend/
│  └─ dist/
└─ deploy/
```

这样做的好处是：

- `/opt/projects/` 适合放多个项目
- 不会和 `/var/www` 的传统静态目录混在一起
- 后续多项目部署时路径更统一

## 3. 前置条件

建议系统版本：

- Ubuntu 22.04 LTS
- Ubuntu 24.04 LTS

服务器至少准备：

- 1 核 CPU
- 2 GB 内存
- 20 GB 磁盘

## 4. 安装基础软件

### 4.1 安装 Git、Docker、Nginx、Node.js

```bash
sudo apt update
sudo apt install -y git curl ca-certificates gnupg lsb-release nginx
```

安装 Docker：

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

安装 Node.js 20：

```bash
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
```

### 4.2 安装 Go 1.25+

```bash
cd /tmp
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
go version
```

如果你使用其他 Go 1.25.x 版本，也可以。

## 5. 拉取项目

```bash
sudo mkdir -p /opt/projects
sudo chown -R $USER:$USER /opt/projects
cd /opt/projects
git clone https://github.com/arthur20230216/testApi.git modelprobe
cd modelprobe
```

后续更新代码：

```bash
cd /opt/projects/modelprobe
git pull origin main
```

## 6. 第一次初始化 PostgreSQL

项目里已经提供：

- `deploy/docker-compose.postgres.yml`
- `deploy/postgres.env.example`
- `deploy/scripts/init_postgres_once.sh`

### 6.1 准备 PostgreSQL 环境文件

```bash
cd /opt/projects/modelprobe
cp deploy/postgres.env.example deploy/postgres.env
```

编辑：

```bash
vim deploy/postgres.env
```

至少修改：

```env
POSTGRES_PASSWORD=your-strong-password
```

### 6.2 执行数据库初始化脚本

```bash
cd /opt/projects/modelprobe
chmod +x deploy/scripts/init_postgres_once.sh
APP_ROOT=/opt/projects/modelprobe ./deploy/scripts/init_postgres_once.sh
```

这个脚本会：

- 启动 PostgreSQL 容器
- 等待数据库 ready
- 初始化表结构

后面一般不需要重复执行。

## 7. 配置后端环境变量

项目里已经提供示例文件：

- `backend/.env.example`

复制并编辑：

```bash
cd /opt/projects/modelprobe/backend
cp .env.example .env
```

编辑 `backend/.env`：

```env
PORT=8080
DATABASE_URL=postgres://modelprobe:your-strong-password@127.0.0.1:5432/modelprobe?sslmode=disable
PROBE_TIMEOUT_MS=10000
ALLOW_ORIGIN=http://your-domain-or-ip
```

如果你通过 Nginx 同域部署前端，`ALLOW_ORIGIN` 可以填你的域名或公网 IP。

## 8. 第一次部署应用

项目里已经提供：

- `deploy/scripts/deploy_app.sh`

第一次执行：

```bash
cd /opt/projects/modelprobe
chmod +x deploy/scripts/deploy_app.sh
APP_ROOT=/opt/projects/modelprobe ./deploy/scripts/deploy_app.sh --first-time
```

这个脚本会完成：

- `git pull origin main`
- 编译 Go 后端
- 构建前端
- 安装 systemd 配置
- 安装 Nginx 配置
- 重启后端
- 重载 Nginx

如果脚本第一次发现 `backend/.env` 不存在，会自动复制模板并中止，等你补完配置再重跑即可。

## 9. 手工拆解版说明

如果你不想直接跑脚本，也可以手工执行。下面是对应步骤。

### 9.1 编译后端

```bash
cd /opt/projects/modelprobe/backend
go mod tidy
go build -o modelprobe-server ./cmd/server
```

### 9.2 手工测试后端

```bash
cd /opt/projects/modelprobe/backend
set -a
source .env
set +a
./modelprobe-server
```

健康检查：

```bash
curl http://127.0.0.1:8080/api/health
```

### 9.3 配置 systemd

```bash
sudo cp /opt/projects/modelprobe/deploy/systemd/modelprobe-backend.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable modelprobe-backend
sudo systemctl start modelprobe-backend
```

查看状态：

```bash
sudo systemctl status modelprobe-backend
journalctl -u modelprobe-backend -f
```

### 9.4 构建前端

```bash
cd /opt/projects/modelprobe/frontend
npm ci
printf "VITE_API_BASE_URL=/api\n" > .env.production
npm run build
```

构建产物在：

```text
/opt/projects/modelprobe/frontend/dist
```

### 9.5 配置 Nginx

```bash
sudo cp /opt/projects/modelprobe/deploy/nginx/modelprobe.conf /etc/nginx/sites-available/modelprobe.conf
sudo ln -sf /etc/nginx/sites-available/modelprobe.conf /etc/nginx/sites-enabled/modelprobe.conf
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
```

## 10. HTTPS

如果你有域名，建议使用 Certbot：

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

## 11. 后续常规更新流程

后续一般不再碰 PostgreSQL。

推荐直接执行：

```bash
cd /opt/projects/modelprobe
APP_ROOT=/opt/projects/modelprobe ./deploy/scripts/deploy_app.sh
```

这就是之后最常用的部署动作。

## 12. 手工更新部署流程

如果你不想用脚本，也可以手工更新：

```bash
cd /opt/projects/modelprobe
git pull origin main
```

重建后端：

```bash
cd /opt/projects/modelprobe/backend
go mod tidy
go build -o modelprobe-server ./cmd/server
sudo systemctl restart modelprobe-backend
```

重建前端：

```bash
cd /opt/projects/modelprobe/frontend
npm ci
npm run build
sudo systemctl reload nginx
```

如果更新涉及数据库结构，需要额外执行新的 SQL 或 migration。

## 13. 常用检查命令

检查 PostgreSQL：

```bash
docker ps
docker logs -f modelprobe-postgres
```

检查后端：

```bash
curl http://127.0.0.1:8080/api/health
journalctl -u modelprobe-backend -f
```

检查前端：

```bash
ls -lah /opt/projects/modelprobe/frontend/dist
sudo nginx -t
```

## 14. 建议的上线顺序

1. 安装基础环境
2. `git clone` 项目到 `/opt/projects/modelprobe`
3. 准备 `deploy/postgres.env`
4. 执行一次 PostgreSQL 初始化脚本
5. 准备 `backend/.env`
6. 执行一次 `deploy_app.sh --first-time`
7. 接入 HTTPS

## 15. 当前部署边界

当前部署方案是简单、稳定、易排障的版本：

- PostgreSQL 用 Docker
- 前后端不容器化
- 后端 systemd 常驻
- 前端静态文件交给 Nginx

这个方案很适合一台 Ubuntu 服务器快速上线。如果后面要做更完整的 CI/CD、灰度发布或多机部署，再考虑补更完整的容器化方案。
