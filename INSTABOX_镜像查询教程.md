# InstaBox 连接 & Docker 镜像查询教程

> 两台 InstaBox 都通过同一个 SSH 网关 `ssh.instabox.sh:2222` 连接，用不同的用户名区分。
> 用户名就是 `@` 前面那串 token，**没有密码、也不用 SSH 密钥**（token 即身份）。

---

## 一、两台机器连接信息

| 机器 | 用户名（token） | 登录后 hostname | 主要用途 |
|------|----------------|----------------|---------|
| Box 1 | `ibssh_y2ltlk75thtaqfbmxdrtfx4rta` | `instabox-instabox-205` | AI / 基础设施（OCR、llama.cpp、向量库、对象存储） |
| Box 2 | `ibssh_n3mneeevuxlljbg72bvzzyoguq` | `instabox-box-dcc623` | 数据库兼容测试（OceanBase/TiDB/openGauss/达梦/金仓）+ 监控 |

SSH 命令：

```bash
# Box 1
ssh -p 2222 ibssh_y2ltlk75thtaqfbmxdrtfx4rta@ssh.instabox.sh

# Box 2
ssh -p 2222 ibssh_n3mneeevuxlljbg72bvzzyoguq@ssh.instabox.sh
```

如果装了 mosh，也可以（更抗网络抖动）：

```bash
mosh --ssh="ssh -p 2222" ibssh_y2ltlk75thtaqfbmxdrtfx4rta@ssh.instabox.sh
mosh --ssh="ssh -p 2222" ibssh_n3mneeevuxlljbg72bvzzyoguq@ssh.instabox.sh
```

---

## 二、首次连接的主机密钥确认

第一次连会提示：

```
The authenticity of host '[ssh.instabox.sh]:2222' can't be established.
ED25519 key fingerprint is SHA256:xxxx.
Are you sure you want to continue connecting (yes/no)?
```

输入 `yes` 回车即可，之后会写入 `~/.ssh/known_hosts`，后面就不再问了。

想跳过这一步（比如写进一次性脚本），加两个参数：

```bash
ssh -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  ibssh_y2ltlk75thtaqfbmxdrtfx4rta@ssh.instabox.sh
```

---

## 三、查询 Docker 镜像（核心）

连上之后，直接执行：

```bash
# 列出所有镜像
docker images

# 推荐：自定义表格，字段更清爽
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.ID}}"

# 只看仓库名和标签
docker images --format "{{.Repository}}:{{.Tag}}"

# 按占用空间从大到小排（找谁最占地方）
docker images --format "{{.Size}}\t{{.Repository}}:{{.Tag}}" | sort -h -r

# 查某个镜像（支持模糊过滤）
docker images | grep -i oceanbase
docker images --filter "reference=mysql*"
```

常用字段说明：

| 字段 | 含义 |
|------|------|
| `.Repository` | 镜像仓库名 |
| `.Tag` | 标签（版本） |
| `.ID` | 镜像 ID（前 12 位） |
| `.Size` | 镜像大小 |
| `.CreatedSince` | 创建距今时间 |

---

## 四、不进入交互，从本地直接查（批量/脚本推荐）

不用先 `ssh` 进去，可以在本地一条命令直接拿到结果，两台一起查：

```bash
# Box 1
ssh -p 2222 -o BatchMode=yes -o ConnectTimeout=15 \
  ibssh_y2ltlk75thtaqfbmxdrtfx4rta@ssh.instabox.sh \
  'docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"'

# Box 2
ssh -p 2222 -o BatchMode=yes -o ConnectTimeout=15 \
  ibssh_n3mneeevuxlljbg72bvzzyoguq@ssh.instabox.sh \
  'docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"'
```

参数解释：

| 参数 | 作用 |
|------|------|
| `-o BatchMode=yes` | 禁用任何交互提示（不弹密码确认），脚本里防止卡住 |
| `-o ConnectTimeout=15` | 15 秒连不上就超时退出 |
| `-o StrictHostKeyChecking=no` | 跳过首次主机密钥确认（可选） |
| `-o UserKnownHostsFile=/dev/null` | 不写 known_hosts（可选，配合上一条） |

> 小技巧：远程命令里想用本地的引号变量，注意用单引号包住整条远程命令，避免本地 shell 提前展开。

---

## 五、顺手查容器状态

镜像归镜像，容器是另一回事。经常一起看：

```bash
# 正在运行的容器
docker ps

# 所有容器（含已停止）
docker ps -a

# 只看容器名、状态、端口、镜像
docker ps -a --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}"

# 已停止的容器（清理前先看看）
docker ps -a --filter "status=exited"
```

---

## 六、常用运维操作（谨慎执行）

```bash
# 停止容器（不删）
docker stop <容器名或ID>

# 启动容器
docker start <容器名或ID>

# 重启容器
docker restart <容器名或ID>

# 进入容器
docker exec -it <容器名或ID> /bin/bash

# 看容器日志
docker logs -f <容器名或ID>
```

**清理类操作（先看清再动手，删除不可恢复）：**

```bash
# 删除已停止的容器
docker container prune

# 删除没有被任何容器引用的镜像（悬空镜像）
docker image prune

# 删除所有未被使用的镜像（范围更大，慎用）
docker image prune -a
```

> ⚠️ 建议流程：先 `docker ps -a` / `docker images` 打出清单确认，再决定删哪些。
> 不要直接 `docker system prune -a --volumes`，那会连数据卷一起清掉。

---

## 七、磁盘占用排查

```bash
# 总览：镜像 / 容器 / 构建缓存各占多少
docker system df

# 看详细明细
docker system df -v

# 容器日志是否膨胀
du -sh /var/lib/docker/containers/*/*-json.log 2>/dev/null | sort -h -r | head
```

---

## 八、常见问题

**Q：SSH 连上后就退出 / 卡住？**
A：如果用的是无交互脚本，确认加了 `-o BatchMode=yes`。token 本身就是用户名，不需要密码，正常不会卡。

**Q：远程显示的日期跟本地差 8 小时？**
A：容器内是 UTC 时区，本地是北京时间（UTC+8），属于正常时区差异，不影响连接和操作。

**Q：`docker: command not found`？**
A：说明当前登录用户没在 docker 组。用 `sudo docker ...`，或让用户加入 docker 组（需管理员）。

**Q：两台 Box 的镜像有什么区别？**
A：Box 1 是 AI/基础设施那套（OCR、llama.cpp、SeaweedFS、MatrixOne、pgvector 等）；Box 2 是数据库兼容测试那套（OceanBase、TiDB、openGauss、达梦、金仓、MySQL、PostgreSQL）+ Prometheus/Grafana 监控。
