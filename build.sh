#!/usr/bin/env bash

# Exit on error
set -e

BASE_NAME=$(basename $(pwd))
TARGET_NAME=""
TARGET_VERSION=""
COMPILE_TIME=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
  echo "[$(date)]${GREEN} $@ ${NC}"
}

log_warn() {
  echo "[$(date)]${YELLOW} $@ ${NC}"
}

log_error() {
  echo "[$(date)]${RED} $@ ${NC}"
}


clean() {
  log_warn "删除编译产物"
  rm -rf "${BASE_NAME}-"*
  log_warn "清理编译缓存"
  go clean -cache
}

build_info() {
  OS=$(uname -s)
  if [ "$OS" = "Darwin" ]; then
    log_warn "在 macOS 系统下编译暂无法获取真实版本时间"
    log_warn "建议在 linux 编译机中交叉编译"
    COMPILE_TIME=$(date)
    ISO_DATE=$(TZ="UTC" git log -1 --format=%cd --date=iso)
    YEAR=$(echo $ISO_DATE | cut -d'-' -f1 | cut -c3-4)
    MONTH=$(echo $ISO_DATE | cut -d'-' -f2 | sed 's/^0//')
    DAY=$(echo $ISO_DATE | cut -d'-' -f3 | cut -d' ' -f1 | sed 's/^0//')
    TIME=$(echo $ISO_DATE | cut -d' ' -f2 | tr -d ':')
    TARGET_VERSION="$YEAR.$MONTH.$DAY-$TIME"
  else
    COMPILE_TIME=$(date --rfc-2822)
    TARGET_VERSION=$(TZ="UTC" date -d "$(git log -1 --format=%cd --date=iso)"  +"%y.%-m.%-d-%H%M%S")
  fi

  TARGET_NAME="${BASE_NAME}-$(go env GOOS)-$(go env GOARCH)-v${TARGET_VERSION}$(go env GOEXE)"
}

build() {
  build_info
  LD_FLAGS="-s -w -extldflags=-static -X 'github.com/xmx/aegis-common/banner.compileTime=${COMPILE_TIME}'"
  CGO_ENABLED=0 go build -o "${TARGET_NAME}" -trimpath -v -ldflags "${LD_FLAGS}" ./main
  if [ $? -ne 0 ]; then
    log_error "编译出错"
    exit 1
  fi
}

main() {
  if [ "$1" = "clean" ]; then
    clean
    exit 0
  fi

  log_info "开始编译..."
  build
  log_info "编译完成：${TARGET_NAME}"
}

main "$@"
