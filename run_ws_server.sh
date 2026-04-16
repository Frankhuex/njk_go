#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

find_go() {
  local candidates=(
    "go"
    "/usr/local/go/bin/go"
    "/opt/homebrew/bin/go"
    "/opt/homebrew/opt/go/bin/go"
    "$HOME/go/bin/go"
    "$HOME/sdk/go/bin/go"
  )

  local candidate
  for candidate in "${candidates[@]}"; do
    if command -v "$candidate" >/dev/null 2>&1; then
      command -v "$candidate"
      return 0
    fi
    if [[ -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  local sdk_go
  for sdk_go in "$HOME"/sdk/go*/bin/go; do
    if [[ -x "$sdk_go" ]]; then
      printf '%s\n' "$sdk_go"
      return 0
    fi
  done

  return 1
}

GO_BIN="$(find_go || true)"

if [[ -z "$GO_BIN" ]]; then
  echo "[ERROR] 未找到可用的 Go 可执行文件。"
  echo "[ERROR] 已检查常见安装路径以及 \$HOME/sdk/go*/bin/go。"
  exit 1
fi

echo "[INFO] 使用 Go: $GO_BIN"
"$GO_BIN" version
echo "[INFO] 启动 WebSocket 服务，监听"
exec "$GO_BIN" run ./cmd/server
