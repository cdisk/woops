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

## 快速开始

### 0) 环境变量（必做）

```powershell
# 仓库根目录
copy .env.example .env
# 按本机改 PUBLIC 地址；生产务必改 JWT / Ticket / 管理员密码
```

`.env` / `.env.local` **不要提交**。生产可参考 [`deploy/env.prod.example`](./deploy/env.prod.example)。

### 1) 前置

JDK 21、Maven、Go 1.22+、Node 20+、Docker（Postgres / 可选 guacd）。

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

# 终端 B — gateway（在 go/ 下）
cd go
.\bin\gateway.exe

# 终端 C — console
cd apps\console
npm run dev
```

浏览器打开 `https://127.0.0.1:5173`（自签证书需信任一次）。  
默认账号：`admin` / `admin123`（未启用 GitLab 时；首次登录须绑定 TOTP）。**上线前务必改密码与密钥。**

本机覆盖可用 `.env.local`（gitignore）。加载顺序：`.env` → `.env.local`。

### 4) 接入一台主机（Agent）

1. 控制台创建资产 → 复制安装命令完成注册（得到 `asset-id`、`agent-token`）。
2. 凭据放在 `go/` 旁：`asset-id`、`agent-token`（已 gitignore）。
3. 参考 [`go/agent.example.yaml`](./go/agent.example.yaml) 写本机 `agent.local.yaml`（`gateway` + pin）。
4. 启动：`.\bin\agent.exe -config agent.local.yaml`

### 5) 全栈 Docker（可选）

```powershell
# 先准备 TLS：deploy/gen-gateway-tls.sh → deploy/tls/，pin 写入 .env
cd deploy
docker compose --env-file ..\.env --profile full --profile desktop up -d --build
```

服务器可将 `deploy/env.prod.example` 复制为 `/opt/ops/.env` 再改域名与密钥。

## 安全提示

- 生产必须更换 `OPS_JWT_SECRET`、`OPS_TICKET_SECRET`、管理员密码。
- Gateway 生产走 `https`/`wss`；自签场景用 SPKI pin（`OPS_GATEWAY_TLS_SPKI_SHA256`），**不要**关证书校验。
- 不要把真实 `.env`、TLS 私钥、`agent-token` 提交到公开仓库。

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
docs/                如 woopsctl GitLab CI 说明
```

## 反馈

欢迎提 Issue（请勿粘贴密钥、安装码、内网拓扑细节）。更细的能力与分期见 [`FEATURES.md`](./FEATURES.md)。
