#!/usr/bin/env bash
# VouchChain 社区版 · 本地便捷构建
#
# 设计：CGO_ENABLED=0 + 纯 Go sqlite 驱动 → 零依赖单二进制，
#       任何装了 Go 的机器都能编（Linux/macOS/Windows、无需 gcc / 无需 Node）。
# -trimpath -ldflags "-s -w" → 去路径+符号表+调试信息，产物约 36MB。
#
# 用法：
#   bash scripts/build.sh          # 默认名 vouchchain(.exe)
#   bash scripts/build.sh vc.exe   # 指定输出文件名
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${1:-vouchchain}"
case "$OUT" in
  *.exe) ;;                                  # 显式 .exe
  *) if [ "$(uname -s)" = "MINGW"* ] || [ "$(uname -s)" = "CYGWIN"* ] || [ "$(uname -s)" = "MSYS"* ]; then
      OUT="$OUT.exe"                         # Windows(Git Bash) 默认加 .exe
    fi ;;
esac

export CGO_ENABLED=0     # 纯 Go：不依赖 C 编译器，跨平台可交叉编译
export GOFLAGS=-trimpath
mkdir -p bin
echo "==> 构建 bin/$OUT（纯 Go sqlite 驱动，-s -w）"
go build -trimpath -ldflags "-s -w" -o "bin/$OUT" .
echo "==> 完成：$(du -h "bin/$OUT" | cut -f1)"
