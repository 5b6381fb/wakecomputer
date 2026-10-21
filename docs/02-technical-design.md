# WakeComputer 技术方案（v1.0）

> 配套文档：`01-requirements.md` · `03-task-plan.md`
> 目标：在 OpenWrt Docker 上以单容器提供 HTTPS API，Android 端通过 Lucky 反向代理访问；WoL 负责开机，Agent 负责关机/重启/心跳。

---

## 1. 架构总览

```
┌──────────────────────────┐
│  Android App (Kotlin)    │  4G/5G/Wi-Fi
│  Compose + Retrofit      │
└───────────┬──────────────┘
            │ HTTPS (443)
┌───────────▼──────────────────────────────────────────┐
│  Lucky（既有，443 终止 TLS）                           │
│  wake.example.com → 192.168.1.2:8085                 │
│  在现有规则下新增子规则，不改动 sub2api                │
└───────────┬──────────────────────────────────────────┘
            │ HTTP (LAN, 仅内网)
┌───────────▼──────────────────────────────────────────┐
│  OpenWrt · Docker (network_mode: host)               │
│  container: wakecomputer                             │
│  ├─ HTTP API (chi)     /api/v1/*                     │
│  ├─ 认证 (JWT+Argon2id) / 限流 / 审计                 │
│  ├─ WoL Sender  ── UDP 9 广播 ───────────────┐        │
│  ├─ 状态引擎 (心跳/TCP/ICMP)                  │        │
│  ├─ 批量任务引擎 (并发/取消/SSE)               │        │
│  └─ SQLite (/data/wake.db, WAL)              │        │
└──────────────────────────────────────────────┼────────┘
                                               │
        ┌──────────────────────────────────────┘
        │  LAN
┌───────▼────────┐   ┌────────────────┐   ┌────────────────┐
│ Windows 主机    │   │ Linux 主机      │   │ 无 Agent 主机   │
│ wake-agent.exe │   │ wake-agent     │   │ 仅 WoL + 探测   │
│ SYSTEM 服务     │   │ systemd/root   │   │                │
│ :45871 mTLS    │   │ :45871 mTLS    │   │                │
└────────────────┘   └────────────────┘   └────────────────┘
```

**关键数据流**

1. **开机**：App → 反代 → API → WoL Sender 发 UDP 魔术包 → 目标网卡 → 状态引擎轮询 → SSE 推送给 App。
2. **关机**：App → API → 校验主机在线 → 调用 Agent `POST https://host:45871/v1/command`（mTLS + token）→ Agent 延迟执行 `shutdown` → 心跳消失 → 状态转离线。
3. **状态**：Agent 每 15 s 心跳 → 服务端更新 `last_seen_at` → 广播 `host.status` 事件（SSE）；无 Agent 主机由服务端主动 TCP/ICMP 探测。

---

## 2. 技术选型

| 层次 | 选型 | 理由 | 备选与否决原因 |
|---|---|---|---|
| 后端语言 | Go 1.24+ | 静态单二进制、内存占用低、交叉编译简单、并发模型适合批量任务 | Node.js：镜像大、内存高；Python：依赖多、镜像 > 150 MB |
| HTTP 框架 | `chi` + 标准库 | 轻量、中间件生态足够、无反射 | Gin：依赖略重，非必需 |
| 数据库 | SQLite（`modernc.org/sqlite`，纯 Go） | 零外部服务、单文件备份、无 CGO 便于多架构构建 | PostgreSQL：违反"不影响现有容器"约束；BoltDB：查询/审计能力弱 |
| 迁移 | 内嵌 SQL + `schema_version` 表 | 无外部工具，启动自动迁移 | golang-migrate CLI：多一个二进制 |
| 认证 | JWT HS256（随机 32B 密钥）+ 刷新令牌哈希入库 | 无状态访问令牌 + 可吊销刷新令牌 | Session Cookie：App 场景不自然 |
| 密码哈希 | Argon2id（m=19MB, t=2, p=1） | OWASP 最低推荐参数，适配路由器内存 | bcrypt：可接受但 Argon2id 更优 |
| 日志 | `log/slog`（JSON/Text 可切） | 标准库、结构化 | zap：额外依赖 |
| Agent | Go，同一仓库 `cmd/agent` | 复用代码、单文件、跨平台 | Python/PS1：依赖运行时、权限控制弱 |
| Agent 通信 | 双向 TLS（自签 + 指纹固定）+ Bearer Token | 局域网内也防嗅探/中间人 | 纯 HTTP：内网可被横向利用 |
| 安卓 | Kotlin 2.x + Jetpack Compose (Material 3) | 现代、声明式、主题切换成本低 | XML View：主题/状态管理成本高 |
| 安卓网络 | Retrofit + OkHttp + kotlinx.serialization | 成熟、支持 SSE、拦截器做刷新 | Ktor：生态略小 |
| 安卓 DI | 手动 `AppContainer` | 无注解处理器，构建更快、包体更小 | Hilt：本期依赖注入规模不需要 |
| 安卓存储 | DataStore + Android Keystore（AES-GCM） | 令牌加密、设置持久化 | Room：本期无复杂本地查询需求 |
| 推送 | SSE（前台）+ 轮询兜底 | 无需 FCM，零外部依赖 | FCM：需 Google 服务与隐私合规 |
| 镜像 | `gcr.io/distroless/static` 或 `scratch` + CA | 最小体积与攻击面 | Alpine：多 ~8 MB 与包管理面 |

---

## 3. 后端设计

### 3.1 目录结构

```
wakecomputer/
├─ cmd/
│  ├─ server/main.go          # 服务端入口 + admin/backup 子命令
│  └─ agent/main.go           # Agent 入口（Windows 服务 / Linux daemon）
├─ internal/
│  ├─ config/                 # 环境变量解析与校验
│  ├─ httpapi/                # 路由、中间件、handler、SSE
│  ├─ auth/                   # JWT、Argon2id、限流、会话
│  ├─ host/                   # 主机与分组领域逻辑
│  ├─ wol/                    # 魔术包构造与多目标发送
│  ├─ status/                 # 心跳表、TCP/ICMP 探测器、状态机
│  ├─ job/                    # 批量任务引擎（信号量、取消、事件）
│  ├─ agentapi/               # 注册/心跳/指令下发
│  ├─ store/                  # SQLite、迁移、查询
│  ├─ audit/                  # 审计写入与查询
│  └─ version/
├─ migrations/*.sql           # embed
├─ deploy/
│  ├─ docker-compose.yml
│  ├─ .env.example
│  ├─ nginx.conf.example
│  ├─ Caddyfile.example
│  └─ openwrt-install.sh
└─ android/                   # 独立 Gradle 工程
```

### 3.2 配置项（全部环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `WC_HTTP_ADDR` | `0.0.0.0:8085` | 监听地址（host 网络下建议填 LAN IP） |
| `WC_DATA_DIR` | `/data` | SQLite 与备份目录 |
| `WC_PUBLIC_URL` | 空 | 生成 App 里的 Agent 安装命令。为空时用当前请求的 Host。接入只允许局域网，部署时填 `http://<路由器IP>:8085`，不要填公网域名 |
| `WC_TRUSTED_PROXIES` | 空 | CIDR 列表，仅信任这些来源的 `X-Forwarded-For` |
| `WC_JWT_SECRET` | 自动生成并存 `/data/secret` | 也可显式注入 |
| `WC_ACCESS_TTL` | `15m` | 访问令牌有效期 |
| `WC_REFRESH_TTL` | `720h` | 刷新令牌有效期 |
| `WC_LOG_LEVEL` | `info` | debug/info/warn/error |
| `WC_LOG_FORMAT` | `text` | text/json |
| `WC_WOL_DEFAULT_BROADCAST` | `255.255.255.255` | 默认广播地址 |
| `WC_WOL_PORT` | `9` | 默认 WoL 端口 |
| `WC_ENABLE_ICMP` | `false` | 开启需 `NET_RAW` 与 root |
| `WC_METRICS` | `false` | 暴露 `/metrics`（P2） |

### 3.3 WoL 实现

```go
func MagicPacket(mac net.HardwareAddr) []byte {
    p := make([]byte, 6+16*len(mac))
    for i := 0; i < 6; i++ { p[i] = 0xFF }
    for i := 0; i < 16; i++ { copy(p[6+i*len(mac):], mac) }
    return p
}
```

发送策略：

1. 目标集合 = 主机的 `broadcast_targets`（默认注入全局默认广播）+ 可选单播 `host.ip`。
2. 每个目标发送 3 个包，间隔 100 ms；整轮失败（`sendto` 出错）重试 2 次，指数退避 500 ms/1 s。
3. `net.DialUDP` 前设置 `SO_BROADCAST`（Go 的 `net.ListenUDP` + `WriteToUDP` 在 Linux 下默认可广播，需确保 `SO_BROADCAST`；实现中用 `golang.org/x/sys/unix` 设置）。
4. 源端口随机，目的端口默认 9（兼容 7）。
5. 全程 ≤ 2 s，不阻塞 API；发送结果（每个目标的成功/失败）写入审计与响应。

> **为什么必须 host 网络**：Docker 默认 bridge 下的广播包只会在 bridge 内泛洪，无法到达 `br-lan`。定向广播 `192.168.1.255` 经 NAT 出去时通常被内核/交换机丢弃。`macvlan` 在 OpenWrt（常含 Wi-Fi 网桥）上不稳定。`network_mode: host` 让容器直接使用 OpenWrt 的网络命名空间，`br-lan` 上的广播天然可达，是本场景最稳妥方案。

### 3.4 状态引擎

```
每台主机状态判定（后台每 10 s 一轮 + 操作后即时触发）：

if now - last_heartbeat <= 45s                         -> online
else if TCP connect(ip:probe_port) 成功 (timeout 1.5s)  -> online_no_agent
else if ICMP echo 成功（可选）                           -> online_no_agent
else if 唤醒任务进行中且未超时                            -> waking
else if 曾有过状态                                        -> offline
else                                                     -> unknown
```

- 心跳表常驻内存 `map[hostID]heartbeat`，落库节流为每 60 s 一次，减少写放大。
- TCP 探测并发上限 16，单轮总预算 3 s；失败不立即判离线，需连续 2 轮失败（避免瞬时抖动）。
- 状态变化时写 `host.status` 事件到 SSE Hub（环形缓冲，慢客户端丢弃旧事件）。
- 服务重启后从 `hosts.last_status/last_seen_at` 恢复，未知主机先标 `unknown`。

### 3.5 Agent 设计

**接入（Enrollment）**

1. App 创建主机时选择"安装 Agent"，服务端生成 8 位一次性接入码（Base32，10 min TTL，存 `enrollment_codes`）。
2. 主机必须在局域网执行安装命令，`--server` 填服务端内网地址。`POST /agent/v1/enroll` 只接受私网、回环或链路本地来源；若请求经过受信任反代，`X-Forwarded-For` 里的客户端也必须是内网地址，因此公网域名不能用来绑定。
   - Linux：`curl -fsSL http://192.168.1.3:8085/install.sh | sudo bash -s -- --server http://192.168.1.3:8085 --code ABCD1234`
   - Windows：`powershell -NoProfile -ExecutionPolicy Bypass -c "irm http://192.168.1.3:8085/install.ps1 -OutFile $env:TEMP\wake-agent-install.ps1; & $env:TEMP\wake-agent-install.ps1 -Server http://192.168.1.3:8085 -Code ABCD1234"`
3. Agent 生成自签证书与私钥（存放于配置目录，权限 0600），调用 `POST /agent/v1/enroll`：
   ```json
   { "code":"ABCD1234", "hostname":"desktop", "os":"windows", "arch":"amd64",
     "version":"0.1.0", "certPem":"...", "fingerprint":"sha256:...",
     "ips":["192.168.1.20","172.17.0.1"] }
   ```
   `ips[0]` 是 Agent 到服务器的出口网卡地址，服务端优先选与自身同 /24 的地址，其次选私网地址，避免把 Lucky 回源 IP 当成 Agent 地址。
4. 服务端校验接入码 → 绑定到主机 → 返回 `hostId` + `agentToken`（只返回一次；服务端用 AES-GCM 加密存储，供后续主动下发指令）。
5. Agent 写入配置并注册为服务，立即发送首次心跳。Windows 服务为自动启动，失败后按 5s / 15s / 60s 重启。
6. 管理员改密码、重置密码、重建容器都不会清除 `agent_token_enc` 和证书指纹。只要 `/data` 还在，已绑定的 Agent 会自己重连，不需要在主机上重新安装。

> 证书指纹由 Agent 在注册时一并上报并入库；此后服务端每次调用 Agent 都校验对端证书指纹（`tls.Config.VerifyPeerCertificate`），实现"首次接入即固定"。

**接口（Agent 侧，监听 `:45871`）**

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/health` | 返回版本、运行时长（需 Bearer） |
| POST | `/v1/command` | `{id, action:"shutdown"\|"reboot"\|"cancel", delaySeconds, reason}`，返回 `202 {accepted, executeAt}` |

**指令语义**

| 平台 | shutdown | reboot | cancel |
|---|---|---|---|
| Windows | `shutdown.exe /s /f /t N /d p:0:0 /c "Remote shutdown"` | `/r /f /t N` | `shutdown.exe /a` |
| Linux | `systemctl poweroff`（失败回退 `shutdown -h now`） | `systemctl reboot` | `shutdown -c` |

- 延迟由 Agent 本地定时执行，服务端只负责下发与记录 `executeAt`，避免服务端重启导致指令丢失。
- 幂等：Agent 内存保存最近 100 个 `commandId`，重复请求直接返回 `200 {duplicate:true}`。
- 关机前先回 `202`，再异步执行（延迟 ≥ 5 s），保证 HTTP 响应能返回。
- Agent 以 SYSTEM/root 运行；仅监听内网地址；安装脚本自动添加防火墙规则（Windows 仅允许服务端 IP，Linux 用 nftables/iptables 或提示手动配置）。
- Agent 不主动外连（除心跳），不执行任意命令，不接受参数化 shell。

**心跳**

```http
POST /agent/v1/heartbeat
Authorization: Bearer <agentToken>
{ "hostId":"...", "version":"0.1.0", "uptimeSec":12345, "ips":["192.168.1.20"] }
```
服务端返回 `{ "serverTime": "...", "status":"ok" }`。心跳里的私网 IP 会更新 `agent_url`，DHCP 变更后不用重新绑定。失败时 Agent 退避重试（15s 起，上限 2 min）并写本地日志，不会因此解除绑定。

### 3.6 认证与安全

| 项 | 实现 |
|---|---|
| 登录 | `POST /api/v1/auth/login`，Argon2id 校验，恒定时间比较 |
| 访问令牌 | JWT HS256，`sub=userId`，`jti`，15 min；`Authorization: Bearer` |
| 刷新令牌 | 32B 随机，SHA-256 入库，30 天，旋转（旧令牌立即失效）；重放检测 → 吊销该设备全部会话 |
| 存储 | Android 侧用 Keystore AES-GCM 加密后写入 DataStore；服务端只存哈希 |
| 客户端会话 | 启动时用刷新令牌恢复会话。只有服务端明确返回 `401/403` 才清除登录态；超时、DNS 失败、5xx 都保留会话，进入缓存视图并等待自动重连 |
| 记住密码 | 登录页可勾选，默认开启。密码用 Keystore AES-GCM 加密后存在 DataStore，与刷新令牌分开；退出登录不清除 |
| 限流 | 进程内令牌桶：登录 5/min/IP + 20/h/账号；全局 600/min/IP；批量 10/min |
| 锁定 | 连续 5 次失败锁 15 min，审计记录 |
| 传输 | 反代终止 TLS（TLS1.2+），HSTS；容器内 HTTP 仅监听内网 |
| Agent | mTLS + 指纹固定 + 每主机独立 Token |
| 审计 | 所有写操作同步落库（失败也记录），异步不影响响应 |
| 敏感信息 | 日志脱敏：`password`、`token`、`authorization`、`totp` |
| 首启 | 数据库还没有用户时自动创建 `admin`。`WC_ADMIN_PASSWORD` 有值就用它，否则随机生成并打印到日志。改这个变量不会改已有账号 |

**威胁与对策（简版 STRIDE）**

| 威胁 | 对策 |
|---|---|
| 公网暴力破解 | 限流 + 锁定 + 强密码策略 + 可选 TOTP |
| 令牌泄露 | 短 TTL + 刷新旋转 + 设备级吊销 + Android Keystore |
| 内网中间人（Agent） | mTLS + 证书指纹固定 |
| 恶意 App 调用 | 鉴权 + 审计 + 无 CORS 通配 |
| 重放关机指令 | commandId 幂等 + TLS + 时间戳校验 |
| 容器逃逸影响现有服务 | 非 root、只读根文件系统、cap_drop ALL、无 Docker socket 挂载 |
| 数据丢失 | SQLite WAL + 每日备份 + 备份校验 |

### 3.7 批量任务引擎

```go
type Job struct {
    ID string; Type string; Status string
    HostIDs []string; DelaySeconds int
    Results map[string]Result   // queued|running|ok|failed|canceled + error
    CreatedAt, FinishedAt time.Time
}
```

- 创建任务后立即返回 `202 {jobId}`，后台 goroutine 执行。
- `semaphore(concurrency=4)` 控制并发，主机之间可插入 `jitter`（0~2 s）。
- 关机类任务在真正下发前插入 **10 s 缓冲期**，期间 `POST /jobs/{id}/cancel` 可整体取消。
- 每台主机结果独立，`partial` 状态表示部分成功。
- 任务保存在内存 + 完成后摘要写入 `audit_logs`；任务记录 TTL 30 min（App 足够读取），服务重启后未完成任务标记 `interrupted`。
- SSE 事件：`job.update`（进度）、`job.done`（完成）。

> P1 可将任务与延迟落库，实现服务重启后续跑；v1 明确不支持。

### 3.8 SSE

```http
GET /api/v1/events
Authorization: Bearer <token>
Accept: text/event-stream
```
事件类型：`hello`、`host.status`、`job.update`、`job.done`；每 20 s 发送 `: ping` 保活。Lucky 的 Go 反代默认按 chunk 转发，SSE 一般可直接工作；若实测被缓冲（事件成批到达），客户端自动降级为 5 s 轮询，不影响功能。客户端断线 3 s 后重连，并用 `Last-Event-ID`（单调递增序号）补发最近 100 条事件。

---

## 4. 数据模型

```sql
CREATE TABLE users (
  id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL, totp_secret TEXT,
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY, user_id TEXT NOT NULL, refresh_hash TEXT NOT NULL,
  device_name TEXT, user_agent TEXT, ip TEXT,
  created_at INTEGER NOT NULL, last_used_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL, revoked_at INTEGER,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_sessions_user ON sessions(user_id, revoked_at);

CREATE TABLE hosts (
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE,
  os_type TEXT NOT NULL CHECK(os_type IN ('windows','linux','other')),
  mac TEXT NOT NULL, ip TEXT,
  broadcast_targets TEXT NOT NULL DEFAULT '[]',  -- JSON array
  wol_port INTEGER NOT NULL DEFAULT 9,
  probe_port INTEGER,                            -- 3389/22/445...
  notes TEXT, sort_order INTEGER NOT NULL DEFAULT 0,
  agent_url TEXT, agent_token_hash TEXT, agent_fingerprint TEXT,
  agent_version TEXT, last_seen_at INTEGER, last_status TEXT NOT NULL DEFAULT 'unknown',
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);

CREATE TABLE groups (
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE host_groups (
  host_id TEXT NOT NULL, group_id TEXT NOT NULL,
  PRIMARY KEY(host_id, group_id),
  FOREIGN KEY(host_id) REFERENCES hosts(id) ON DELETE CASCADE,
  FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
);

CREATE TABLE enrollment_codes (
  code_hash TEXT PRIMARY KEY, host_id TEXT NOT NULL,
  expires_at INTEGER NOT NULL, used_at INTEGER
);

CREATE TABLE audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT, action TEXT NOT NULL, host_id TEXT,
  result TEXT NOT NULL, detail TEXT, ip TEXT, user_agent TEXT,
  created_at INTEGER NOT NULL
);
CREATE INDEX idx_audit_time ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_host ON audit_logs(host_id, created_at DESC);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
```

约定：
- 时间统一 Unix 毫秒（INTEGER），API 输出 ISO-8601。
- SQLite：`journal_mode=WAL`、`synchronous=NORMAL`、`busy_timeout=5000`、`foreign_keys=ON`。
- 单连接写 + 连接池读（`SetMaxOpenConns` 读 4 / 写 1，或统一 1 写 4 读的 DSN 双池）。
- 迁移在启动时串行执行，失败即退出（配合 `restart: unless-stopped` 可见日志）。

---

## 5. API 规范

- Base：`/api/v1`；请求/响应 `application/json`；错误统一：
  ```json
  { "error": { "code": "HOST_NOT_FOUND", "message": "主机不存在", "details": {} } }
  ```
- 鉴权：除 `/healthz`、`/api/v1/auth/login`、`/api/v1/auth/refresh`、`/agent/v1/enroll` 外均需 Bearer。
- 时间：ISO-8601 带时区；ID：ULID 字符串。
- 幂等：写操作支持 `Idempotency-Key` 头（10 s 去重）。

### 5.1 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | `{status, version, db:"ok", uptime}` |
| POST | `/api/v1/auth/login` | `{username,password,totp?}` → `{accessToken,refreshToken,expiresIn,user}` |
| POST | `/api/v1/auth/refresh` | `{refreshToken}` → 新令牌对 |
| POST | `/api/v1/auth/logout` | 吊销当前刷新令牌 |
| GET | `/api/v1/auth/me` | 当前用户与设置 |
| POST | `/api/v1/auth/password` | `{oldPassword,newPassword}` |
| GET | `/api/v1/hosts` | 主机 + 分组 + 状态列表 |
| POST | `/api/v1/hosts` | 创建主机 |
| GET/PATCH/DELETE | `/api/v1/hosts/{id}` | 详情/更新/删除 |
| POST | `/api/v1/hosts/{id}/wake` | `{waitSeconds?:120}` → `{accepted, sentTargets}` |
| POST | `/api/v1/hosts/{id}/power` | `{action:"shutdown"\|"reboot"\|"cancel", delaySeconds:30}` |
| POST | `/api/v1/hosts/{id}/probe` | 立即探测并返回状态 |
| POST | `/api/v1/hosts/{id}/agent/enroll-code` | 生成接入码；安装命令的服务器地址取 `WC_PUBLIC_URL` |
| DELETE | `/api/v1/hosts/{id}/agent` | 解绑 Agent |
| GET/POST | `/api/v1/groups`；PATCH/DELETE `/api/v1/groups/{id}` | 分组 CRUD |
| POST | `/api/v1/batch` | `{hostIds:[], action, delaySeconds, concurrency?}` → `{jobId}` |
| GET | `/api/v1/jobs/{id}` | 任务与逐台结果 |
| POST | `/api/v1/jobs/{id}/cancel` | 取消未执行项 |
| GET | `/api/v1/audit?limit=50&cursor=&hostId=&action=&from=&to=` | 审计分页 |
| GET/PUT | `/api/v1/settings` | 全局设置（WoL 默认值、并发、保留天数） |
| GET | `/api/v1/events` | SSE |
| POST | `/agent/v1/enroll` | Agent 接入（接入码 + 仅局域网） |
| POST | `/agent/v1/heartbeat` | Agent 心跳（Bearer） |

### 5.2 示例

```http
POST /api/v1/hosts/01H.../wake
Authorization: Bearer eyJ...
```
```json
{
  "accepted": true,
  "targets": [
    {"address":"255.255.255.255:9","ok":true},
    {"address":"192.168.1.255:9","ok":true}
  ],
  "status": "waking",
  "etaSeconds": 60
}
```

```json
// GET /api/v1/hosts
{
  "items": [
    {
      "id":"01H...", "name":"书房主机", "osType":"windows",
      "mac":"AA:BB:CC:DD:EE:FF", "ip":"192.168.1.20",
      "groups":[{"id":"01G...","name":"办公"}],
      "status":"online", "lastSeenAt":"2026-10-17T09:21:33+08:00",
      "agent":{"version":"0.1.0","fingerprint":"sha256:ab..","lastHeartbeatSec":8},
      "capabilities":{"wake":true,"shutdown":true,"reboot":true,"cancel":true}
    }
  ],
  "serverTime":"2026-10-17T09:21:41+08:00"
}
```

---

## 6. Android 设计

### 6.1 技术栈

| 项 | 选型 |
|---|---|
| 语言/UI | Kotlin 2.x + Jetpack Compose + Material 3 |
| 最低版本 | minSdk 26（Android 8.0），targetSdk 按发布时最新稳定版（当前 36） |
| 架构 | 单模块 + 分包（`data` / `domain` / `ui`），MVVM + 单向数据流 |
| 状态 | `StateFlow<UiState>`，`collectAsStateWithLifecycle` |
| 网络 | Retrofit 2 + OkHttp + kotlinx.serialization + `okhttp-sse` |
| DI | Hilt（KSP） |
| 存储 | DataStore Preferences + Android Keystore AES-GCM（令牌） |
| 导航 | Navigation-Compose（类型安全路由） |
| 图片/图标 | Material Symbols（无图片库，控制包体） |
| 测试 | JUnit5 + Turbine + MockWebServer + Compose UI Test |
| 包体目标 | release APK ≤ 12 MB（R8 + resource shrink） |

### 6.2 模块与页面

```
ui/
├─ theme/        Color.kt, Type.kt, Theme.kt (Light/Dark schemes)
├─ login/        LoginScreen + ServerConfigSheet
├─ hosts/        HostListScreen, HostDetailScreen, HostEditScreen
├─ groups/       GroupManageScreen
├─ batch/        BatchSheet, BatchProgressScreen
├─ audit/        AuditScreen
└─ settings/     SettingsScreen, ThemePicker, DiagnosticsScreen, AboutScreen
```

| 页面 | 要点 |
|---|---|
| 登录 | 服务器地址、用户名、密码、可选 TOTP；错误分类提示（DNS/TLS/401/超时） |
| 主机列表 | 顶栏状态汇总（在线 x / 离线 y）、分组吸顶、搜索、下拉刷新、长按多选、FAB 添加 |
| 主机详情 | 大状态卡 + 主操作按钮（唤醒/关机/重启/取消）、信息卡、Agent 卡、最近操作 |
| 主机编辑 | 表单校验（MAC/IP/端口/广播地址）、"安装 Agent"引导含可复制命令 |
| 批量执行 | 底部抽屉确认 → 进度页（环形进度 + 逐台列表）→ 失败重试 |
| 审计 | 筛选条 + 分页列表 + 详情展开 |
| 设置 | 主题三态、服务器、超时、通知、诊断、关于、退出 |

### 6.3 主题（深色/浅色）

```kotlin
enum class ThemeMode { SYSTEM, LIGHT, DARK }

@Composable
fun WakeTheme(mode: ThemeMode, content: @Composable () -> Unit) {
    val dark = when (mode) {
        ThemeMode.SYSTEM -> isSystemInDarkTheme()
        ThemeMode.LIGHT -> false
        ThemeMode.DARK -> true
    }
    MaterialTheme(
        colorScheme = if (dark) DarkColors else LightColors,
        typography = WakeTypography,
        content = content
    )
}
```

- 品牌主色：深青蓝（seed `#0E7490`）；状态色语义化：
  - 在线 `#16A34A`（浅色 `#15803D`）/ 离线 `#94A3B8` / 唤醒中 `#D97706` / 失败 `#DC2626`
- 深色背景 `#0B1220`，卡片 `#111A2B`，避免纯黑（P2 提供 AMOLED 纯黑）。
- 不使用动态取色（Dynamic Color），保证品牌与状态色语义一致；可在 P2 提供开关。
- 状态不以颜色为唯一信息载体：同时显示文案 + 图标（无障碍要求）。
- 启动时在 `setContent` 前读取主题偏好，避免闪白；启用 `enableEdgeToEdge()` 并按需设置状态栏图标明暗。

### 6.4 关键实现要点

- **令牌刷新**：OkHttp `Authenticator` 同步刷新，使用 `Mutex` 防止并发刷新风暴；刷新失败清除令牌并跳转登录。
- **证书**：默认走系统信任；自签场景支持导入 CA（`network_security_config` 运行时不可改，故使用 OkHttp 自定义 `X509TrustManager` + 用户选择的 CA 文件）。
- **SSE**：v1 采用前台 5 s 轮询（`HostListViewModel.startPolling`），SSE 端点已实现并保留给 v1.1 客户端接入；`onStop` 停止轮询。
- **离线缓存**：最近一次 hosts 响应写入 DataStore（JSON），启动先渲染缓存（带"缓存于 xx"标记），再请求网络。
- **批量进度**：任务页前台时用 SSE，后台时 WorkManager 轮询（P1）；完成发本地通知。
- **错误映射**：网络层统一 `AppError`（网络/认证/服务端/校验/未知），UI 统一 Snackbar + 可重试。
- **安全**：`FLAG_SECURE` 可选（P2）；日志不打印令牌；R8 保留序列化模型。

### 6.5 目录结构

```
android/app/src/main/java/com/wakecomputer/
├─ WakeApp.kt, MainActivity.kt
├─ core/  (Result, AppError, Clock, DispatcherProvider)
├─ data/  (api/, dto/, repo/, local/, token/)
├─ domain/ (model/, usecase/)
└─ ui/    (theme/, components/, login/, hosts/, groups/, batch/, audit/, settings/)
```

---

## 7. 主机侧配置要求（交付文档的一部分）

### 7.1 Windows

| 项 | 设置 |
|---|---|
| BIOS/UEFI | 开启 `Wake on LAN` / `Power On by PCI-E` / `Resume by LAN`；关闭 `ErP`/`Deep Sleep`（否则 S5 断电） |
| 网卡驱动 | 高级 → `Wake on Magic Packet` = Enabled；`Shutdown Wake-On-Lan` = Enabled（Realtek 常见） |
| 电源管理 | 取消"允许计算机关闭此设备以节约电源"（部分机型需保留，实测决定） |
| 快速启动 | 关闭（`powercfg /h off` 或控制面板取消"启用快速启动"），否则 S5 实际为休眠、WoL 行为不一致 |
| 网络 | 建议 DHCP 静态绑定，保证 IP 不变 |
| Agent | 服务以 LocalSystem 运行，防火墙入站仅允许服务端 IP 访问 45871 |

### 7.2 Linux

```bash
# 查询与开启 WoL
sudo ethtool eth0 | grep -i wake-on
sudo ethtool -s eth0 wol g
# 持久化（systemd .link）
# /etc/systemd/network/10-eth0.link
[Match]
OriginalName=eth0
[Link]
WakeOnLan=magic
```
NetworkManager 用户可用 `nmcli c modify <conn> 802-3-ethernet.wake-on-lan magic`。
关机前确保 `systemctl poweroff` 可用；Agent 以 root 运行，仅监听内网。

### 7.3 无 Agent 主机

仅需 MAC + 广播地址。能力矩阵：唤醒 ✅、状态（TCP/ICMP）✅、关机 ❌、重启 ❌。

---

## 8. 部署方案（OpenWrt）

### 8.1 前置检查（脚本自动执行）

```bash
uname -m                      # 需 x86_64（已确认）
docker version                # 需 20.10+
docker compose version        # 需 v2（无则可改用 docker run）
df -h /overlay /mnt/*         # 数据目录需 ext4 且 ≥ 200 MB
free -m                       # 可用内存 ≥ 128 MB
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'  # 部署前快照
```

> 已确认目标设备为 **x86_64**，发布 `linux/amd64` 镜像；CI 同时产出 `linux/arm64` 以便日后迁移到 ARM 小主机。

### 8.2 目录与文件

```
/opt/wakecomputer/
├─ docker-compose.yml
├─ .env
└─ data/            # wake.db, secret, backups/  (ext4 挂载点)
```

### 8.3 docker-compose.yml

```yaml
name: wakecomputer

services:
  wakecomputer:
    image: ghcr.io/<owner>/wakecomputer:0.1.0
    container_name: wakecomputer
    restart: unless-stopped
    network_mode: host          # 关键：WoL 广播可达 br-lan
    read_only: true
    user: "1000:1000"           # 与 data 目录属主一致；如需 ICMP 改为 root + cap_add NET_RAW
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    environment:
      TZ: Asia/Shanghai
      WC_HTTP_ADDR: "192.168.1.2:8085"
      WC_DATA_DIR: /data
      WC_PUBLIC_URL: "http://192.168.1.3:8085"   # 安装命令用这个地址；公网域名不能用来绑定 Agent
      WC_TRUSTED_PROXIES: "192.168.1.2/32,172.16.0.0/12"   # Lucky 若为容器，收窄为其网桥网关或容器 IP
      WC_LOG_LEVEL: info
      WC_WOL_DEFAULT_BROADCAST: "192.168.1.255"
      GOMEMLIMIT: "96MiB"
    volumes:
      - ./data:/data
    tmpfs:
      - /tmp:size=16m
    mem_limit: 128m
    cpus: 0.5
    logging:
      driver: json-file
      options: { max-size: "5m", max-file: "3" }
    healthcheck:
      test: ["CMD", "/app/wakecomputer", "healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 10s
```

> `mem_limit`/`cpus` 为 Compose v2 本地模式支持的资源限制；若版本不支持，改用 `deploy.resources.limits`。
> 健康检查使用二进制内置子命令，避免镜像内安装 curl/wget。

### 8.4 HTTPS 反向代理（Lucky，已确认）

现状假设：Lucky 已在 443 终止 TLS，并已为 `sub2api.example.com` 提供服务。

**操作步骤（只新增子规则，不改动现有规则）**

1. 打开 Lucky 管理台（默认 `http://<路由器IP>:16601`，仅内网可访问）。
2. 进入「Web 服务」→ 找到监听 **443** 的现有规则（sub2api 使用的那条）→ 编辑。
3. 在「子规则」中 **新增一条**：
   - 前端域名：`wake.example.com`
   - 后端地址：`http://192.168.1.2:8085`
   - 类型：反向代理；如界面有 WebSocket 开关则开启
4. 证书：父规则证书需覆盖 `wake.example.com`
   - 已有泛域名 `*.example.com` → 直接复用；
   - 单域名证书 → 在 Lucky 的 ACME 中把两个域名加入同一张证书（SAN），或在规则中选择含 wake 域名的证书；
   - **不要**新建第二个监听 443 的 Web 服务规则，会与现有规则端口冲突。
5. 保存并重载规则。无需重启 Lucky，无需改动 sub2api 子规则。

**必须确认的 3 个点**

| 检查项 | 做法 |
|---|---|
| 来源 IP | Lucky 若为 bridge 容器，回源 IP 是 Docker 网关（如 `172.17.0.1`）；`WC_TRUSTED_PROXIES` 需同时包含 `192.168.1.2/32` 与 `172.16.0.0/12`，否则审计里全是网关地址 |
| SSE | Lucky 基于 Go `ReverseProxy`，默认流式转发；若诊断显示事件成批到达，无需改 Lucky，客户端会自动降级为 5 s 轮询 |
| 缓存 | 该规则不要开启缓存；API 全为 JSON，也不需要压缩 |

**验证**

```bash
curl -i  https://wake.example.com/healthz
curl -i  https://wake.example.com/api/v1/hosts        # 期望 401
curl -N  https://wake.example.com/api/v1/events -H "Authorization: Bearer <token>"   # 期望逐条到达
```

**备选（日后更换反代时）**

```nginx
# nginx：新增 server 块，不影响 sub2api
server {
  listen 443 ssl;  http2 on;
  server_name wake.example.com;
  ssl_certificate     /etc/nginx/certs/fullchain.pem;
  ssl_certificate_key /etc/nginx/certs/privkey.pem;
  add_header Strict-Transport-Security "max-age=31536000" always;
  location / {
    proxy_pass http://192.168.1.2:8085;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_read_timeout 3600s;
  }
}
```

```caddyfile
wake.example.com {
    reverse_proxy 192.168.1.2:8085 { flush_interval -1 }
    header Strict-Transport-Security "max-age=31536000"
}
```

> NPM：新建 Proxy Host → `wake.example.com` → `192.168.1.2:8085`，勾选 WebSocket，Advanced 加 `proxy_buffering off;`。
> 若 Lucky 与后端不在同一台设备，将 `WC_HTTP_ADDR` 改为 `0.0.0.0:8085`，并在 OpenWrt 防火墙仅放行 Lucky 的来源 IP。

### 8.5 防火墙与暴露面

- 公网仅 443（既有规则），不新增端口映射。
- 8085 仅内网可达；OpenWrt 默认 WAN 入站拒绝，无需新增规则。
- 45871 仅服务端 → 主机内网通信。
- Lucky 管理台（16601）仅内网可达，不要与 443 一起暴露。
- 可选：在 Lucky 该子规则上配置 IP 白名单/访问控制，进一步缩小暴露面。

### 8.6 初始化与日常运维

```bash
cd /opt/wakecomputer
docker compose up -d
docker compose logs -f wakecomputer          # 获取首次生成的 admin 密码
docker exec -it wakecomputer /app/wakecomputer admin reset-password
docker exec wakecomputer /app/wakecomputer backup /data/backups/$(date +%F).db
# OpenWrt cron: 0 4 * * * docker exec wakecomputer /app/wakecomputer backup /data/backups/daily.db
```

**升级（只影响本项目）**

```bash
docker compose pull && docker compose up -d    # 仅重建 wakecomputer
# 回滚
sed -i 's/:0.2.0/:0.1.0/' docker-compose.yml && docker compose up -d
```

**隔离检查清单（每次变更后执行）**

```bash
docker ps --format '{{.Names}}\t{{.Status}}\t{{.Image}}'
docker volume ls
docker inspect sub2api --format '{{.State.StartedAt}}'
```

- 不执行 `docker system prune` / `docker volume prune` / 无参数的 `docker compose down`。
- 不修改既有 network、不挂载 Docker socket、不复用现有 volume。
- 部署前记录 `docker ps` 快照，部署后逐项比对容器 ID 与启动时间。

### 8.7 资源与存储注意

- 数据目录必须在 ext4 的 USB/SSD；`overlay` 写入会磨损闪存且重启可能丢失。
- SQLite 体积预估：50 台主机 + 180 天审计 ≈ 5~20 MB/年。
- 日志严格轮转（5 MB × 3），避免撑爆路由器存储。
- 关闭容器内时区依赖，统一 UTC 存储、按 `TZ` 展示。

---

## 9. 可观测性

| 项 | 方案 |
|---|---|
| 访问日志 | 方法、路径、状态码、耗时、请求 ID、来源 IP（脱敏） |
| 应用日志 | `slog`，结构化字段 `hostId/jobId/action` |
| 审计日志 | 独立表，App 可查 |
| 指标 | P2：`/metrics`（请求数、延迟直方图、任务成功率、在线主机数） |
| 告警 | P2：心跳连续失败 → 本地通知（App 轮询） |

---

## 10. 测试策略

| 层次 | 内容 |
|---|---|
| 单测 | WoL 包字节正确性、状态机、批量引擎（并发/取消/超时）、认证（令牌旋转/重放）、限流 |
| 集成 | `httptest` + 临时 SQLite + Mock Agent（`httptest.NewTLSServer`）覆盖全 API |
| 契约 | OpenAPI 生成 + Android 侧 DTO 校验（CI 比对） |
| E2E | 真机：Win11 + Ubuntu 各一台，验证唤醒/关机/重启/取消/批量 |
| 网络 | Wireshark 抓包确认魔术包目标与内容；跨 VLAN 定向广播验证 |
| 安全 | 未授权访问矩阵、暴力破解限流、令牌失效、证书指纹错误拒绝、容器权限检查 |
| 兼容 | Android 8/10/13/15/16 模拟器 + 2 台真机；深色/浅色/大字体/横屏 |
| 破坏性 | 关机中断网、服务重启、DB 只读、代理超时、时钟漂移 |
| 验收 | 需求文档 §8 全部 AC |

---

## 11. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| Docker bridge 下广播不可达 | 唤醒全部失败 | 强制 `network_mode: host`；部署自检发送测试包 |
| OpenWrt 存储不足/写在 overlay | 数据丢失、闪存损坏 | 前置检查 + 拒绝启动并给出提示 |
| Windows S5 唤醒失败 | 核心功能不可用 | 交付 BIOS/NIC 检查清单 + App 内"唤醒失败自助排查" |
| Agent 权限不足 | 关机失败 | 安装为 SYSTEM/root；启动时自检并上报 `capabilities` |
| 反向代理缓冲 SSE | 状态不实时 | 提供三套已验证配置 + 诊断页检测 |
| 动态 IP 变化 | 无法连接 | 推荐 DHCP 静态绑定；支持 mDNS/主机名作为探测目标（P1） |
| 公网暴露被扫描 | 安全风险 | 限流 + 锁定 + 审计 + 建议 TOTP + 反代层额外限制 |
| 批量误关机 | 业务中断 | 二次确认 + 10 s 缓冲 + 取消 + 审计 |
| 服务重启丢失延迟任务 | 指令未执行 | 延迟下发给 Agent 本地执行；未完成批量标记 `interrupted` 并提示 |
| 时区/NTP 偏差 | JWT/TLS 失败 | 依赖 OpenWrt NTP；容器内 UTC；启动检查时钟偏差 > 5 min 告警 |

---

## 12. 演进路线

| 版本 | 内容 |
|---|---|
| v1.0 | 本文档 P0 全部 |
| v1.1 | TOTP、设备管理、桌面小组件/磁贴、通知、诊断页、i18n |
| v1.2 | 分组快捷操作、定时任务、可用性历史、Agent 自动升级 |
| v2.0 | 多用户与权限、Web 管理台、多网段中继、场景编排 |
