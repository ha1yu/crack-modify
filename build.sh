#!/bin/bash

# crack-modify 多平台交叉构建脚本
#
# 用法: ./build.sh
# 产物: ./bin/crack_modify_<os>_<arch>[.exe]
#
# 说明:
# - 使用 go1.20.14 工具链(go.mod 要求 go 1.20)
#   重要: 必须用 1.20.x 才能产出支持 Windows 7 的二进制(Go 1.21+ 已放弃 Win7)。
#   GOTOOLCHAIN=go1.20.14 会自动下载该版本工具链。
# - CGO_ENABLED=0 纯静态, 依赖(crypto/ssh, go-redis, mgo 等)均无 cgo 要求, 可放心交叉编译
# - -trimpath 去掉路径信息, -s -w 去掉调试符号与 DWARF, 缩小体积
# - windows 不加 -H windowsgui: 本工具是 CLI, 需要控制台输出

set -e

# 自愈: IDE/gopls 可能把 go.mod 的 go 指令抬到 1.24.0(三段式),
# 而 go1.20.14 只认两段式(报 "must match format 1.23")。
# 构建前强制写回 go 1.20, 保证脚本可重复运行。
# (x 系列依赖的实际版本由 go.mod 的 replace 锁定, 与此处指令无关)
if grep -q '^go 1\.24\.' go.mod 2>/dev/null; then
    sed -i 's/^go 1\.24\.[0-9]*/go 1.20/' go.mod
    echo "(已将 go.mod 的 go 指令修正为 1.20 以兼容 go1.20.14 工具链)"
fi

# 清理并创建 bin 目录
rm -rf bin/*
mkdir -p bin

# 锁定 go1.20.14 工具链(支持 Win7; go.mod 也是 go 1.20)
TOOLCHAIN="go1.20.14"
# 主包位于仓库根目录
MAIN_PKG="./"

build_client() {
    local os=$1 arch=$2 output=$3
    local ldflags="-s -w -buildid="
    # 注意: crack-modify 是命令行工具, windows 构建不加 -H windowsgui
    # (那样会隐藏控制台, 导致无输出)
    echo "  -> $(basename "$output")"
    GOTOOLCHAIN=$TOOLCHAIN CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -buildvcs=false -trimpath -ldflags "$ldflags" -o "$output" "$MAIN_PKG"
}

echo "开始构建 crack-modify (工具链 $TOOLCHAIN, 支持 Win7)..."

build_client darwin  arm64 ./bin/crack_modify_darwin_arm64
build_client darwin  amd64 ./bin/crack_modify_darwin_amd64
build_client linux   arm   ./bin/crack_modify_linux_arm
build_client linux   386   ./bin/crack_modify_linux_386
build_client linux   arm64 ./bin/crack_modify_linux_arm64
build_client linux   amd64 ./bin/crack_modify_linux_amd64
build_client windows 386   ./bin/crack_modify_windows_386.exe
build_client windows arm64 ./bin/crack_modify_windows_arm64.exe
build_client windows amd64 ./bin/crack_modify_windows_amd64.exe

echo "构建完成, 产物:"
ls -lh bin/
