#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_ROOT="$SCRIPT_DIR"
DEPLOY_SCRIPT="$APP_ROOT/deploy/scripts/deploy_app.sh"
BRANCH="${BRANCH:-main}"

if [[ ! -f "$DEPLOY_SCRIPT" ]]; then
  echo "未找到部署脚本: $DEPLOY_SCRIPT"
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "未找到 git，请先安装 Git。"
  exit 1
fi

cd "$APP_ROOT"

# Prevent pulling over local edits, which causes deploy interruption.
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "检测到本地未提交改动，已停止更新。"
  echo "请先处理改动（提交 / stash）后再执行 ./update。"
  exit 1
fi

git pull --ff-only origin "$BRANCH"

# Keep wrapper ergonomic: one command for both first-time and routine updates.
chmod +x "$DEPLOY_SCRIPT"
APP_ROOT="$APP_ROOT" BRANCH="$BRANCH" SKIP_GIT_PULL=1 "$DEPLOY_SCRIPT" "$@"
