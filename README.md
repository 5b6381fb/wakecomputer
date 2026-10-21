# WakeComputer

Android 多主机远程开关机系统：标准 Wake-on-LAN 开机 + 轻量 Agent 关机/重启 + OpenWrt Docker 后端 + HTTPS 反向代理。

- **服务端**：Go 单二进制 + SQLite，单容器，常驻内存 < 80 MB
- **Agent**：Windows 服务 / Linux systemd，mTLS + 证书指纹固定，仅监听内网 `:45871`
- **App**：Kotlin + Jetpack Compose，Material 3，深色/浅色，批量操作，记住密码
- **离线可用**：网络中断时保留登录态并显示缓存，恢复后自动重连，不用重输密码
- **局域网扫描**：服务端读取 ARP + NetBIOS 名称，添加主机时可一键选择设备
- **外网**：复用 Lucky 既有 443/8443，仅新增 `wake.example.com` 子规则

## 目录

```
cmd/server            服务端（API、WoL、状态、批量、SSE、Agent 接入）
cmd/agent             Windows/Linux Agent
internal/             认证、存储、状态机、任务引擎、WoL
android/              Android App（Compose）
deploy/               compose、安装脚本、反向代理说明
scripts/              构建、Smoke/E2E、环境安装
docs/                 需求 / 技术方案 / 任务清单
```

## 本地开发

要求 Go 1.24+。

```bash
go mod download
go test ./...
go run ./cmd/server
```

首次启动会打印随机管理员密码；也可以用环境变量指定：

```bash
WC_DATA_DIR=./.localdata \
WC_HTTP_ADDR=127.0.0.1:8085 \
WC_ADMIN_PASSWORD=change-me-please \
go run ./cmd/server
```

本地忘记密码时，不要删 `.localdata`（里面有数据库）。停掉正在跑的进程后执行：

```bash
WC_DATA_DIR=./.localdata go run ./cmd/server admin reset-password -username admin
```

不带 `-password` 会打印一个新的随机密码。重置只让 App 退出登录，不会解除 Agent 绑定。已部署环境的步骤见 [简介与部署](docs/04-server-deploy.md#忘记密码)。

快速验证：

```powershell
powershell -File scripts/smoke.ps1
powershell -File scripts/e2e-local.ps1   # 含 Agent 注册/心跳/指令通道，不会真的关机
```

## 构建

```bash
# 服务端 + 各平台 Agent（WSL / Linux）
bash scripts/build.sh

# 打包 linux/amd64 Docker 镜像 tar（需要 Docker）
bash scripts/build-image.sh
```

Android APK：

```bash
cd android
./gradlew :app:assembleDebug
# 输出 app/build/outputs/apk/debug/app-debug.apk
```

> 当前 release 构建暂用 debug 签名，正式分发前请在 `android/app/build.gradle.kts` 配置自己的签名。

## 部署到 OpenWrt（x86_64）

前置：Docker + Compose、数据目录位于 ext4 挂载点、可用内存 ≥ 128 MB。

推荐直接拉取镜像。把 `deploy/docker-compose.yml`、`deploy/.env.example`、`deploy/up.sh` 放到 `/opt/wakecomputer`，在 `.env` 里填写 `WC_IMAGE`，然后：

```sh
WC_IMAGE=ghcr.io/owner/wakecomputer:0.1.0 sh up.sh
```

说明见 [简介与部署](docs/04-server-deploy.md#部署)。下面是没有镜像仓库时的离线安装。

1. 生成部署包：

   ```bash
   bash scripts/build.sh
   bash scripts/build-image.sh
   ```

2. 把 `dist/wakecomputer-0.1.0.tar`、`dist/wake-agent-*`、`deploy/*` 拷到路由器：

   ```bash
   scp dist/wakecomputer-0.1.0.tar root@192.168.1.3:/tmp/
   scp dist/wake-agent-* root@192.168.1.3:/tmp/
   scp deploy/docker-compose.yml deploy/.env.example deploy/openwrt-install.sh root@192.168.1.3:/tmp/wake-deploy/
   ```

3. 安装：

   ```bash
   ssh root@192.168.1.3
   mkdir -p /opt/wakecomputer/data/downloads
   cp /tmp/wake-deploy/docker-compose.yml /tmp/wake-deploy/.env.example /opt/wakecomputer/
   cp /tmp/wake-agent-* /opt/wakecomputer/data/downloads/
   cp /tmp/wakecomputer-0.1.0.tar /opt/wakecomputer/
   cd /opt/wakecomputer
   docker load -i wakecomputer-0.1.0.tar
   cp .env.example .env && vi .env      # WC_HTTP_ADDR 与 WC_PUBLIC_URL 都填局域网地址
   docker compose up -d
   docker compose logs -f wakecomputer  # 记下首次密码；日志轮转后无法找回，见 docs/04-server-deploy.md
   ```

4. Lucky 反向代理：在**现有 443 Web 服务规则**下新增子规则：

   - 前端域名：`wake.example.com`
   - 后端地址：`http://192.168.1.3:8085`
   - 证书需覆盖该子域（泛域名或 SAN）

   不要新建第二个 443 监听规则。

5. 验证：

   ```bash
   curl -i https://wake.example.com/healthz
   curl -i https://wake.example.com/api/v1/hosts   # 期望 401
   ```

## 安装 Agent

绑定只能在局域网完成，公网域名会被拒绝。在 App 中打开主机详情生成接入码，然后在该主机执行，`--server` / `-Server` 填局域网地址：

**Linux**

```bash
curl -fsSL http://192.168.1.3:8085/install.sh | sudo bash -s -- \
  --server http://192.168.1.3:8085 --code ABCD1234
```

**Windows（管理员 PowerShell）**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -c "irm http://192.168.1.3:8085/install.ps1 -OutFile $env:TEMP\wake-agent-install.ps1; & $env:TEMP\wake-agent-install.ps1 -Server http://192.168.1.3:8085 -Code ABCD1234"
```

Agent 会自行生成证书，服务端固定其指纹；心跳每 15 秒一次，并上报当前内网 IP。改管理员密码或重启服务端不会解除绑定，不用在 Windows 上重装。只有删除数据目录，或在 App 里解绑后，才需要重新接入。

## 主机侧前置设置

**Windows**：BIOS 开启 Wake on LAN / Power On by PCI-E；网卡启用 Wake on Magic Packet、Shutdown Wake-On-Lan；关闭快速启动（`powercfg /h off`）；建议 DHCP 静态绑定。

**Linux**：

```bash
sudo ethtool -s eth0 wol g
# 持久化：/etc/systemd/network/10-eth0.link 中 WakeOnLan=magic
```

## 局域网扫描

App 的「添加主机」页提供「扫描局域网设备」：

- 服务端（运行在 LAN 内）向默认网段发一轮 UDP 探测刷新 ARP，再读取邻居表
- 返回 IP、MAC，以及 Windows 主机的 NetBIOS 名称（Linux/macOS 可能显示“未知设备”）
- 选中后自动填写名称、MAC、IP 和广播地址；手动添加不受影响
- 默认网段由 `WC_WOL_DEFAULT_BROADCAST` 推导（如 `192.168.1.255` → `192.168.1.0/24`），也可用 `WC_LAN_SUBNET` 显式指定
- 接口：`GET /api/v1/discovery`，限流 4 次/分钟

## 安全要点

- 全链路 HTTPS，8085 仅内网可达；Lucky 管理台不要暴露公网。
- 登录限流 + 5 次失败锁定 15 分钟；Argon2id 密码哈希。
- Agent 使用每主机独立令牌 + 证书指纹固定，明文令牌不落库（AES-GCM 加密存储）。绑定只能从局域网发起，改密码或重启服务端不会解除。
- 所有写操作落审计表，默认保留 180 天。
- 批量关机 10 秒缓冲期内可取消。

## 已知限制

- 无 Agent 的主机只能唤醒和状态探测，不能关机/重启。
- 批量任务保存在内存，服务重启后未完成任务标记为中断（P1 持久化）。
- 定时开关机、TOTP、桌面小组件在 P1/P2 路线图中。

## 文档

- [需求文档](docs/01-requirements.md)
- [技术方案](docs/02-technical-design.md)
- [任务清单](docs/03-task-plan.md)
- [简介与部署](docs/04-server-deploy.md)
