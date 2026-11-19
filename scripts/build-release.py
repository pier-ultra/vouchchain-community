#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""VouchChain 跨平台发布包构建：6 个裸二进制（linux/windows/darwin × amd64/arm64）。

纯 Go sqlite 驱动 → CGO_ENABLED=0 静态编译，任何装了 Go 的机器都能出全平台包，无需 gcc。
发布产物是【单文件自包含裸二进制】：
  - 内嵌前端 web/dist（main.go go:embed）→ 无需旁置 web/dist；
  - 内嵌迁移脚本 migrations（main.go go:embed）→ 启动自动建库迁移；
  - 启动时自动生成 config.yml + data/（internal/preflight）→ 零配置双击即跑。
因此无需打包 zip、无需附带 config.example / STARTUP.md / web/dist，直接分发单个二进制文件即可。

用法：python scripts/build-release.py [版本号]     # 默认 0.3.0
产物：release/vouchchain-<ver>-<os>-<arch>[.exe]（裸二进制，无 zip）
"""
import os
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "release"

PLATFORMS = [("linux", "amd64"), ("linux", "arm64"),
             ("windows", "amd64"), ("windows", "arm64"),
             ("darwin", "amd64"), ("darwin", "arm64")]


def go_env() -> dict:
    """Go 编译环境（沿用本机已验证的 Go 安装与缓存）。"""
    env = dict(os.environ)
    candidates = [
        Path(r"E:\Go\bin"),
        Path("/usr/local/go/bin"),
        Path("/opt/homebrew/bin"),
        Path(sys.base_prefix, "go", "bin"),
    ]
    go_dir = next((p for p in candidates if (p / ("go.exe" if os.name == "nt" else "go")).exists()), None)
    if go_dir:
        env["PATH"] = str(go_dir) + os.pathsep + env.get("PATH", "")
    # 本机 Go 缓存加速仅在 Windows 生效；POSIX 上 r"E:\..." 是相对路径，
    # 会导致 go 因 GOCACHE/GOMODCACHE 非绝对路径而启动即失败
    if os.name == "nt":
        for name, path in (("GOMODCACHE", r"E:\GoCache\mod"), ("GOCACHE", r"E:\GoCache\build")):
            if path and Path(path).parent.exists():
                env[name] = path
    env.setdefault("GOPROXY", "https://goproxy.cn,direct")
    env["CGO_ENABLED"] = "0"
    env["GOFLAGS"] = "-trimpath"
    env["GOSUMDB"] = "off"
    return env


def go_cmd() -> str:
    """go 可执行文件的绝对路径。

    Windows 下 Python 的 subprocess 不像 shell 那样自动补 .exe / 走 PATHEXT 查找，
    传裸的 "go" 会报 [WinError 2]，所以必须用绝对路径调用。
    """
    ext = "go.exe" if os.name == "nt" else "go"
    for d in (Path(r"E:\Go\bin"), Path(r"C:\Program Files\Go\bin"),
              Path(r"C:\Program Files (x86)\Go\bin"), Path(r"C:\Go\bin"),
              Path(r"D:\Go\bin"), Path(r"F:\Go\bin"), Path("/usr/local/go/bin")):
        exe = d / ext
        if exe.exists():
            return str(exe)
    found = shutil.which("go") or shutil.which(ext)
    if found:
        return found
    sys.exit("错误：未找到 go，请安装 Go 1.23+ 并加入 PATH")


def build_binary(os_name: str, arch: str, ver: str) -> Path:
    """交叉编译单个裸二进制，直接输出到 release/，无 zip 包装。

    返回二进制路径。文件名带版本与平台，便于用户区分：
      windows → vouchchain-<ver>-windows-<arch>.exe
      linux/darwin → vouchchain-<ver>-<os>-<arch>
    """
    bin_name = f"vouchchain-{ver}-{os_name}-{arch}"
    if os_name == "windows":
        bin_name += ".exe"
    target = OUT / bin_name
    env = go_env()
    env["GOOS"], env["GOARCH"] = os_name, arch
    print(f"==> 编译 {os_name}/{arch} ({target})", flush=True)
    proc = subprocess.run(
        [go_cmd(), "build", "-trimpath", "-ldflags", "-s -w", "-o", str(target), "."],
        cwd=ROOT, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        sys.exit(f"编译失败 {os_name}/{arch}（exit {proc.returncode}）：\n"
                 + proc.stderr.decode("utf-8", "replace"))
    return target


def main() -> None:
    ver = sys.argv[1] if len(sys.argv) > 1 else "0.3.0"
    OUT.mkdir(exist_ok=True)
    # 清理旧产物（仅本脚本命名的裸二进制）
    for old in OUT.glob("vouchchain-*"):
        if old.is_file():
            old.unlink()
    done = []
    for os_name, arch in PLATFORMS:
        path = build_binary(os_name, arch, ver)
        done.append((path.name, path.stat().st_size))
    print("\n=== 发布裸二进制清单（v%s）===" % ver)
    total = 0
    for name, size in done:
        total += size
        print(f"  {name:<44} {size/1024/1024:6.1f} MB")
    print(f"  {'合计 %d 个裸二进制' % len(done):<44} {total/1024/1024:6.1f} MB")
    print(f"  输出目录：{OUT}")
    print("  每个文件均为单文件自包含：内嵌前端+迁移，双击/./ 即自动生成 config.yml + data/ 并建库。")


if __name__ == "__main__":
    main()
