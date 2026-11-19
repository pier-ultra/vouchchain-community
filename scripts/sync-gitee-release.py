#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 GitHub Release 同步到 Gitee：按 tag 建 release + 上传资产（同名资产跳过，可重入）。

用法：
  python3 scripts/sync-gitee-release.py v0.3.3 [v0.3.2 ...]

凭据来源（按优先级）：
  CI:      环境变量 GITHUB_TOKEN / GITEE_TOKEN
  本地:    git credential fill（凭据管理器）
"""
import base64, json, mimetypes, os, subprocess, sys, urllib.error, urllib.request, uuid

OWNER = "pier-ultra"
REPO = "vouchchain-community"
GH_API = f"https://api.github.com/repos/{OWNER}/{REPO}"
GE_API = f"https://gitee.com/api/v5/repos/{OWNER}/{REPO}"

def _cred(host):
    out = subprocess.run(["git", "credential", "fill"],
                         input=f"protocol=https\nhost={host}\n\n",
                         capture_output=True, text=True).stdout
    for line in out.splitlines():
        if line.startswith("password="):
            return line.split("=", 1)[1]
    return None

GH_TOKEN = os.environ.get("GITHUB_TOKEN") or _cred("github.com")
GE_TOKEN = os.environ.get("GITEE_TOKEN") or _cred("gitee.com")
assert GH_TOKEN and GE_TOKEN, "缺少 GITHUB_TOKEN / GITEE_TOKEN"
GH_HDR = {"Authorization": f"Bearer {GH_TOKEN}", "Accept": "application/vnd.github+json",
          "User-Agent": "release-sync"}

def gh_get(url):
    req = urllib.request.Request(url, headers=GH_HDR)
    return json.load(urllib.request.urlopen(req))

def ge_get(url):
    req = urllib.request.Request(url, headers={"User-Agent": "release-sync"})
    try:
        return json.load(urllib.request.urlopen(req)), None
    except urllib.error.HTTPError as e:
        return None, e.code

def ge_post_json(url, payload):
    data = json.dumps(payload).encode()
    req = urllib.request.Request(url, data=data,
        headers={"Content-Type": "application/json", "User-Agent": "release-sync"}, method="POST")
    return json.load(urllib.request.urlopen(req))

def ge_upload(release_id, path, filename):
    """multipart 上传资产到 Gitee release（POST .../attach_files），返回 HTTP 状态码"""
    boundary = uuid.uuid4().hex
    mime = mimetypes.guess_type(filename)[0] or "application/octet-stream"
    with open(path, "rb") as f:
        content = f.read()
    parts = []
    parts.append(f"--{boundary}\r\nContent-Disposition: form-data; name=\"access_token\"\r\n\r\n{GE_TOKEN}\r\n".encode())
    parts.append(f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\nContent-Type: {mime}\r\n\r\n".encode())
    body = b"".join(parts) + content + f"\r\n--{boundary}--\r\n".encode()
    url = f"{GE_API}/releases/{release_id}/attach_files"
    req = urllib.request.Request(url, data=body, method="POST",
        headers={"Content-Type": f"multipart/form-data; boundary={boundary}", "User-Agent": "release-sync"})
    try:
        resp = urllib.request.urlopen(req)
        return resp.status
    except urllib.error.HTTPError as e:
        print(f"  上传失败({e.code}): {e.read().decode()[:300]}")
        return e.code

def download(url, dest):
    subprocess.run(["curl", "-sSL", "--retry", "3", "-H",
                    f"Authorization: Bearer {GH_TOKEN}", "-o", dest, url], check=True)

def sync_release(tag):
    print(f"=== {tag} ===", flush=True)
    gh = gh_get(f"{GH_API}/releases/tags/{tag}")
    ge, _code = ge_get(f"{GE_API}/releases/tags/{tag}")
    if not ge:  # Gitee 不存在时返回 HTTP 200 + body null
        print("  Gitee 无此 release，创建…")
        repo_root = os.environ.get("VC_REPO_ROOT") or os.getcwd()
        sha = subprocess.run(["git", "rev-parse", f"{tag}^{{commit}}"],
                             capture_output=True, text=True, cwd=repo_root).stdout.strip()
        ge = ge_post_json(f"{GE_API}/releases?access_token={GE_TOKEN}",
                          {"tag_name": tag, "name": gh.get("name") or tag,
                           "body": gh.get("body") or "", "target_commitish": sha})
        print("  created @", sha[:7], ", id =", ge["id"])
    rid = ge["id"]
    existing = {a["name"] for a in (ge.get("assets") or [])}
    ok = True
    for a in gh["assets"]:
        name = a["name"]
        if name in existing:
            print(f"  跳过（已存在）: {name}")
            continue
        tmp = os.path.join(os.environ.get("TEMP", "/tmp"), f"rel_{tag}_{name}")
        print(f"  下载 {name} ({a['size']/1024/1024:.1f} MB)…", flush=True)
        download(a["browser_download_url"], tmp)
        print(f"  上传 {name} ({os.path.getsize(tmp)/1024/1024:.1f} MB)…", flush=True)
        st = ge_upload(rid, tmp, name)
        os.remove(tmp)
        print(f"  -> HTTP {st}")
        if st != 201:
            ok = False
    print(f"=== {tag} {'完成' if ok else '有失败项'} ===", flush=True)
    return ok

if __name__ == "__main__":
    tags = sys.argv[1:]
    if not tags:
        sys.exit("用法: python3 scripts/sync-gitee-release.py <tag> [<tag> ...]")
    all_ok = all(sync_release(t) for t in tags)
    sys.exit(0 if all_ok else 1)
