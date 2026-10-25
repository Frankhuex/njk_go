#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
die() { echo "[ERROR] $*" >&2; exit 1; }
usage() { echo '用法: bash deploy.sh <主版本.次版本> [--check | --upload-only]'; }
[[ ${1:-} != --help && ${1:-} != -h ]] || { usage; exit 0; }
[[ $# -ge 1 && $# -le 2 ]] || { usage >&2; exit 1; }
PREFIX=$1
MODE=${2:-deploy}
[[ $PREFIX =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die '版本前缀格式应为 2.3'
case "$MODE" in deploy|--check|--upload-only) ;; *) die "未知选项: $MODE";; esac
CONFIG=${DEPLOY_CONFIG:-$ROOT_DIR/deploy.conf}
[[ -f $CONFIG ]] || die "找不到 ${CONFIG}，请先复制 deploy.conf.example"
source "$CONFIG"
: "${DEPLOY_HOST:?}" "${DEPLOY_PORT:?}" "${DEPLOY_KEY:?}" "${DEPLOY_ROOT:?}" "${DEPLOY_BASELINE:?}"
[[ $DEPLOY_HOST =~ ^[a-zA-Z0-9_.-]+@[a-zA-Z0-9_.-]+$ ]] || die '服务器地址格式错误'
[[ $DEPLOY_PORT =~ ^[0-9]+$ ]] || die 'SSH 端口格式错误'
[[ $DEPLOY_ROOT =~ ^/[a-zA-Z0-9_/-]+$ && $DEPLOY_ROOT != / && $DEPLOY_ROOT != *..* ]] || die '部署目录必须为安全的绝对路径'
[[ $DEPLOY_BASELINE =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die '初始版本格式错误'
[[ -f $DEPLOY_KEY ]] || die "找不到 SSH 私钥: $DEPLOY_KEY"
for tool in ssh rsync xz; do command -v "$tool" >/dev/null || die "缺少工具: $tool"; done
SSH=(ssh -p "$DEPLOY_PORT" -i "$DEPLOY_KEY" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10)
remote() { "${SSH[@]}" "$DEPLOY_HOST" "$@"; }
preflight() {
remote "bash -s -- '$DEPLOY_ROOT' '$MODE'" <<'REMOTE'
set -euo pipefail
root=$1
test -d "$root" && test -w "$root" || { echo '部署目录不存在或不可写' >&2; exit 1; }
test -f "$root/.env" || { echo '服务器缺少 .env' >&2; exit 1; }
for tool in rsync flock bash python3; do command -v "$tool" >/dev/null; done
python3 -c 'import lzma'
if [[ $2 == deploy ]]; then
  test -f "$root/restart.sh" || { echo '服务器缺少 restart.sh；可用 --upload-only 单独上传' >&2; exit 1; }
fi
case "$(uname -m)" in x86_64) echo amd64;; aarch64|arm64) echo arm64;; *) echo '不支持的服务器架构' >&2; exit 1;; esac
REMOTE
}
ARCH=$(preflight)
echo "[INFO] 服务器检查通过: linux/$ARCH"
[[ $MODE != --check ]] || exit 0
TARGET_OS=linux TARGET_ARCH="$ARCH" bash "$ROOT_DIR/build.sh"
STAGE=$(mktemp -d "${TMPDIR:-/tmp}/njk-deploy.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT
cp "$ROOT_DIR/build/njk_go-linux-$ARCH" "$STAGE/njk_go"
VERSION=$(remote "bash -s -- '$DEPLOY_ROOT' '$PREFIX' '$DEPLOY_BASELINE'" < "$ROOT_DIR/scripts/reserve-version.sh")
[[ $VERSION =~ ^${PREFIX//./\.}\.[0-9]+$ ]] || die '服务器返回的版本号无效'
echo "[INFO] 打包版本 ${VERSION}"
# 发布包只有程序本身，xz 压缩成单文件传输。
xz -6 -c "$STAGE/njk_go" > "$STAGE/njk_go.xz"
echo "[INFO] 压缩包大小: $(wc -c < "$STAGE/njk_go.xz" | tr -d ' ') bytes"
# rsync 的远程 shell 字符串由 Bash %q 转义，支持密钥路径中的空格。
printf -v RSYNC_SSH '%q ' "${SSH[@]}"
echo "[INFO] 上传版本 $VERSION"
rsync -a --partial --progress --stats -e "$RSYNC_SSH" \
  "$STAGE/njk_go.xz" "$DEPLOY_HOST:$DEPLOY_ROOT/releases/$VERSION.uploading/"
remote "bash -s -- '$DEPLOY_ROOT' '$VERSION'" <<'REMOTE'
set -euo pipefail
root=$1
version=$2
cd "$root/releases/$version.uploading"
python3 -c 'import lzma, shutil; shutil.copyfileobj(lzma.open("njk_go.xz"), open("njk_go", "wb"))'
rm njk_go.xz
chmod 755 njk_go
test ! -e "$root/releases/$version"
mv "$root/releases/$version.uploading" "$root/releases/$version"
printf '%s uploaded\n' "$version" >> "$root/deploy-history.log"
REMOTE
if [[ $MODE == --upload-only ]]; then
  echo "[INFO] 上传完成: $DEPLOY_ROOT/releases/${VERSION}；尚未重启服务"
else
  remote "bash '$DEPLOY_ROOT/restart.sh' '$VERSION'"
  echo "[INFO] 远端 restart.sh 执行成功: $VERSION"
fi
