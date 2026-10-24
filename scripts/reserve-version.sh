#!/usr/bin/env bash
# 由 deploy.sh 通过 SSH 标准输入执行；与远端 restart.sh 共用 deploy.lock。
set -euo pipefail
root=$1
prefix=$2
baseline=$3
exec 9>"$root/deploy.lock"
flock -x 9
mkdir -p "$root/releases"
ledger="$root/versions.log"
if [[ ! -e $ledger ]]; then
  printf '%s\n' "$baseline" > "$ledger"
fi
max=-1
while IFS= read -r version; do
  if [[ $version == "$prefix."* ]]; then
    patch=${version#"$prefix."}
    [[ $patch =~ ^(0|[1-9][0-9]*)$ ]] || { echo '版本记录损坏' >&2; exit 1; }
    if (( patch > max )); then max=$patch; fi
  fi
done < "$ledger"
version="$prefix.$((max + 1))"
[[ ! -e $root/releases/$version && ! -e $root/releases/$version.uploading ]] || { echo '版本目录冲突，请核对版本记录' >&2; exit 1; }
mkdir "$root/releases/$version.uploading"
printf '%s\n' "$version" >> "$ledger"
printf '%s reserved\n' "$version" >> "$root/deploy-history.log"
echo "$version"
