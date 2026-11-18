#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""VouchChain 跨平台发布包构建：6 个包（linux/windows/darwin × amd64/arm64）。

纯 Go sqlite 驱动 → CGO_ENABLED=0 静态编译，任何装了 Go 的机器都能出全平台包，无需 gcc。
用法：python scripts/build-release.py [版本号]     # 默认 0.3.0
产物：release/vouchchain-<ver>-<os>-<arch>.zip
"""
import os
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "release"
CONFIG_EXAMPLE = ROOT / "config" / "config.yaml.example"
WEB_DIST = ROOT / "web" / "dist"

PLATFORMS = [("linux", "amd64"), ("linux", "arm64"),
             ("windows", "amd64"), ("windows", "arm64"),
             ("darwin", "amd64"), ("darwin", "arm64")]

STARTUP = """# VouchChain 社区版 v{ver}

解压即用。零依赖：不需要数据库、不需要装任何服务，默认用 SQLite（单文件库，启动时自动建库并迁移）。

## 启动

```bash
{run}
```

启动后访问：{url}

## 配置（可选）

默认零配置。需要自定义时，复制示例并按需修改：

```bash
{cp}
```

- 数据库默认 SQLite（`./data/vouchchain.db`）
- 换 PostgreSQL：把 `database.default.type` 改为 `pgsql` 并填 `link`

## 首次启动

服务会自动执行 `migrations/*.sql` 建表，无需手工跑 SQL。
AI 能力（条款抽取/向量化）需在模型配置页录入密钥，或设置环境变量 `APP_AES_KEY`。

## 目录

- `{bin}` 后端单二进制
- `web/dist` 前端构建产物（与二进制同目录旁置）
- `config/config.yaml.example` 配置模板
- `migrations/` 建表脚本（服务自动执行，一般不用手动跑）
"""

RUN_CMD = {"linux": "./vouchchain", "darwin": "./vouchchain", "windows": "vouchchain.exe"}


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
    name = f"vouchchain-{os_name}-{arch}"
    tmp = Path(os_name) if os_name != "windows" else Path("vouchchain.exe")
    bin_name = "vouchchain.exe" if os_name == "windows" else "vouchchain"
    target = OUT / "tmp" / name / bin_name
    target.parent.mkdir(parents=True, exist_ok=True)
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


def make_zip(ver: str, os_name: str, arch: str, bin_path: Path) -> tuple[Path, int]:
    pkg = f"vouchchain-{ver}-{os_name}-{arch}"
    zip_path = OUT / f"{pkg}.zip"
    rel_bin = "vouchchain.exe" if os_name == "windows" else "vouchchain"
    with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        z.writestr(f"{pkg}/{rel_bin}", bin_path.read_bytes())
        z.writestr(f"{pkg}/config/config.yaml.example", CONFIG_EXAMPLE.read_bytes())
        z.writestr(f"{pkg}/STARTUP.md",
                   STARTUP.format(ver=ver, run=RUN_CMD[os_name], url="http://127.0.0.1:8000",
                                   cp=f"{os_name == 'windows' and 'copy' or 'cp'} config/config.yaml.example config/config.yaml",
                                   bin=rel_bin).encode("utf-8"))
        for f in sorted(WEB_DIST.rglob("*")):
            if f.is_file():
                z.write(f, f"{pkg}/web/dist/{f.relative_to(WEB_DIST).as_posix()}")
    return zip_path, zip_path.stat().st_size


def main() -> None:
    ver = sys.argv[1] if len(sys.argv) > 1 else "0.3.0"
    if not WEB_DIST.is_dir():
        sys.exit(f"错误：缺少前端产物 {WEB_DIST}，请先在 web/ 下执行 npm run build")
    OUT.mkdir(exist_ok=True)
    for old in OUT.glob("*.zip"):
        old.unlink()
    done = []
    for os_name, arch in PLATFORMS:
        bin_path = build_binary(os_name, arch, ver)
        zp, size = make_zip(ver, os_name, arch, bin_path)
        done.append((zp.name, size))
    shutil.rmtree(OUT / "tmp", ignore_errors=True)
    print("\n=== 发布包清单（v%s）===" % ver)
    total = 0
    for name, size in done:
        total += size
        print(f"  {name:<42} {size/1024/1024:6.1f} MB")
    print(f"  {'合计 %d 个包' % len(done):<42} {total/1024/1024:6.1f} MB")
    print(f"  输出目录：{OUT}")


if __name__ == "__main__":
    main()
