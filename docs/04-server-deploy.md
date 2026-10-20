# WakeComputer

Android 远程开关机。服务端跑在局域网里的 OpenWrt 上，用标准 Wake-on-LAN 开机，用主机上的轻量 Agent 关机和重启。外网复用已有的 HTTPS 反向代理，不新开公网端口。

- 服务端：一个 Go 二进制，SQLite，单容器，内存上限 128 MB
- Agent：Windows 服务 / Linux systemd，只监听内网
- App：Android 客户端，登录后管理多台主机

```
手机 App  --HTTPS-->  反向代理  --HTTP-->  OpenWrt 上的服务端 :8085
                                              |  UDP 魔术包 / 内网 Agent
                                              v
                                           局域网主机
```

## 要求

- OpenWrt / iStoreOS，x86_64，已安装 Docker 和 Compose
- 数据目录放在 ext4 上，可用内存至少 128 MB
- 开发机安装 Go 1.24+
- 路由器不要把 `8085` 暴露到公网

## 部署

推荐方式是拉取已经构建好的镜像，不在路由器上编译。

把 `deploy/docker-compose.yml`、`deploy/.env.example` 和 `deploy/up.sh` 拷到路由器，编辑 `.env` 里的 `WC_IMAGE`，然后：

```sh
cd /opt/wakecomputer
vi .env          # WC_IMAGE=ghcr.io/owner/wakecomputer:0.1.0
sh up.sh
```

也可以一行覆盖镜像地址：

```sh
WC_IMAGE=ghcr.io/owner/wakecomputer:0.1.0 sh up.sh
```

`up.sh` 会创建 `/opt/wakecomputer/data`，`docker compose pull`，再 `up -d`，并等待 healthcheck。之后更新同样执行这一条。

必须使用 host 网络。Wake-on-LAN 的 UDP 广播出不了 Docker bridge。容器以 uid `1000` 运行，根文件系统只读，可写目录只有 `/data`。

发布镜像在开发机进行，路由器只拉取：

```bash
docker build -t ghcr.io/owner/wakecomputer:0.1.0 .
docker push ghcr.io/owner/wakecomputer:0.1.0
```

没有镜像仓库时，仍可用静态二进制离线安装：`powershell -File scripts/bundle.ps1`，再 `OWRT_HOST=192.168.1.3 bash scripts/deploy-openwrt.sh`。安装脚本看到 `WC_IMAGE` 含 `/` 时会改为 `docker pull`。

## 配置

编辑路由器上的 `/opt/wakecomputer/.env`，然后重建容器：

```sh
cd /opt/wakecomputer
vi .env
docker compose up -d --force-recreate wakecomputer
```

| 变量 | 示例 | 说明 |
|---|---|---|
| `WC_IMAGE` | `ghcr.io/owner/wakecomputer:0.1.0` | 要拉取的镜像。更新版本只改这一行再执行 `sh up.sh` |
| `WC_HTTP_ADDR` | `192.168.1.3:8085` | 监听地址，建议绑局域网 IP |
| `WC_PUBLIC_URL` | `http://192.168.1.3:8085` | App 复制出来的 Agent 安装命令用这个地址。接入只允许局域网，必须填内网地址，不要填公网域名 |
| `WC_ADMIN_PASSWORD` | 留空或自行设置 | 仅在还没有用户时创建 `admin` |
| `WC_WOL_DEFAULT_BROADCAST` | `192.168.1.255` | 默认唤醒广播地址 |
| `WC_TRUSTED_PROXIES` | `192.168.1.3/32` | 允许携带 `X-Forwarded-For` 的反代来源 |

数据库、JWT 密钥和 Agent 安装包都在 `/opt/wakecomputer/data`。更新服务时不要删除这个目录。

首次启动如果还没有用户，会创建 `admin`。`.env` 里写了 `WC_ADMIN_PASSWORD` 就用它，否则随机生成并打印到容器日志。日志只保留约 15 MB，转走之后就找不回了，请当时记下来。

## 忘记密码

改 `.env` 里的 `WC_ADMIN_PASSWORD` 再重启，**不会**改变已经存在的账号。这个变量只在数据库里还没有用户时生效。

还记得旧密码时，在 App 的「设置 → 修改密码」里改。新密码至少 8 位。改完后所有已登录设备都会退出。

忘记旧密码时，在路由器上重置。要先停容器，否则 SQLite 可能被正在运行的进程锁住：

```sh
cd /opt/wakecomputer
docker compose stop wakecomputer
docker compose run --rm --no-deps -u 1000:1000 \
  --entrypoint /app/wakecomputer wakecomputer \
  admin reset-password -username admin
docker compose start wakecomputer
```

不写 `-password` 时会生成 12 位随机密码，并打印 `password: ...`。也可以自己指定，建议至少 8 位，以便以后还能在 App 里修改：

```sh
admin reset-password -username admin -password '至少8位'
```

重置只会让 App 退出登录，**不会**解除 Agent 绑定。服务端重启也一样：绑定保存在 `/opt/wakecomputer/data`，Windows 上的 Agent 会自己重连，不用重新安装。

不要为了重置密码删除 `data/wake.db`。那样主机、Agent 指纹和审计记录会一起丢掉。只有确定要清空整个系统时才删数据库；下次启动会按 `.env` 重新创建 `admin`。

## 安装 Agent

只能在局域网绑定。公网域名上的接入请求会被拒绝。App 主机详情里复制的安装命令，服务器地址来自 `WC_PUBLIC_URL`；这个变量必须是 `http://<路由器IP>:8085`。如果复制出来的是公网地址，先改成内网地址再执行。

在 App 主机详情生成接入码后，在对应主机执行。Windows 用管理员 PowerShell：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -c "irm http://192.168.1.3:8085/install.ps1 -OutFile $env:TEMP\wake-agent-install.ps1; & $env:TEMP\wake-agent-install.ps1 -Server http://192.168.1.3:8085 -Code ABCD1234"
```

Linux：

```bash
curl -fsSL http://192.168.1.3:8085/install.sh | sudo bash -s -- \
  --server http://192.168.1.3:8085 --code ABCD1234
```

绑定写入数据库后一直有效。改密码、重置密码、重启或重建容器都不用在 Windows 上重装。心跳会更新内网 IP。只有删除 `data/`，或在 App 里解绑，才需要重新接入。

## 反向代理

在现有 443 规则下新增子规则，不要再监听一个 443。

- 域名：`wake.example.com`
- 后端：`http://192.168.1.3:8085`
- 证书覆盖该域名
- 关闭响应缓冲，否则状态推送（SSE）会被延迟

## 验证

```sh
wget -qO- http://127.0.0.1:8085/healthz
wget -S -O- http://127.0.0.1:8085/api/v1/hosts
```

`healthz` 应返回 `"status":"ok"`。主机接口未登录时应返回 `401`。

## 更新

改 `.env` 里的 `WC_IMAGE` 标签，再执行 `sh up.sh`。脚本会重新拉取并重建容器。`data/` 不要删。

没有仓库、仍在用本地 `docker import` 镜像时，把新的 `wakecomputer-linux-amd64` 放到 `/opt/wakecomputer`，再执行 `scripts/redeploy-remote.sh`。

## 备份

```sh
docker exec wakecomputer /app/wakecomputer backup /data/backups/$(date +%F).db
```

回滚时换回旧二进制并重建容器即可，不要回滚 `data/`。

## 常见问题

| 现象 | 处理 |
|---|---|
| 登录提示密码错误，改了 `.env` 仍不行 | 账号已经存在。按上面的「忘记密码」用 CLI 重置，不要只改环境变量 |
| 第一次启动的密码找不到了 | 只打印在当时的容器日志里，日志轮转后就没了。直接重置 |
| App 改密码说至少 8 位 | 这是 App 的限制。CLI 可以设更短的密码，但之后在 App 里再改时，新密码仍要至少 8 位 |
| 外网域名解析失败 | 若域名只有 IPv6，没有公网 IPv6 的设备会失败。局域网改用 `http://<路由器IP>:8085` |
| 能打开页面但唤醒不了主机 | 确认用的是 host 网络，且 `WC_WOL_DEFAULT_BROADCAST` 是该网段的广播地址 |
| 重置密码或重启后还要重装 Agent | 不需要。绑定不会因改密码或重启失效。只有删了 `data/` 或在 App 里解绑才要重新绑定 |
| App 复制的安装命令绑定失败 | 命令里的地址来自 `WC_PUBLIC_URL`。改成 `http://<路由器IP>:8085` 后再执行；公网域名会被拒绝 |
