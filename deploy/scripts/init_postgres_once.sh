#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APP_ROOT:-/opt/projects/modelprobe}"
COMPOSE_FILE="$APP_ROOT/deploy/docker-compose.postgres.yml"
POSTGRES_ENV="$APP_ROOT/deploy/postgres.env"
POSTGRES_ENV_EXAMPLE="$APP_ROOT/deploy/postgres.env.example"
INIT_SQL="$APP_ROOT/backend/scripts/init_postgres.sql"

if [[ ! -d "$APP_ROOT" ]]; then
  echo "APP_ROOT 不存在: $APP_ROOT"
  echo "请先完成 git clone。"
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "未找到 docker，请先安装 Docker。"
  exit 1
fi

if [[ ! -f "$POSTGRES_ENV" ]]; then
  cp "$POSTGRES_ENV_EXAMPLE" "$POSTGRES_ENV"
  echo "已生成 $POSTGRES_ENV"
  echo "请先修改其中的 POSTGRES_PASSWORD，再重新执行本脚本。"
  exit 1
fi

cd "$APP_ROOT"
docker compose -f "$COMPOSE_FILE" up -d

POSTGRES_DB="$(grep '^POSTGRES_DB=' "$POSTGRES_ENV" | cut -d '=' -f2-)"
POSTGRES_USER="$(grep '^POSTGRES_USER=' "$POSTGRES_ENV" | cut -d '=' -f2-)"

echo "等待 PostgreSQL 就绪..."
until docker exec modelprobe-postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; do
  sleep 2
done

docker exec -i modelprobe-postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < "$INIT_SQL"

echo "PostgreSQL 已启动并完成表结构初始化。"
