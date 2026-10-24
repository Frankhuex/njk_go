#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for arg in "$@"; do
  case "$arg" in --check|--install|--help|-h)
    exec python3 "$ROOT_DIR/scripts/server/deploy_server.py" --root "$ROOT_DIR" "$@" ;;
  esac
done
# 发布任务交给用户级 systemd，SSH 中断后继续完成；输出写入 journal。
UNIT="njk-deploy-$(date +%s%N)-$$"
if systemd-run --user --quiet --wait --collect --unit "$UNIT" \
    python3 "$ROOT_DIR/scripts/server/deploy_server.py" --root "$ROOT_DIR" "$@"; then
  STATUS=0
else
  STATUS=$?
fi
journalctl --user -u "$UNIT" --no-pager -o cat -n 100 || true
exit "$STATUS"
