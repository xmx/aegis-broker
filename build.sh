#!/usr/bin/env bash

# Exit on error
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored messages
print_message() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to get project info
get_project_info() {
    BASE_NAME=$(basename "$(pwd)")
    COMPILE_TIME=$(date --rfc-2822)
    VERSION=$(TZ="Etc/GMT" date -d "$(git log -1 --format=%cd --date=iso)"  +"%y.%-m.%-d-%H%M%S")
    TARGET_NAME="${BASE_NAME}-v${VERSION}$(go env GOEXE)"
}

# Function to clean all build files
clean() {
    print_warning "清理所有构建文件..."
    rm -rf "${BASE_NAME}"*
    print_warning "清理构建缓存..."
    go clean -cache
}

# Function to build the project
build() {
    print_message "GOPROXY: $(go env GOPROXY)"
    print_message "GOPRIVATE: $(go env GOPRIVATE)"
    print_message "GOVERSION: $(go version)"
    
    # Build configuration
    export CGO_ENABLED=0
#     LD_FLAGS="-s -w -extldflags=-static -X '$(go list -m)/banner.compileTime=${COMPILE_TIME}'"
    LD_FLAGS="-s -w -extldflags=-static -X 'github.com/xmx/aegis-common/banner.compileTime=${COMPILE_TIME}'"

    if ! go build -o "${TARGET_NAME}" -trimpath -v -ldflags "${LD_FLAGS}" ./main; then
        print_error "编译失败"
        exit 1
    fi

    print_message "版 本 号：${VERSION}"
    print_message "文 件 名：${TARGET_NAME}"
    print_message "编译时间：${COMPILE_TIME}"
}

# Main function
main() {
    get_project_info
    
    if [ "$1" = "clean" ]; then
        clean
        exit 0
    fi
    
    build
}

# Execute main function
main "$@"
