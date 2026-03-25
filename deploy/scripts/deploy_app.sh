#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APP_ROOT:-/opt/modelprobe}"
BRANCH="${BRANCH:-main}"
FIRST_TIME=0

for arg in "$@"; do
  case "$arg" in
    --first-time)
      FIRST_TIME=1
      ;;
    *)
      echo "未知参数: $arg"
      echo "用法: $0 [--first-time]"
      exit 1
      ;;
  esac
done

if [[ ! -d "$APP_ROOT/.git" ]]; then
  echo "未找到 git 仓库: $APP_ROOT"
  echo "请先执行 git clone。"
  exit 1
fi

cd "$APP_ROOT"
git pull origin "$BRANCH"

if [[ ! -f "$APP_ROOT/backend/.env" ]]; then
  cp "$APP_ROOT/backend/.env.example" "$APP_ROOT/backend/.env"
  echo "已生成 backend/.env，请先修改 DATABASE_URL 和 ALLOW_ORIGIN 后重新执行。"
  exit 1
fi

pushd "$APP_ROOT/backend" >/dev/null
go mod tidy
go build -o modelprobe-server ./cmd/server
popd >/dev/null

pushd "$APP_ROOT/frontend" >/dev/null
npm ci
printf "VITE_API_BASE_URL=/api\n" > .env.production
npm run build
popd >/dev/null

if [[ "$FIRST_TIME" -eq 1 ]]; then
  sudo cp "$APP_ROOT/deploy/systemd/modelprobe-backend.service" /etc/systemd/system/
  sudo cp "$APP_ROOT/deploy/nginx/modelprobe.conf" /etc/nginx/sites-available/modelprobe.conf
  sudo ln -sf /etc/nginx/sites-available/modelprobe.conf /etc/nginx/sites-enabled/modelprobe.conf
  sudo rm -f /etc/nginx/sites-enabled/default
  sudo systemctl daemon-reload
  sudo systemctl enable modelprobe-backend
fi

sudo systemctl restart modelprobe-backend
sudo nginx -t
sudo systemctl reload nginx

echo "应用部署完成。"
