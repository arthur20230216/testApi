# Ubuntu 部署文档

本文档说明如何在 Ubuntu 服务器上从 Git 拉取项目并完成完整部署。

当前部署方式约定：

- 代码从 GitHub 拉取
- PostgreSQL 使用 Docker 部署
- Go 后端在主机上编译并通过 systemd 常驻
- 前端使用 Vite 构建静态文件，由 Nginx 提供访问并反向代理 `/api`

## 1. 部署目标结构

部署完成后，服务器上的主要结构如下：

```text
/var/www/modelprobe/
├─ backend/
│  ├─ .env
│  └─ modelprobe-server
├─ frontend/
│  └─ dist/
└─ deploy/
```

## 2. 前置条件

建议系统版本：

- Ubuntu 22.04 LTS
- Ubuntu 24.04 LTS

服务器至少准备：

- 1 核 CPU
- 2 GB 内存
- 20 GB 磁盘

## 3. 安装基础软件

### 3.1 安装 Git、Docker、Nginx、Node.js

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

### 3.2 安装 Go 1.25+

推荐使用官方二进制包。下面以 `/usr/local/go` 为安装位置：

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

## 4. 拉取项目

```bash
sudo mkdir -p /var/www
sudo chown -R $USER:$USER /var/www
cd /var/www
git clone https://github.com/arthur20230216/testApi.git modelprobe
cd modelprobe
```

后续更新代码：

```bash
cd /var/www/modelprobe
git pull origin main
```

如果默认分支不是 `main`，把命令里的分支名替换成实际分支。

## 5. 启动 PostgreSQL 容器

项目中已经提供了 Docker Compose 文件：

- `deploy/docker-compose.postgres.yml`

先修改默认密码：

```bash
cd /var/www/modelprobe
sed -i 's/change-me/your-strong-password/g' deploy/docker-compose.postgres.yml
```

启动 PostgreSQL：

```bash
docker compose -f deploy/docker-compose.postgres.yml up -d
```

确认数据库已启动：

```bash
docker ps
```

## 6. 初始化数据库

先创建数据库：

```bash
docker exec -i modelprobe-postgres psql -U modelprobe -d postgres < backend/scripts/create_database.sql
```

如果 `modelprobe` 数据库已经由容器环境变量自动创建，这一步可以跳过。

再初始化表结构：

```bash
docker exec -i modelprobe-postgres psql -U modelprobe -d modelprobe < backend/scripts/init_postgres.sql
```

## 7. 配置后端环境变量

项目里已经提供示例文件：

- `backend/.env.example`

复制并编辑：

```bash
cd /var/www/modelprobe/backend
cp .env.example .env
```

编辑 `backend/.env`：

```env
PORT=8080
DATABASE_URL=postgres://modelprobe:your-strong-password@127.0.0.1:5432/modelprobe?sslmode=disable
PROBE_TIMEOUT_MS=10000
ALLOW_ORIGIN=http://your-domain-or-ip
```

如果你通过 Nginx 同域部署前端，`ALLOW_ORIGIN` 可以直接填你的域名或公网 IP。

## 8. 构建并启动后端

编译后端：

```bash
cd /var/www/modelprobe/backend
go mod tidy
go build -o modelprobe-server ./cmd/server
```

先手工启动测试：

```bash
cd /var/www/modelprobe/backend
set -a
source .env
set +a
./modelprobe-server
```

确认健康检查正常：

```bash
curl http://127.0.0.1:8080/api/health
```

## 9. 配置 systemd 托管后端

项目里已经提供 service 模板：

- `deploy/systemd/modelprobe-backend.service`

复制到 systemd：

```bash
sudo cp /var/www/modelprobe/deploy/systemd/modelprobe-backend.service /etc/systemd/system/
```

启用并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable modelprobe-backend
sudo systemctl start modelprobe-backend
```

查看状态：

```bash
sudo systemctl status modelprobe-backend
```

查看日志：

```bash
journalctl -u modelprobe-backend -f
```

## 10. 构建前端

```bash
cd /var/www/modelprobe/frontend
cp .env.example .env.production
```

编辑 `frontend/.env.production`：

```env
VITE_API_BASE_URL=/api
```

然后构建：

```bash
npm install
npm run build
```

构建产物在：

```text
/var/www/modelprobe/frontend/dist
```

## 11. 配置 Nginx

项目里已经提供 Nginx 模板：

- `deploy/nginx/modelprobe.conf`

复制到 Nginx：

```bash
sudo cp /var/www/modelprobe/deploy/nginx/modelprobe.conf /etc/nginx/sites-available/modelprobe.conf
sudo ln -sf /etc/nginx/sites-available/modelprobe.conf /etc/nginx/sites-enabled/modelprobe.conf
sudo rm -f /etc/nginx/sites-enabled/default
```

如果你有域名，把 `server_name _;` 改成你的域名。

检查并重载：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

## 12. HTTPS

如果你有域名，建议用 Certbot：

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

## 13. 更新部署流程

以后每次从 Git 更新：

```bash
cd /var/www/modelprobe
git pull origin main
```

重新构建后端：

```bash
cd /var/www/modelprobe/backend
go mod tidy
go build -o modelprobe-server ./cmd/server
sudo systemctl restart modelprobe-backend
```

重新构建前端：

```bash
cd /var/www/modelprobe/frontend
npm install
npm run build
sudo systemctl reload nginx
```

如果更新涉及数据库结构，需要额外执行新的 SQL 或 migration。

## 14. 常用检查命令

检查 PostgreSQL 容器：

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
ls -lah /var/www/modelprobe/frontend/dist
sudo nginx -t
```

## 15. 建议的上线顺序

1. 装基础环境
2. `git clone` 项目
3. 启动 PostgreSQL 容器
4. 初始化数据库
5. 配置并编译后端
6. systemd 托管后端
7. 构建前端
8. 配置 Nginx
9. 接入 HTTPS

## 16. 当前部署边界

当前部署方案是简单、稳定、易排障的版本：

- PostgreSQL 用 Docker
- 前后端不容器化
- 后端 systemd 常驻
- 前端静态文件交给 Nginx

这个方案很适合一台 Ubuntu 服务器快速上线。如果后面要做更完整的 CI/CD、灰度发布或多机部署，再考虑补完整的 Docker Compose 或 Kubernetes。
