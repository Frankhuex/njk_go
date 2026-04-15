#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$ROOT_DIR/../NJK/.env"
SQL_FILE="$ROOT_DIR/sql/create_njk_tables.sql"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

DB_NAME="${DB_NAME:-njk}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-njk}"
DB_PWD="${DB_PWD:-}"

if [[ "$DB_USER" == "postgres" ]]; then
  DB_USER="njk"
fi

if [[ "$DB_NAME" != "njk" ]]; then
  echo "[ERROR] 该脚本只允许连接 njk 数据库，当前 DB_NAME=$DB_NAME"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "[ERROR] 未找到 psql，请先安装 PostgreSQL 客户端。"
  exit 1
fi

echo "[INFO] 将在数据库 $DB_NAME 中建表"
echo "[INFO] host=$DB_HOST port=$DB_PORT user=$DB_USER"

PGPASSWORD="$DB_PWD" psql \
  -v ON_ERROR_STOP=1 \
  -h "$DB_HOST" \
  -p "$DB_PORT" \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  -f "$SQL_FILE"

echo "[INFO] 建表脚本执行完成"
