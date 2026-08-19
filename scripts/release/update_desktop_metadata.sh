#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <version-without-v> [homepage-url]" >&2
  exit 1
fi

VERSION="$1"

# Windows 安装包（NSIS）要求产品版本必须是纯数字 X.Y.Z：
# project.nsi 会拼成 "X.Y.Z.0" 传给 VIProductVersion / VIFileVersion，
# 非数字版本（如手动触发的 manual-2-c7e6b26）会导致
# "invalid VIFileVersion format" 构建失败。
# 这里统一把写入 config.yml 的版本规范化为纯数字三段，
# 不匹配时回退到 0.0.0 并告警；Go 二进制版本（KITE_VERSION）由
# workflow 单独用 tag_name 注入，不受此归一化影响。
ORIGINAL_VERSION="${VERSION}"
if [[ "${VERSION}" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
  VERSION="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
else
  echo "warning: invalid NSIS version '${VERSION}', falling back to 0.0.0" >&2
  VERSION="0.0.0"
fi
export KITE_VERSION="${VERSION}"

perl -0pi -e 's/(version:\s*")[^"]+(")/${1}$ENV{KITE_VERSION}$2/g' \
  desktop/build/config.yml

(
  cd desktop
  wails3 task common:update:build-assets
)

echo "desktop release metadata updated: version=${VERSION}${ORIGINAL_VERSION:+ (from ${ORIGINAL_VERSION})}"
