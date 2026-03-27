#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_ROOT="$SCRIPT_DIR"
DEPLOY_SCRIPT="$APP_ROOT/deploy/scripts/deploy_app.sh"

if [[ ! -f "$DEPLOY_SCRIPT" ]]; then
  echo "未找到部署脚本: $DEPLOY_SCRIPT"
  exit 1
fi

# Keep wrapper ergonomic: one command for both first-time and routine updates.
chmod +x "$DEPLOY_SCRIPT"
APP_ROOT="$APP_ROOT" "$DEPLOY_SCRIPT" "$@"
