# Woops

出站 Agent 统一运维通道：多机房 / 网闸后主机只需能访问 Gateway，**不必**先把堡垒打进内网。

Shell · 文件 · RDP/VNC · 端口映射 · 轻量监控 · 审计回放 · GitLab CI（woopsctl）走**同一条 Agent 通道**。
控制面 Java，数据面 Go Gateway/Agent，控制台 Vue 3。

功能与架构约定见 [`FEATURES.md`](./FEATURES.md)。许可证：[Apache License 2.0](./LICENSE)。

## 和 JumpServer / Teleport 的差别（一句话）

| | 传统堡垒（如 JumpServer） | Woops |
|--|--------------------------|--------|
| 网络假设 | 中心能连到资产（不通就要 FRP/跳板） | **资产 Agent 出站**连 Gateway |
| 目标 | 会话与权限 | 会话 + 轻监控 + **CI 复用同一通道** |
| 目标侧 | 通常依赖 sshd 等 | Agent 原生 Shell/文件（不依赖 sshd） |

Teleport 也是出站隧道模型，偏零信任大平台；Woops 更偏「少装几套、一键装完能干活」的运维面板。

## 网闸与多级内网（Agent 自带受限 proxy）

传统堡垒要「中心能拨到资产」；Woops 反过来：**每台资产上的 Agent 只出站**连 Gateway（HTTPS / WSS）。防火墙可以挡住从公网打进内网，只要内网里有一台机器能摸到 Gateway（或摸到上一跳代理），更深的机器也能上线。

**不必另装 Squid / nginx 正向代理。** Agent **自带**可选的入站 HTTP 正向代理插件（`agent.yaml` → `proxy.*`），与管控共用一个二进制；默认关闭，打开后仍是**受限**代理，不是开放上网出口：

| 限制 | 说明 |
|------|------|
| 账密强制 | 必须配置 `username` / `password`，无匿名代理 |
| 来源 CIDR | `allowCIDRs` 白名单，只有指定网段能连上代理端口 |
| 默认只通 ops | `allowGlobal=false`（默认）时，出站目标仅限 Gateway 等 ops 主机（`gateway` 主机及其端口 / `:9100`），**不能**当通用翻墙或任意网站代理 |
| 可选放开 | 仅当明确设 `allowGlobal=true` 才允许更广目标；仍拒绝 loopback / 云元数据等危险地址 |
| 串联防环 | 本机若已有 `gatewayProxy`，入站流量会经上游再出站，并拒绝连回自身代理，避免环路 |

典型拓扑：

```text
  ┌─ 公网 / DMZ ─┐         防火墙          ┌──────── 内网 ────────┐
  │  Woops       │  ←———— firewall ————→  │  跳板机 A            │
  │  Gateway     │                        │  Agent（开受限 proxy）│
  │  Console …   │                        │         │            │
  └──────────────┘                        │         │ proxy      │
                                          │         ▼            │
                                          │  业务机 B / C …      │
                                          │  Agent（经 A 出站）  │
                                          └──────────────────────┘
```

用一句话对照常见场景：

`互联网上的 Woops（Gateway） ←firewall→ 内网跳板（Agent 自带受限 proxy） ←proxy→ 更深内网（Agent）`

| 角色 | 做什么 |
|------|--------|
| **Gateway（互联网侧）** | 公网可达的 `https`/`wss`；Agent / 安装脚本都连这里 |
| **跳板机 A（能出网或能到 Gateway）** | 正常安装 Agent；在 `agent.yaml` 打开入站 **`proxy.*`**（账密 + `allowCIDRs`；默认只转发到 ops） |
| **更深内网 B** | 安装前设 `https_proxy`/`http_proxy` 指向 **A 的代理**；脚本写入 `gatewayProxy`。之后 B→Gateway 的 WSS 经 A 的受限 proxy 出站 |
| **再深一层 C（只达 B）** | 同理：`https_proxy` 指向 **B**；B 若同时开了 `proxy.*`，且自己带 `gatewayProxy=A`，则链路为 **C → B → A → Gateway** |

要点：

- **不用**再为每层网闸单独搭 FRP / SSH / 第三方正向代理；跳板就是已纳入管控、并打开**受限 proxy** 的 Agent。
- Agent **只出站**，不要求 Gateway 能主动拨进内网。
- 安装与日常上线认同一套代理环境变量；控制台「生成安装链接」弹窗里有 `https_proxy` 写法与密码编码说明。
- 配置示例见 [`go/agent.example.yaml`](./go/agent.example.yaml)（`gatewayProxy`、`proxy.*`）。

## 界面预览

截图在 [`docs/screenshots/`](./docs/screenshots/)（IP 等已打码）。

| 首页 · 异常资产与分组 | 资产列表 · 分组树与会话入口 |
|:---:|:---:|
| ![首页](docs/screenshots/home.png) | ![资产](docs/screenshots/assets.png) |

| Shell（Agent 原生，不依赖 sshd） | 文件管理 |
|:---:|:---:|
| ![Shell](docs/screenshots/shell.png) | ![文件](docs/screenshots/files.png) |

| 资产监控 | 操作审计 · 会话回放入口 |
|:---:|:---:|
| ![监控](docs/screenshots/monitor.png) | ![审计](docs/screenshots/audit.png) |

| 资产设置 · 桌面凭据 / 端口映射 / 部署 Token |
|:---:|
| ![资产设置](docs/screenshots/asset-settings.png) |

## 快速开始

### 0) 环境变量（必做）

```powershell
# 仓库根目录
copy .env.example .env
# 按本机改 PUBLIC 地址；生产务必改 JWT / Ticket / 管理员密码
```

`.env` / `.env.local` **不要提交**。生产可参考 [`deploy/env.prod.example`](./deploy/env.prod.example)。

### 1) 前置

JDK 21、Maven、Go 1.22+、Node 20+、Docker（Postgres / 可选 guacd）、OpenSSL（生成自签证书时用）。

### 1.5) Gateway TLS 证书（克隆后需自己生成，编译不会自动出）

`deploy/tls/` **已 gitignore**，仓库里没有证书；`mvn` / `go build` / `npm` **都不会**生成它。

| 场景 | 是否需要 |
|------|----------|
| 只编译二进制 / 只跑 control-api + Postgres | 可不做 |
| 本机起 Gateway（Agent 走 `wss`）或 Docker 全栈 | **必须** |
| 本机 Vite Console | 有 `deploy/tls/gateway.*` 时才启用 HTTPS；没有也能起，但是 HTTP |

自签示例（Linux / macOS / Git Bash；需本机有 `openssl`）：

```bash
# 仓库根目录；按你的访问地址改 --host / --ip（可重复）
./deploy/gen-gateway-tls.sh --host 127.0.0.1 --ip 127.0.0.1
```

脚本写入 `deploy/tls/gateway.crt` + `gateway.key`，并打印 `OPS_GATEWAY_TLS_SPKI_SHA256=...`。把该 pin 与证书绝对路径写入 `.env`（变量名见 [`.env.example`](./.env.example)）。已有公有 CA 证书时，把文件放进 `deploy/tls/`（或自定路径）并算同一 pin 即可，不必用自签脚本。

### 2) 编译

```powershell
cd go
go build -trimpath -o bin\gateway.exe ./cmd/gateway
go build -trimpath -o bin\woopsctl.exe ./cmd/woopsctl
.\scripts\build-agent-windows.ps1
# Linux Agent：见 go/scripts/build-agent-linux.sh（勿在 Windows 上交叉当生产包）

cd ..\apps\control-api
mvn -DskipTests package

cd ..\console
npm install
npm run build
```

### 3) 启动（本机进程 + Docker 只跑库）

先完成 **§0**；若要起 Gateway / Agent，再完成 **§1.5**。

```powershell
cd deploy
docker compose up -d postgres
# 需要远程桌面时：
docker compose --profile desktop up -d guacd

cd ..
. .\scripts\load-env.ps1

# 终端 A — control-api
cd apps\control-api
mvn -DskipTests spring-boot:run

# 终端 B — gateway（在 go/ 下；需 .env 里 TLS 路径与 pin）
cd go
.\bin\gateway.exe

# 终端 C — console
cd apps\console
npm run dev
```

浏览器打开 `https://127.0.0.1:5173`（已按 §1.5 放好证书时；自签需信任一次）。  
默认账号：`admin` / `admin123`（未启用 GitLab 时；首次登录须绑定 TOTP）。**上线前务必改密码与密钥。**

本机覆盖可用 `.env.local`（gitignore）。加载顺序：`.env` → `.env.local`。

### 4) 接入一台主机（Agent）

两条路：**方法 1** 给真实/虚拟机装服务（推荐）；**方法 2** 只适合在本仓库旁调试 Agent。

#### 方法 1 — 控制台安装码（推荐）

1. 本机 **Gateway + control-api** 已按 §3 跑着，且 §1.5 的 pin 已写入 `.env`（安装脚本/Agent 靠 pin 校验证书）。
2. 打开控制台 → **资产** → 左侧先选中目标**分组**（选「全部」不能发码）→ **生成安装链接**。
3. 在弹窗复制对应系统的命令，到目标机执行：
   - **Linux**：`curl … | bash`
   - **Windows**：`curl.exe` 下载 `install.ps1` 再 `powershell -File …`（须管理员；弹窗有无 curl 时的折叠说明）
4. 安装码约 **15 分钟**有效、期内可多次用；过期重新生成。脚本会下载 Agent、向 Gateway 注册，并落盘服务（Linux 优先 `/usr/local/bin/woops-agent`，Windows 服务名 `woops-agent`）。
5. 控制台资产列表出现该主机且为「在线」即成功。重装会保留 `asset-id`、轮换 `agent-token`。

网闸 / 多级内网：在能出网的机器上装 Agent，打开**自带受限**入站 `proxy.*` 作跳板；更深主机安装前设 `https_proxy` 指向该跳板（写入 `gatewayProxy`）。详见上文 **「网闸与多级内网（Agent 自带受限 proxy）」**；控制台安装弹窗也有变量示例。

#### 方法 2 — 本机开发调试（不装成系统服务）

用于改 Agent 代码后前台跑，**不是**生产装机方式。

1. 仍建议先用**方法 1**在某台机装一次，或走控制台注册拿到一对凭据；把 `asset-id`、`agent-token` 放到 `go/` 目录旁（已 gitignore）。
2. 参考 [`go/agent.example.yaml`](./go/agent.example.yaml) 写 `go/agent.local.yaml`（`gateway` + **与 `.env` 相同的** `gatewayTlsSpkiSha256`）。
3. 编译并启动：

```powershell
cd go
.\scripts\build-agent-windows.ps1
.\bin\agent.exe -config agent.local.yaml
```

Linux 用 `go/scripts/build-agent-linux.sh` 后在同机运行对应二进制。

### 5) 全栈 Docker（可选）

```powershell
# 必先做 §1.5：deploy/tls/ 有证书，且 .env 已填 pin（compose 挂载 deploy/tls）
cd deploy
docker compose --env-file ..\.env --profile full --profile desktop up -d --build
```

服务器可将 `deploy/env.prod.example` 复制为 `/opt/ops/.env` 再改域名与密钥；证书仍放服务器本地 `deploy/tls/`（勿提交）。

## 安全提示

- 生产必须更换 `OPS_JWT_SECRET`、`OPS_TICKET_SECRET`、管理员密码。
- Gateway 生产走 `https`/`wss`；自签 / 动态 IP 用 SPKI pin（`OPS_GATEWAY_TLS_SPKI_SHA256`），**不要**关证书校验。
- 不要把真实 `.env`、`deploy/tls/` 私钥、`agent-token` 提交到公开仓库（`deploy/tls/` 已 gitignore）。

## 端口

| 端口 | 用途 |
|------|------|
| 443 | Console（Docker HTTPS；本地 Vite 开发用 5173） |
| 9100 | control-api（宜内网） |
| 9200 | Gateway **PUBLIC**（Agent / woopsctl / 浏览器） |
| 9201 | Gateway **INTERNAL**（仅 control-api → gateway） |
| 5432 | Postgres |
| 4822 | guacd（desktop profile） |

## 目录

```
FEATURES.md          功能清单与进度（唯一约定源）
.env.example         环境变量模板
LICENSE              Apache-2.0
apps/control-api     Java Spring Boot
apps/console         Vue3 + Element Plus（en/zh）
go/cmd/{gateway,agent,woopsctl}
go/internal/opsctl   woopsctl 内部实现
deploy/              docker-compose、Dockerfile、env.prod.example、gen-gateway-tls.sh
docs/                woopsctl CI 说明、screenshots/ 界面截图
```

## 反馈

欢迎提 Issue（请勿粘贴密钥、安装码、内网拓扑细节）。更细的能力与分期见 [`FEATURES.md`](./FEATURES.md)。
