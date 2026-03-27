#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APP_ROOT:-/opt/modelprobe}"
BRANCH="${BRANCH:-main}"
FIRST_TIME=0
SKIP_GIT_PULL="${SKIP_GIT_PULL:-0}"
SUDO="sudo"

if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  SUDO=""
fi

require_cmd() {
  local cmd="$1"
  local tip="${2:-}"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "缺少命令: $cmd"
    if [[ -n "$tip" ]]; then
      echo "$tip"
    fi
    exit 1
  fi
}

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

require_cmd git "请先安装 Git。"
require_cmd go "未找到 go。请先安装 Go 1.25+，并确保 /usr/local/go/bin 在 PATH 中（或链接 /usr/local/bin/go）。"
require_cmd npm "未找到 npm。请先安装 Node.js 20+。"
require_cmd nginx "未找到 nginx。请先安装 Nginx。"
require_cmd systemctl "未找到 systemctl。请在支持 systemd 的 Ubuntu 服务器执行该脚本。"

if [[ -n "$SUDO" ]]; then
  require_cmd sudo "未找到 sudo。请安装 sudo 或使用 root 用户执行。"
fi

cd "$APP_ROOT"
if [[ "$SKIP_GIT_PULL" != "1" ]]; then
  git pull origin "$BRANCH"
fi

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
  $SUDO cp "$APP_ROOT/deploy/systemd/modelprobe-backend.service" /etc/systemd/system/
  $SUDO cp "$APP_ROOT/deploy/nginx/modelprobe.conf" /etc/nginx/sites-available/modelprobe.conf
  $SUDO ln -sf /etc/nginx/sites-available/modelprobe.conf /etc/nginx/sites-enabled/modelprobe.conf
  $SUDO rm -f /etc/nginx/sites-enabled/default
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable modelprobe-backend
fi

$SUDO systemctl restart modelprobe-backend
$SUDO nginx -t
$SUDO systemctl reload nginx

echo "应用部署完成。"
