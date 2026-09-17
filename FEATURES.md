# Woops — 功能清单与进度

> **本文件是功能、进度与架构约定的唯一清单（自包含）。**  
> 任何功能新增、完成、搁置、行为变更，都必须先读本文件，并在同一变更中更新对应条目的状态与说明。  
> README 只保留快速启动。历史设计稿 `bastion_architecture_design_*.plan.md` 不必再读。

**最后更新：** 2026-09-17（GitHub 镜像 + Release 产物；公开 Demo 环境：一键登录 broker + ADMIN 演示账号 + 自愈重置；真证书部署可免 SPKI pin；Agent 内网 IP 不再漏报桥接/VLAN/容器网卡；监控查询索引在 hypertable 上改按 chunk 建）

---

## 状态图例

| 标记 | 含义 |
|------|------|
| `[x]` | 已完成（可用 / 已联调） |
| `[~]` | 部分完成（有 UI/骨架/占位，核心未通或未达设计） |
| `[ ]` | 未开始 / 明确延期 |
| `[!]` | 已知问题、过渡实现，或与原设计有意偏离（见说明） |

---

## 0. 实现路径约定（拍板）

| 主题 | 约定 |
|------|------|
| 终端 | **Agent 原生 Shell**（Linux PTY；Windows：ConPTY 优先，Server 2016 等无 ConPTY 时 WinPTY；仅 PowerShell + UTF-8；Linux bash），不依赖 sshd |
| 文件管理 | **Agent 原生文件协议**，不依赖 SFTP/sshd |
| RDP/VNC | **系统服务 + Agent TCP 隧道 + guacd/Guacamole**；不做 Agent 自采屏 |
| 会话 UI | **新浏览器标签**多开；会话页宜无侧栏；Shell/桌面/文件顶栏两行：资产名·协议；分组 / 公网IP / 内网IP；浏览器 tab 标题：协议·资产名·内网IP；**监控**同新标签无侧栏 |
| Console 语言 | **仅 en/zh**；默认 **en**；浏览器 `Accept-Language`/`navigator.language` 以 `zh` 开头用中文；无手动切换器（后置） |
| OS 操作区 | Windows：PowerShell · 远程桌面 · 文件；Linux：Shell · VNC · 文件 |
| 数据面 | 控制 1 条 WSS + **一会话一数据 WSS** + **可选监控 WSS**（插件 `/ws/agent/metrics`）；Gateway 不连库；无 Redis/MinIO；文件内容走独立 `filetransfer` 会话（Binary），目录操作走 `filemanager` JSON-RPC |
| 控制面 | Java 唯一写 PostgreSQL；短时票据；本地管理员 break-glass（账密 **强制 TOTP**）；交互登录目标为 GitLab OAuth（GitLab 路径无堡垒侧 2FA） |
| Agent 身份 | 安装下发：`asset-id`（= `assets.id`）+ `agent-token`；重装保留 id、刷新 token；无并行 `agentId`；清库后旧 token 失效，须用安装码重装轮换 token（`asset-id` 只标识不证明所有权） |
| Agent 配置 | 运维配 `agent.yaml` 的 `gateway`（保留 `https://`）+ 可选 `gatewayTlsSpkiSha256`；Agent 身份凭据不进 yaml；WS 路径代码内拼接。安装脚本只写旁路一次性 `install-code` 后启动 Agent，由 Agent 经直连 / `gatewayProxy` / `proxyBridge` 自注册，原子落 `asset-id`/`agent-token` 后删除安装码 |
| Gateway TLS | 生产 Agent 走 `wss://`；公有 CA 可省略 pin；自签名/动态 IP 用 SPKI SHA-256 pin（`OPS_GATEWAY_TLS_SPKI_SHA256`，control-api 与 Gateway 同值；安装命令/脚本/`agent.yaml` 同源）；非本机禁止明文 `ws://`；**禁止**忽略证书校验。自签名+算 pin：[`deploy/gen-gateway-tls.sh`](deploy/gen-gateway-tls.sh)；镜像 compose 另有 [`deploy/ensure-gateway-tls.sh`](deploy/ensure-gateway-tls.sh)（`tls-init` 缺证自签并写 `compose-pin.env`）。**公有 CA 部署置 `OPS_TLS_TRUSTED_CA=1`**：`tls-init` 改写**空 pin**，Agent 走系统 CA，acme.sh 续期即使换私钥也不会让已上线 Agent 掉线；该模式下缺证书直接报错，不再静默自签 |
| 本地端口 | **9100** = control-api（REST）；Gateway **9200 PUBLIC**（https/wss）+ **9201 INTERNAL**（http）；Compose 下 Gateway **host 网络**直绑；Console Docker **443**。生产可用 Nginx 终止 TLS 后反代。**公网只放 443 + 9200**（正向 portmap 另放 20000–21000）；9100/9201/5432/4822 靠服务器防火墙/安全组，**不**在 compose 里绑 `127.0.0.1`（Gateway host 网需要宿主机环回映射；**9201 绑环回会断** console→`host.docker.internal`）。Console nginx 已 404 `/api/internal`、`/api/sessions/internal`、`/api/opsctl` |
| URL 变量 | 模板 [`.env.example`](.env.example)（中文注释）；本机复制为 `.env`（**gitignore，勿提交**），`scripts/load-env.ps1` 先加载 `.env`，再叠加 `.env.local`。**PUBLIC** / **INTERNAL** 见该文件。Compose full：Gateway host 网 → `OPS_CONTROL_INTERNAL_HTTP=http://127.0.0.1:9100`、bridge 服务经 `host.docker.internal:9201` 调 Gateway |
| woopsctl / 部署 Token | 对外二进制名 **`woopsctl`**，内部包/API/配置契约保留 `opsctl` 命名；**资产级**一机多 Token（`ops_<tokenId>_<secret>`）；可选 `expiresAt`（空=无限期）+ 可选备注；scope 五项独立：`upload` / `download` / `exec` / `forward` / `reverse`（后两者对应临时端口映射，默认不授权）；可见资产即可管理；仅 `OPSCTL_CONFIG` JSON（`server`+`token`+`pin`，创建时展示一次）；`server` 为 Gateway **https** 基址；通道：Gateway 反代换票 + filetransfer/exec/临时 portmap WS；旧 `OPSCTL_SERVER`/`OPSCTL_TOKEN` 已移除 |
| 用户 API Token | 与 Deploy Token 隔离；个人中心 `/profile` 管理；明文 `wpat_<id>_<secret>` 仅创建展示一次、库存 SHA-256；scope 白名单可扩展：`assets:read`（资产/分组只读列表与详情）、`metrics:read`（监控与指标统计报表）；可选 `expiresAt`（**空=永久**）；仅新建+删除；路径→scope 白名单 + 实时用户资产范围；个人中心「API 说明」用 `{{API_REQUEST_PREFIX}}` 模板注入环境变量/浏览器来源地址，Markdown 预览与复制全文均带实际前缀；见 [`docs/metrics-report-api.md`](docs/metrics-report-api.md) |

---

## 1. 平台与工程

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | Monorepo 骨架 | `apps/console`、`apps/control-api`、`go/{gateway,agent}`、`go/cmd/woopsctl`、`go/internal/opsctl`、`deploy/`；环境变量 `.env.example` → 本机 `.env`（不入库）+ README 编译/启动；许可证 Apache-2.0 |
| `[x]` | PostgreSQL + TimescaleDB | Compose 用 **`timescale/timescaledb:2.29.2-pg16`**（监控依赖扩展；`command` 设 `shared_preload_libraries=timescaledb`）；Java 唯一写库；**不用 H2**。默认账密 `ops`/`ops`（生产务必改）；compose 映射宿主 `:5432` 给本机工具，公网靠防火墙关掉。**control-api 启动**幂等执行 `CREATE EXTENSION` / `create_hypertable` / 压缩策略（`MetricsTimescaleBootstrap`，打进 control-api 镜像） |
| `[x]` | docker-compose 全栈 | **拉镜像**：根目录 [`docker-compose.yml`](docker-compose.yml) → Hub `cdisk/woops-{console,control-api,gateway}`（默认标签 `WOOPS_IMAGE_TAG=0.1.6`）；README §A 快速启动；**`tls-init`**：无 `deploy/tls/gateway.*` 时按 PUBLIC URL 自签，并刷新 [`deploy/compose-pin.env`](deploy/compose-pin.env)（SPKI，供 control-api/gateway）。**源码构建**：[`deploy/docker-compose.yml`](deploy/docker-compose.yml)。`deploy/env.prod.example`；服务器 `/opt/ops`（**10.255.17.30**）；profiles `full`+`desktop`；**Gateway `network_mode: host`**；TLS 挂 `deploy/tls`（gitignore）；`Dockerfile.control-api` 用阿里云 Maven + BuildKit `/root/.m2` 缓存。full 另映射 control-api `:9100`、guacd `:4822`（公网靠防火墙）。**「重新部署」**：见 `.cursor/rules/redeploy.mdc`（17.30 源码构建 + Hub 推送 `$ver`/`latest` + 升 compose 默认标签） |
| `[x]` | 公开 Demo 环境 | [`deploy/demo/`](deploy/demo/)：`bootstrap-host.sh`（Docker / socat / acme.sh）、`make-env.sh`（随机密钥 + 真证书 `.env`）、`issue-cert.sh`（acme.sh standalone 签到 `deploy/tls/gateway.*`，`--reloadcmd` 续期后重启 console/gateway）、`docker-compose.demo.yml`（`!override []` 撤掉根 compose 发布的 5432/9100/4822；起容器化 Agent 当演示资产，身份文件落具名卷、容器文件系统一次性）、`Dockerfile.agent` + `entrypoint-agent.sh`（Agent 二进制取自 gateway 镜像；不写 pin，走系统 CA）、`firewall.sh`（只放 22/80/443/9200，docker0/br+ 放行 9201（否则 /ws 超时）；DOCKER-USER 丢弃 Agent 子网 FORWARD 掐断出网）、`reset.sh` + `install-cron.sh`（每小时整点重置）。域名 `woops-demo.tool4dev.net` |
| `[x]` | Demo 一键登录 | 访客不填账号密码：[`deploy/demo/broker/`](deploy/demo/broker/)（Python 标准库，无依赖）用演示账号在**服务端**完成两步登录，把 12h 的 JWT 缓存下来给所有访客共用，页面按 Console 的 localStorage 约定写 `token`/`username`/`nickname`/`role`/`capabilities` 后跳转。**不能让访客自己填 TOTP**：`AuthService.consumeTotpCode` 按 `step <= last` 拒绝任何不比上次更新的时间步，同一个 30 秒窗口里只有第一个人能登进去。broker 由 `nginx.demo.conf` 挂在 Console 同源的 `/demo/`（同源才写得了 localStorage），并往 `index.html` 注入未登录时的浮动入口。**上游必须写成变量**（`resolver 127.0.0.11` + `set $demo_broker`）：`proxy_pass http://demo-broker:8088` 这种写死主机名是 **nginx 启动时解析**，broker 没在跑时 nginx 直接 `[emerg] host not found in upstream` 拒绝启动，整个 Console 陷入重启循环（单独 `--force-recreate console` 时踩过）；变量形式下 broker 挂了只是 `/demo/` 返 502，且 broker 回来后无需重启 console。broker 同时认 `/token` 与 `/demo/token`，所以变量上游不带 URI、原样透传前缀即可。演示账号密钥由 `setup-demo.py` 生成并直写 `users.totp_secret`（该列无对外接口），落到 gitignore 的 `demo.env` / `.demo-secrets.env` |
| `[x]` | Demo 账号权限 | 演示账号给 **ADMIN**（非 SUPER_ADMIN）：端口映射的对外可达性已被 `firewall.sh` 捏死，风险面主要是「访客能删资产、改自己密码、建用户建分组」。因此重置必须**自愈**：`reset.sh` 读身份卷里的 `asset-id` 与库比对，资产被删过就清卷、用新安装码重新注册；访客建的用户/分组/多余资产经 API 删除（`assets` 被 `user_scopes`、告警表外键引用，裸 `DELETE` 会撞约束）；演示账号的密码 / 角色 / `totp_last_step` 一并复位。**ADMIN 不等于看得见全部**：只有 SUPER_ADMIN 绕过 scope，ADMIN 的可见范围为空时资产列表是空的，所以 `setup-demo.py` 每次都把全部分组以 `GROUP` scope 授给演示账号（`PUT /api/users/{id}/scopes`） |
| `[x]` | guacd sidecar | `deploy/docker-compose.yml` → `guacamole/guacd:1.5.5`（发布宿主 `:4822`）；Gateway `OPS_GUACD_ADDR=127.0.0.1:4822`、`OPS_GUAC_BRIDGE_HOST=host.docker.internal`；console/control-api/guacd 配 `extra_hosts: host.docker.internal:host-gateway` |
| `[x]` | `data/ops-audit/` 卷 | 运行态 JSONL **与会话录像**的本地根目录；`OPS_AUDIT_DIR`（默认 `./data/ops-audit`；Compose `/data/ops-audit` 同时挂 control-api / gateway / guacd）；已 gitignore |
| `[ ]` | `openapi/` 契约 | Java REST → Vue TS client |
| `[~]` | 运维文档 | CI 已有 [`docs/woopsctl-gitlab-ci.md`](docs/woopsctl-gitlab-ci.md)；用户 API Token / 指标报表 [`docs/metrics-report-api.md`](docs/metrics-report-api.md)；Linux Agent **离线/Bridge 手工安装** [`docs/agent-manual-install.md`](docs/agent-manual-install.md)；README **§A 镜像快速启动** + 本机编译路径；控制台安装弹窗可**单独复制安装码**；仍缺 GitLab OAuth 专文 |
| `[x]` | 健康检查 | control-api `/api/health`、gateway `/health` |
| `[x]` | 公网/LAN URL 解析 | `PublicUrlResolver`：配置了非 loopback 的 `OPS_GATEWAY_PUBLIC_*` / `OPS_CONTROL_PUBLIC_HTTP` 时用配置；仅 loopback 时按浏览器 Origin 改写 LAN IP；`GatewayClient` 走 `OPS_GATEWAY_INTERNAL_HTTP` |
| `[x]` | 代码组织约定 | Java 按业务模块（`PageSupport`、`AccessService`、`SessionTicketService` → `ProtocolRegistry` + `protocol.*TicketIssuer`）。Go 最终分类：Agent `agent/{app,core,sessions,services,plugins,infra,sessionreg}`；Gateway `gateway/{app,core,sessions,services,plugins,infra}`。`app` 是唯一具体组合根并可依赖全部功能；`core` 只依赖注入契约，不得反向依赖 `sessions/services/plugins`；功能实现不得跨 `sessions/services/plugins` 横向依赖另一功能。共享契约位于 `internal/{protocol,sessioncore,sessionws,tlsutil,wsutil,hostinfo}`；UDP 帧为 `protocol/datagram`，网卡筛选为 `hostinfo/netiface`；Guacamole 实现由 `gateway/sessions/desktop/guac` 私有拥有。Gateway 验票边界只返回 `BaseClaims`+原始 JSON，`sessioncore.BridgeSpec` 提供通用桥接编排；`cmd/{agent,gateway}` 只进入各自 `app`。Vue `features/registry.js` 是路由/资产动作唯一聚合点；忌过度抽象 |
| `[!]` | 迁走后的旧文件未删 | **Console：** 路由已走 `features/`，`modules/` 实际只用 `assets`/`auth`/`users`；`modules/{sessions,monitor,portmaps,ops-audit,audit,asset-events}` 仍在仓库、无引用，待删。**Go：** `cmd/gateway` 已走 `gateway/app`；旧顶层 `server.go`/`desktop.go`/`bridge.go`、`gateway/{audit,opsaudit,installscripts,portmap}` 仍在仓库，待删 |
| `[x]` | Go 架构边界与静态可移除性审计 | `go/internal/architecture` 从实时文件系统解析 Go imports（不读取 Git index），强制 core 依赖方向与功能横向隔离；审计分类：`core`=中立运行时，`sessions`=会话数据面，`services`=常驻业务服务，`plugins`=可选后台/端点，`infra`=跨切面技术设施，`app`=组合根。功能由独立 `app/register_*.go` 静态注册；删除/替换对应注册文件即从二进制组合中移除，无需修改 `core`/`app.go`。CI overlay 已证明 shell（Agent+Gateway）、desktop（Gateway）、portmap（Agent+Gateway）、monitor（Agent+Gateway）移除后相关 cmd 仍可构建 |
| `[~]` | 协议兼容测试 / 集成测试 | `internal/protocol/control` open_session nested-params golden 已有；Agent→Gateway→echo / Testcontainers 仍缺 |
| `[x]` | 运行时 | 控制面 **Java 21** + Spring Boot 3；Go **`go 1.20`**（模块最低版本；日常可用 Go 1.22+ 编译；legacy Agent 须 **`GOTOOLCHAIN=go1.20.14`**）；Vue 3 + Element Plus；图标 `@tabler/icons-vue` |
| `[x]` | Console 中英 i18n | `vue-i18n`（`legacy:false`）；仅 `en`/`zh`；**默认英文**，`navigator.language` 以 `zh` 开头则中文；目录 `apps/console/src/i18n/{en,zh,index}.js`；`App.vue` 用 `el-config-provider` 同步 Element Plus locale；SFC `useI18n()`，纯 JS `import { t } from '…/i18n'`；控制台可见文案已迁入目录（含 Layout/Login/资产/用户/审计/会话/文件/桌面/端口映射/监控等） |
| `[x]` | Console 视觉（冷静工程风） | 全局 `styles/{tokens,element-theme,base}.css`；主色 `#2F5D9F`、浅色底；字体 **IBM Plex Sans/Mono** 经 `@fontsource` **同源自托管**（无 Google Fonts CDN）；登录品牌首屏；Layout 侧栏图标+**顶栏显示当前页标题**（路由 `meta.titleKey`，页内不再重复大标题）；侧栏底部 **版本号**（`package.json` → `v0.1.6`）+ **源码链接**（`https://gitee.com/cdisk/woops`）；业务页筛选/操作留在内容区工具条；会话页顶栏抛光（桌面页保持暗色功能面）；**favicon** `public/favicon.svg`（主）+ `.ico` / apple-touch PNG；登录与侧栏品牌点同源 SVG |
| `[x]` | 共享资产树选择器 | `shared/AssetTreeSelect.vue` + `assetTree.js`：分组树 + 可选资产节点；**可搜索**（名称/主机名/公网·内网 IP）；分组不可选；端口映射创建与控制/操作/资产事件审计筛选共用 |

---

## 2. 认证、RBAC、GitLab

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | 本地 break-glass 管理员 | JWT（**12h**，存在 Console `localStorage`）；默认 `admin`/`admin123` → `SUPER_ADMIN`（生产必须改，见 §9）；配置齐 GitLab 后关闭本地登录；**本地账密强制 TOTP**（未绑定则登录后扫码绑定，已绑定则输 6 位码；`POST /auth/login` → `login/totp` 或 `login/totp-setup`；pending JWT 不可当会话用）。**每次受保护请求查库**：禁用/软删立即 401；角色以库为准。登录/TOTP/GitLab 兑换 **10 次失败锁 15 分钟**（内存，按 IP+身份） |
| `[x]` | 本地登录 TOTP（2FA） | RFC 6238（`dev.samstevens.totp`）；用户表 `totp_secret`/`totp_enabled`/`totp_last_step`（secret **现明文存库**，待加密见 §9）；登录页二维码+密钥；校验允许 ±1×30s 时钟偏差，**同一步长验证码不可重放**；管理员可 `POST /api/users/{id}/totp/reset` 清 2FA（下次登录重绑）；控制审计 `TOTP_ENABLE`/`TOTP_RESET` |
| `[x]` | GitLab OAuth2 | `OPS_GITLAB_BASE_URL` + `CLIENT_ID` + `CLIENT_SECRET` 齐则启用并关本地登录；`OPS_GITLAB_ADMIN_USER`（逗号分隔 GitLab 用户名）登录时升为 `SUPER_ADMIN`，其余首登 `MEMBER`+空 scope；推荐 `OPS_GITLAB_REDIRECT_URI=https://<console>/api/auth/gitlab/callback`（经 Nginx `/api`）；scope 仅 `read_user`；回调把 access JWT 存内存 **60s 一次性 code**，跳转 `/login?code=`，登录页 `POST /api/auth/gitlab/exchange` 兑换（不再把 JWT 放 query）；GitLab `name` → 用户 `nickname` |
| `[x]` | 系统设置 · GitLab 安装信息 | `GET /api/settings/gitlab/install-info`（超管）；含 Redirect URI |
| `[ ]` | GitLab Group → Role 映射 | CRUD 映射表；登录合并角色（后置） |
| `[x]` | User / Role + 数据权限 | 三角色 `SUPER_ADMIN`/`ADMIN`/`MEMBER`；表 `user_scopes`（`GROUP`/`ASSET`）；勾组=子树可管（受角色约束），勾资产=仅用不可删；展示祖先组动态计算；自写 `AccessService`；列表与按 id 读写/票据均强制校验防 IDOR；`JwtAuthFilter` + `SessionController` 发票走 `requireUser`（禁用后不能再开壳） |
| `[~]` | 短时网关票据策略 | Java 已发 shell/filemanager/filetransfer/rdp/vnc/**exec** 票据（约 90s；`exec` 供控制台一键更新与 woopsctl）；按协议 `ProtocolTicketIssuer` 分发（`ProtocolRegistry`）；portmap 另 `PortmapTicketIssuer`（120s 原始 JWT）；目标 30–60s 后续统一；发票前校验资产可见性 |
| `[x]` | 多用户管理 UI | `/users`：顶栏本地搜索（用户名/昵称/角色/来源）；建用户（用户名/昵称/密码）、启用/禁用、软删、超管改角色、设可见范围（混合树：组+资产）；管理员仅管 MEMBER；顶栏优先显示昵称；软删释放用户名，GitLab 再登会新建且启用；禁用后 GitLab 再登仍拒绝；本地用户列 **2FA** 状态，管理员可重置 TOTP |
| `[x]` | 个人中心 | `/profile`（顶栏用户名进入）：**API Token**（含 `assets:read` / `metrics:read`）；**API 说明**（Markdown 预览 + 复制全文）；本地账密用户另有 **账号安全**（改密码、重置 TOTP，需验证当前密码；已绑定时重置还需当前验证码；丢失验证器仍走管理员重置） |
| `[x]` | 审计拆表 | **控制面** `control_audit_events`；**对服** `server_operation_records`（Gateway JSONL → Java 偏移 ingest；Shell/文件/EXEC/桌面录像/portmap）；**资产事件** `asset_events`（系统观测：上下线等，非人为）；`GET /api/server-operations`(+recording)、`GET /api/control-audit`、`GET /api/asset-events` 均支持 `page`/`pageSize` → `{items,total,…}`。`status` 含 `PURGED`。旧表 `audit_events`/`port_mapping_connections` 已退役。**全员强制录制**。控制审计类别含 `MONITOR`（首页预警忽略/取消忽略、指标报表 `REPORT_READ`）与用户 API Token 的 **`API_TOKEN`/`CREATE`/`REMOVE`**（与用户账号 `USER` 分离；部署 Token 为 `CI`） |
| `[x]` | 审计日志页（控制审计） | 控制台 `/audit/control` → `GET /api/control-audit?page&pageSize`（`{items,total,page,pageSize}`，默认 50/页）；有资产按可见范围，分组按可见分组，登录/用户类管理员可见（成员仅自己的登录）；列表分组独立列；资产筛选用 **AssetTreeSelect**（可搜索），`?assetId=` 深链；类别含监控忽略 |
| `[x]` | 操作审计页 | 侧栏「审计」下拆「控制审计」/「操作审计」/「资产事件」；操作审计内 **tab**：会话操作（排除 `PORTMAP_*`）/ 端口连接（仅隧道连通）；端口映射**清单配置**仍在控制审计 `PORTMAP`；`features/audit/{control,operations,assetevents,shared}`；`GET /api/server-operations?scope=session|portmap&page&pageSize`；列表分组独立列；资产筛选 **AssetTreeSelect**；终端 `asciinema-player`；桌面 `Guacamole.SessionRecording` 页内回放；资产详情可跳操作审计/控制审计/资产事件 |
| `[x]` | 资产事件页 | `/audit/asset-events` → `GET /api/asset-events?page&pageSize`（返回 `{items,total,page,pageSize}`，默认 50/页）；记 **上线/离线**（detail：`sourceIp`/`privateIp`/`agentVersion`/`reason`/`gatewayInstance`/`connectionId`）与 **内网 IP 变化**（`from`/`to`）；列表：分组独立列、详情列展示公网/内网/版本/原因，悬停 tip 含 Gateway/连接 ID；按资产可见范围分页；资产筛选 **AssetTreeSelect**；`?assetId=` |

**角色摘要：** 超管无限制；管理员不可改超管/其他管理员，可见范围内管资产与 MEMBER；成员不可增删资产/发安装码/改分组，**可见即可**开 Shell·文件·桌面·exec·一键更新、管部署 Token 与端口映射、改显示名与桌面凭据（不做协议/动作细 ACL，残余风险见 §9）。

---

## 3. 资产、分组、凭证、ACL

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | Asset 注册模型 | PK `assets.id` 即主机身份（落盘 `asset-id`）；hostname、在线、IP；`os` 为安装脚本上报的详细版本；`agent_version` 为 Agent 二进制版本（`yymmddhhMM`） |
| `[x]` | 公网/内网 IP 自动上报 | **公网/来源 IP**：Gateway 控制 WSS 的 TCP `RemoteAddr`（**不**信 `X-Forwarded-For`），上线时写入 `assets.public_ip`（仅公网可路由地址；经正向代理则为最后一跳出站 IP）；**内网 IP**：Agent `core/netinfo` 本机网卡枚举（上线立刻报一次，之后每 **5 分钟**），与监控共用 `hostinfo/netiface` 排除 docker/veth/virbr/VMware/Wintun/VPN/`虚拟` 等；**桥接 / VLAN / 容器网卡不再误杀**：sysfs 除 `device`/`wireless`/`bonding`/`team` 外也认 `bridge` 与 `lower_*`（IP 落在 `br0`/`br1` 或 VLAN 子接口、成员网卡不带地址的物理机）；无 sysfs 判据时按主网卡名兜底（容器内唯一网卡是 veth peer `eth0`，否则内网 IP 全空）；docker 自建桥只按精确 `br-<12 hex>` 拒绝，运维自建的 `br-lan` 不受影响；**RFC1918 排前**，其后可含非 RFC1918 企业内网段（如 `188.x`）；内网变化写资产事件；**已移除** Agent 第三方公网探测（ipify 等）及用连接来源回填内网的兜底 |
| `[x]` | 显示名 / 分组字段 | `group_id` 可空=未分组（仅「全部」可见）；列表 DTO 附带解析后的 `groupName`；旧列 `group_name` 已退役 |
| `[x]` | ServerGroup 树 + 移动资产 | 表 `server_groups`；`GET/POST/PATCH/DELETE /api/groups`；资产 `?groupId=` / `?includeSubtree=` / `?rootOnly=`；控制台 `GroupTree.vue`：「全部」为顶（旁「+分组」建一级）、其下各级（旁「+分组」拆分下拉：点建子组，下拉重命名/删除）；右侧「显示所有」勾选后含子孙组资产（默认仅本级） |
| `[ ]` | Credential 密文库 | AES-GCM + 主密钥；网关按票据取一次性凭据。**现** `desktop_password` 半明文；待做见 §9 |
| `[x]` | ACL：用户可见范围 | `user_scopes`（`GROUP`/`ASSET`）；勾组含子树；单资产仅用不可删（详情 `canDelete`）；祖先组动态计算只读展示；角色×范围：可见≠可管（不可删/不可发安装码）；**可见=可开全部会话协议**（Shell/文件/桌面/exec），不做协议/动作细 ACL |
| `[x]` | 资产列表 / 详情 / 删除 | 列表顶栏本地搜索（名称/主机名/公网 IP/内网 IP 任意子串；支持深链 `?q=`）；左侧分组支持深链 `?groupId=`（可选 `includeSubtree=`）；保留会话/监控操作；列表**不再自动轮询**（点顶栏刷新或切换分组/搜索深链时再拉）；列表紧凑列：**名称+主机名**、**内网 IP+公网 IP** 各一列两行（上行主文、下行小灰字、行距紧）；表头可前端排序：名称、在线、内网 IP、分组、系统、监控、Agent 版本；「详情」弹 dialog（非独立路由）：顶栏改显示名、跳转审计（操作/控制/资产事件，`?assetId=`）、**一键更新**（任何角色可用，可见资产即可；`POST /assets/{id}/agent-update` 发分组安装码并记控制审计 `UPDATE_AGENT`/「版本更新」，再用户 `exec` 票据跑安装命令，弹窗流式日志；**exec 结束后每 1s 轮询资产**，确认重新上线且 `agentVersion` 变化并刷新详情/列表版本号，最多约 120s；未分组/未保存分组变更/离线则拒绝；**更新命令自带代理前缀**：exec 环境无代理，故命令先从目标机 `agent.yaml` 读 `gatewayProxy` 并 `export https_proxy` 等（Linux `/etc/woops-agent/`，Windows `%ProgramData%\woops-agent\`），网闸后主机才能 curl 到 Gateway；读盘而非问 Agent；不回显代理值，避免泄露 user:pass）、RDP/VNC 桌面凭据与分组、左端口映射右部署 Token、可删资产（须输入显示名确认，提示中名称加粗红色）；顶栏按钮：审计+一键更新+删除一组、**保存单独**；无密码提醒；`DELETE` 仅超管/管理员且须在 scope；列表显示 `agentVersion`；**Agent 版本前**窄列「监控」：有异常时 danger tag 显示条数，悬停 tip 列明细（复用 `GET /dashboard/summary` 的 `abnormalAssets`，含离线） |
| `[x]` | 桌面凭据字段 | 统一 `desktop_port` / `desktop_username` / `desktop_password`；Win 首装默认 3389/`Administrator`，Linux 默认 5900/空用户名；资产详情可改（**可见即可改密码**）；列表/详情 API **不**回传密码明文（仅 `hasDesktopPassword`）；开票始终读库；已删除 `ssh_*`；**RDP 另存** `desktop_color_depth`（8/16/24/32，默认 **16**；Guacamole 无 15 位）与 `desktop_rdp_quality`（`low`/`medium`/`high`，默认 **`low`** 关壁纸/主题/字体平滑等） |

---

## 4. Agent 安装、控制面、代理、指标

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | 安装码 15min / 不限次数 / 可吊销 | 16 位 hex（8 字节）；控制台用左侧当前选中分组一键生成（选「全部」则提示先选组）；API 必填 `groupId`；首装与重装均写入/迁移到该分组；命令弹窗注明目标分组并**按 `expiresAt` 倒计时**（到期显示已过期）；无 group 的旧码视为无效 |
| `[x]` | `GET /i/{code}/install.sh` | Linux：`curl … \| bash`；源文件 `go/internal/gateway/services/agentinstall/install.sh`（embed） |
| `[x]` | `GET /i/{code}/install.ps1` | Windows（Win10+）：`curl.exe -o $env:TEMP\…; powershell -File`；有 pin：`-k --pinnedpubkey`；源文件 `gateway/services/agentinstall/install.ps1` |
| `[x]` | `GET /i/{code}/install.bat` | Windows legacy（Win7 / Server 2012）：纯 cmd、**ASCII-only REM**、**CRLF**；**手工安装**下载 `%TEMP%\install.bat`；**一键更新**下载 `%ProgramData%\woops-agent\install.bat` 再 `cmd /c call`；`LIVE=1` 时暂存 `woops-agent-new.exe` 后 `restart-update.bat` 异步重启；`agent.yaml` **整文件重写**为 ASCII 最小配置（禁止 findstr 合并 UTF-8/BOM，否则 `yaml: line 3`）；`asset-id` 用 `set /p` 复用；build &lt; 17763 自动下 WinPTY |
| `[x]` | `GET /i/{code}/agent/{os}/{arch}` | 产物名 `woops-agent-{os}-{arch}`（`.exe`/`.gz`）；`-ldflags=-s -w -X main.Version=…`；可选旁路 `.gz`，安装脚本 `?format=gz` 优先（约 10MB→3MB 下载）；**Docker Gateway** 同步编 `linux/amd64` + `linux/arm64` + `windows/amd64`；gateway/Linux/woopsctl 用当前 Go，**Windows Agent 独立固定 Go 1.20.14**（Go 1.21+ 产物不能运行于 Win7 / Server 2012） |
| `[x]` | `GET /bin/woopsctl/{os}/{arch}` | 公开下载（无需安装码）；产物 `woopsctl-{os}-{arch}`（Windows `.exe`）；**Docker Gateway** 同编 `linux/amd64` + `linux/arm64` + `windows/amd64`；Console 部署 Token 旁弹窗展示绝对 URL 并可复制；nginx/Vite 同源 `/bin/` 反代 Gateway |
| `[x]` | install 脚本行为 | 先下载并校验，再更新 `agent.yaml`，原子写权限受限的旁路 `install-code`，最后启动 Agent；**注册由 Agent 完成**，脚本不再 POST/解析注册响应或写身份凭据。冷装等待最多 60s，确认 `asset-id`/`agent-token` 非空且安装码已消费；重装保留 `asset-id`，Agent 提交旧 id 以迁组并轮换 token；**在线更新**：`LIVE=1`（一键更新/Web Shell）始终延迟 stop→start，安装进程内不停活 agent，新进程自注册，Console 轮询重上线；优先 `systemd-run --no-block`，否则回退 `setsid`/`nohup`；Windows legacy CMD 用独立 `restart-update.bat`。Linux/PowerShell 保留本地 `proxy`/`proxyBridge` 配置；legacy BAT 对已有 YAML 原样保留，避免 UTF-8/BOM 与嵌套配置损坏。下载按 pin；Gateway 嵌入 agent SHA-256；依赖预检 curl/xxd/sha256sum；gzip 可选；已移除 `ops-agent` 迁移/回退 |
| `[x]` | Agent HTTPS 自注册 | Agent 启动先起 pre-auth `proxy`/`proxyBridge`，读取旁路 `install-code`，经统一 Gateway HTTP client 调 `POST /api/agent/register`（Gateway 反代私网 control-api）；响应 `assetId`/`agentToken`/`reused`；可选旧 `assetId` 复用并轮换 token，同时按安装码 `groupId` 迁移分组；上报 hostname、详细 OS/arch、内网 IP、`agentVersion`。凭据临时文件 sync 后原子替换（Windows `MoveFileExW`），确认后删除安装码，无需重启直接进入 control/metrics；同一被拒安装码不轰炸，替换文件后恢复 |
| `[x]` | Agent 验 Gateway | control / session / metrics（含 portmap 隧道）共用 TLS dialer；有 pin 时校 SPKI；无 pin 走系统 CA；假 Gateway / 错 pin 在握手失败，不发 `agent-token`；可选 `gatewayProxy` 时经 HTTP CONNECT 出站（目标 DNS 由代理解析） |
| `[x]` | Agent 构建版本 | `-ldflags -X main.Version=yymmddHHMM`；`woops-agent -version`；**Windows** `go/scripts/build-agent-windows.ps1`（`GOTOOLCHAIN=go1.20.14`，Win7–Win11 通用）；Linux `build-agent-linux.sh`；Docker `Dockerfile.gateway` 同步注入 |
| `[x]` | 控制 WSS | 在线（带 `sourceIp`）、ping、`open_session`、`netinfo`（**仅内网 IP**，不含公网、**不含** metrics）；`ctx` 取消时关闭 WS，避免 `ReadMessage` 卡满读超时（曾致 `systemctl stop` 约 60–90s）；Agent 对 Gateway Ping 用 `PingHandler` 续期 90s 读超时（仅 `PongHandler` 时曾约 90s 周期断连）；**重连前重读** `agent-token`/`asset-id`（一键更新 register 轮换 token 后，旧进程不致永久 `bad handshake`）；**拨号失败** 1s 起指数退避至 30s（带抖动，`control connect retry`）；**曾连上后断开** 重置为 1s 再拨（`control ended`） |
| `[x]` | 一会话一数据 WSS | TCP binary chunk ≈ io.Copy；**协议层 Ping/Pong keepalive**（Gateway 对 browser+agent 两侧每 25s Ping、90s 无 Pong/数据则读超时关闭并 `auditEnd`；与 Text/Binary 业务帧分离，无新信令）。Shell 的 Agent 侧同超时续期；filemanager/exec/filetransfer 仅靠 Gateway 探测（避免长写无读误杀） |
| `[x]` | `protocol_version` / Envelope | `internal/protocol/control`；未知类型忽略；`open_session` **仅**嵌套 `params`（`ShellParams`/`FileTransferParams`/`TunnelParams`）；`netinfo` 仅 `privateIp` CSV；**已移除**平铺字段双写/回退与 `privateIps` 数组双发。golden：`internal/protocol/control/testdata/open_session/` |
| `[x]` | 监控插件 · 独立 metrics WSS | `/ws/agent/metrics`；`agent/plugins/monitor` + `gateway/plugins/monitor`；默认 60s；`metrics.enabled` 可关；断线不标离线；网卡与 `core/netinfo` 共用 `hostinfo/netiface` 真实网卡过滤；磁盘容量排除光驱（`iso9660`/`udf`/`cdfs`；Windows 另 `GetDriveType`=`DRIVE_CDROM`）；Gateway 对 Agent Ping 用 `PingHandler` 续期 120s 读超时（曾约 120s 周期 1006） |
| `[x]` | 监控 Timescale history + trends | **`monitor_history`**（分钟明细 hypertable，近 **7** 天可配）+ **`monitor_trends`**（小时 `min/max/avg/sample_count`，保留 **3** 年可配）；字典 `monitor_item_def`；每小时汇集上一小时并删除超期明细；压缩策略由启动引导自动加；旧表名 `monitor_data` 已迁完。**查询索引** `idx_monitor_history_series`（`asset_id,item_id,time,instance`）由 `MetricsIndexMigrator` 启动时后台建、按 advisory lock 去重、每次启动幂等重试：**hypertable 上必须用 `WITH (timescaledb.transaction_per_chunk)`**，`CREATE INDEX CONCURRENTLY` 会被 TimescaleDB 拒绝（`hypertables do not support concurrent index creation`，SQLSTATE `0A000` 还会让 Hikari 弃掉该连接）；仅未转换的普通表才走 CONCURRENTLY |
| `[x]` | 监控曲线 / 聚合 | `GET …/metrics/series`；grain=minute\|hour\|day\|month；选表：起点在明细保留窗内且跨度≤保留窗 → history，否则 trends（跨边界混合未汇总 history）；trends **按 `sample_count` 加权平均**；响应含服务端 `summaries`（min/avg/max/sampleCount，不随显示粒度变；断线缺口不进均值分母）；磁盘/网卡按 instance 拆分。控制台曲线页直接展示该 summary |
| `[x]` | 指标统计报表 API | `POST /api/reports/metrics`：单资产、多 `itemId`、`[from,to)`、`hour\|day` 等粒度；一次返回区间统计 + 趋势 `points`（`time`/`value`/`min`/`max`，由调用方绘图）；JWT 或 `metrics:read` API Token；控制审计 `MONITOR`/`REPORT_READ`（不记 points/secret）。调用示例 [`docs/metrics-report-api.md`](docs/metrics-report-api.md) |
| `[x]` | 监控预警 | 全局阈值（优先 %）；入库评估；`asset_alert_status`；配置 CRUD；**仅超管**可见菜单与 API。首页异常含离线；忽略按 **资产+监控项**（`asset_alert_ignores`，离线项 `host.online`） |
| `[x]` | 首页概览 | 资产总数 / 在线 / 异常（**含离线**，与监控预警并列）；登录进首页。异常按监控项可 **忽略 / 取消忽略**（如只忽略离线或 CPU，不影响该资产其他项；忽略后不占异常列表；「异常资产」标题右侧打开已忽略弹窗 `IgnoredAlertsDialog`；可见资产即可操作；控制审计 `MONITOR`/`IGNORE`/`UNIGNORE`）。异常列表与已忽略清单均展示 **分组**（`groupId`/`groupName`，未分组显示「未分组」；已忽略弹窗加宽）；**名称+主机名**合成一列两行（名称可点 → `/assets?q=`；主机名小灰字）；**分组**可点 → `/assets?groupId=`（左侧树选中该组；未分组不可点） |
| `[x]` | 资产监控页 | 操作「监控」→ 新浏览器标签 `/assets/:id/monitor`（无侧栏，同会话页）；复用 `AssetMonitorPanel`；顶部规格；各图下最高/平均/最低；时间范围快捷：5 分钟 / 30 分钟 / 1 小时 / 6 小时 / 1 天 / 7 天 / 30 天 / 90 天 / 1 年；图表横向框选时间后按该范围重载全部曲线并自适应 grain；标题左侧关闭（有 opener 则关标签）；首屏 items/latest/series 并行加载，ECharts 按需打包 |
| `[~]` | 资产列表指标摘要 | 已做异常条数 tag（见上「资产列表」监控列，数据来自 dashboard summary）；完整 `monitor_latest` 指标数值摘要未做 |
| `[x]` | Agent 内置 HTTP 正向代理 | 插件式 `go/internal/agent/plugins/proxy`：`TryStart` 软失败不影响控制面；`agent.yaml` `proxy.*`（enabled/listen/username/password/allowCIDRs/allowGlobal）；CONNECT + forward；账密强制；来源 CIDR；`allowGlobal=false` 仅 ops host（`gateway` 主机 :gwPort+`:9100`）；全局时拒 loopback/元数据；**有 `gatewayProxy` 时入站出站经上游 CONNECT/forward 串联**（防环：拒连上游自身） |
| `[x]` | 代理模式 + agent 模式同二进制 | 外网/内网1 开入站 `proxy.*`；更深内网装 Agent 时设 `https_proxy`→`gatewayProxy`（本机 WSS + 可选再开 `proxy.*` 给下一跳）；多级：C→B(proxy+gatewayProxy=A)→A(proxy)→Gateway；`proxy.enabled=true` 时自动写本地 `proxy.log`（见下行） |
| `[x]` | Proxy Bridge 反向载波 | 独立 camelCase `proxyBridge.*`（enabled/listen/key/allowGlobal/targets[address,key]），实现复用 `plugins/proxy` engine；用于 **A 只能主动连 B** 的网络：A targets 主动 TCP 连 B listen，HMAC-SHA256 双向随机挑战认证（64 hex key），yamux 多路承载标准 HTTP forward/CONNECT。B 的 `gatewayProxy` 指向本机 `proxy.listen`，Proxy 父载波优先；A 可 `proxy.enabled=false` 只开 Bridge targets，并继承 A 自己的 `gatewayProxy` 出口。`proxy.allowGlobal` 管普通 listener，`proxyBridge.allowGlobal` 管 peer 流；默认仅 Gateway，全局仍拒 loopback/link-local/metadata。支持 listen+targets 多级 A→B→C、TCP/应用心跳、1–30s 抖动重连；Gateway TLS/WSS 与 pin 端到端不变 |
| `[x]` | Agent 本地文件日志 | `woops-agent.log`：启动/控制与 metrics 连断、会话 `op START/END/FAIL`；**不**记成功 netinfo / monitor 周期 sent。`proxy.enabled=true` 另开 `proxy.log`（请求 START/END/FAIL、耗时、字节；无凭据/header/body/query）。Linux `/var/log/woops-agent/`；Windows `%ProgramData%\woops-agent\`。lumberjack：50MiB 轮转、压缩、`MaxAge=90`（最长约 90 天）。同步 stderr/journald |
| `[x]` | 安装命令带 proxy URL / 读环境变量写入配置 | 安装：`curl`/`curl.exe` 认 `https_proxy`/`http_proxy`/`ALL_PROXY`；脚本写入 `agent.yaml` `gatewayProxy`；控制台弹窗说明单引号与密码百分号编码 |
| `[x]` | Windows 安装 | Win10+：`install.ps1`（PowerShell）；Win7/2012：`install.bat`（cmd，见上行）；OS build &lt; 17763 下载 WinPTY；注册 Service `woops-agent`；**须管理员** |
| `[!]` | Linux Agent 交叉编译 | 勿用本机 `GOOS=linux`（曾 segfault）；用 Linux 容器 `go build` |

---

## 5. 远程连接

### 5.1 操作入口（Console）

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | 新标签打开会话 | `openSessionTab` + 独立路由（无 Layout）；唯一窗口名；Shell/桌面/文件顶栏两行（名称·协议；分组/公网/内网 IP，票据 `asset` 带回）；**浏览器 tab 标题** `sessionTabTitle.js`：`协议 · 资产名 · 首个内网IP`（如 `Shell · web-01 · 10.0.0.5`；`SessionAssetTitle` / 监控页同步 `document.title`）；监控同机制 |
| `[x]` | Windows 操作区 | PowerShell · 远程桌面 · 文件管理；Shell/文件不校验登录密码，RDP 校验 |
| `[x]` | Linux 操作区 | Shell · VNC · 文件管理；同上（VNC 校验密码） |
| `[~]` | 工作台 | 首页已有资产/在线/异常；缺「最近会话 / 我的资产」 |

### 5.2 终端

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | Console Shell 页 | `features/shell/ShellPage.vue`：`/sessions/:id/shell?kind=` → 票据 `shell_*` → `/ws/shell`；状态 i18n 标签；曾连接后 WS 关闭弹「连接已断开」对话框（可关/关标签），避免只靠顶栏小 tag 难察觉；xterm 回滚区滚动条为细圆角暗色（贴合终端底色） |
| `[x]` | Shell 常用命令（按资产） | 顶栏右「常用命令」：`ShellCommands.vue`；`assets.shell_commands` jsonb `List<String>`；`GET/PUT /api/assets/{id}/shell-commands`（可见即可）；点击整段粘贴并回车执行；管理弹窗增删改；审计只记条数不落正文 |
| `[x]` | Agent 原生 Linux Shell（PTY） | `agent/sessions/shell` + `creack/pty`；`LookPath(bash)`→`LookPath(sh)`，再回退 `/bin|/usr/bin` 固定路径（防 systemd PATH 过窄）；`-l` 启动；显式 `SHELL`/`TERM=xterm-256color`/`COLORTERM=truecolor`（避免 `dircolors: no SHELL…`）；启动目录见下行；尺寸在会话内 `R,cols,rows` 热更新（不必像 Windows 那样先等） |
| `[x]` | Agent 原生 Shell 启动目录 | Linux：`cmd.Dir` + 强制 `HOME`/`PWD` 为用户 home；systemd unit 使用 `WorkingDirectory=/`（勿用 conf 目录作 cwd）；Windows：ConPTY `ConPtyWorkDir` + WinPTY `Dir` 用用户目录；LocalSystem 时优先 `C:\Users\Administrator`，否则扫描 `C:\Users\*`（排除 Public/Default），**禁止**回退 `…\systemprofile`（无可用配置则 Public 或盘符根）；另设 `USERPROFILE`/`HOME` 并 `Set-Location` 兜底 |
| `[~]` | Agent 原生 Windows Shell | ConPTY（≥17763）优先 **PowerShell**；无 ConPTY（Win7 / Server 2012）**WinPTY + cmd**（Agent 自动将 powershell 回退为 cmd；控制台 legacy 资产开 `shell_cmd`、xterm 不用 `windowsPty: conpty`）；UTF-8 `chcp 65001`；启动目录同上；**会话先等浏览器 `R,cols,rows` 再建 PTY** |
| `[x]` | Gateway `/ws/shell` + `shell_*` 票据 | Java `type=shell`+`shellKind`（无密码）；shell 模块解码 Claims/构造 Params/录像 hooks，通用 `BridgeSpec` 透传 + 两侧 keepalive；半开断连可收尾审计 |
| `[x]` | 终端按键 | xterm 原样透传；浏览器原生 `Ctrl+W`/`Ctrl+T` 无法可靠拦截，Shell 顶栏提供按钮向终端发送对应控制字符；Windows 终端依赖 ConPTY/WinPTY |
| `[x]` | 已移除经 sshd 的 Web SSH | 无 `/ws/ssh`、无 `ssh` 票据、无 legacy `/sessions/:id`；终端仅 Agent 原生 Shell |
| `[~]` | 终端会话审计上报 Java | 通用桥接在 Agent 数据 WSS 就绪后写 `START`，退出写 `END`（字节数）；shell/filemanager/filetransfer/exec 模块分别提供录像、ACTION sniffer、END finalizer；开会话失败写 `FAILED`。**缺**命令级通用 Shell `ACTION`（见 §10） |

### 5.3 文件管理

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | filemanager（目录） | Agent `sessions/filemanager` JSON-RPC：`list`/`stat`/`mkdir`/`remove`/`rename`/`list roots`；**不含**文件内容读写。list 排序：目录优先再文件（名不区分大小写） |
| `[x]` | Gateway `/ws/file-manager` | 票据 `type=filemanager` → 透传 filemanager JSON-RPC |
| `[x]` | filetransfer（内容） | 独立会话 `type=filetransfer` → `/ws/file-transfer`；一任务一 `transferId`（跨重连稳定）；Binary 帧（OPST 头 + 1MiB 分块 + 分块 SHA-256），**无 base64**；控制帧 STATUS/ACK/COMMIT/ABORT；上传写 `.ops-upload.<name>.<transferId>.part/.meta`，commit 后原子替换；Windows 用 `MoveFileExW(REPLACE_EXISTING|WRITE_THROUGH)`，遇杀软/索引器短暂占用会退避重试，失败时保留原文件（禁止先删目标再 rename）；断线=暂停（保留临时文件），明确取消才清理；不做后台过期清理 |
| `[x]` | Gateway `/ws/file-transfer` | filetransfer 模块自行解码 Claims/构造 Params，通用桥接发送 `open_session`；Binary/Text 原样桥接；模块 sniffer 审计 direction/path/offset/bytes/resumed（不含 payload） |
| `[x]` | Console 文件管理页 | 资源管理器布局；目录操作用 `/ws/file-manager`；上传/下载/文本编辑各自开 `filetransfer`；上传自动重连续传、失败暂停可继续；刷新后须重选原文件（指纹校验）；下载优先 File System Access 流式写盘，否则 ≤64MiB Blob；**连接/列目录/树懒加载/下载共用同一 SessionConnectingMask**（连接：「正在连接文件服务器」；列目录：「正在加载目录」；下载两行：「正在下载 文件名」+「已传/总量（%）」；第三行速度·已耗时·剩余 ETA；完成后改为成功态（大小·耗时），须点关闭；浏览器无法给出另存为完整路径故不展示） |
| `[x]` | 上传进度/速度统计 | `rateMeter.js` 5s 滑动窗口算**瞬时**速度（每 500ms 采样，停滞自动衰减到 0）；续传的远端 offset 只作基线不计入速度（`phase:'start'`）；整体速度只统计本次会话实际推送的字节；单文件与整体均显示「已传/总大小」与按瞬时速度推算的剩余时间；完成行显示本次均速 |
| `[x]` | Console 文件模块拆分 | `features/filemanager/FileManagerPage.vue` 只管连接/列目录/树/重命名删除与接线；目录 RPC、路径、列表格式与编辑 UI（`FileEditDialog.vue`/`TextFileEditor.vue`）在同 feature；内容传输位于 `features/filetransfer/`：`transferClient.js`、上传队列、下载、任务 dialog、文本/编码/速率 helper；依赖方向仅 filemanager → filetransfer |
| `[x]` | 文本在线编辑 | CodeMirror 6；短生命周期 filetransfer 读/写；≤2MiB；二进制拒绝；**编辑器内搜索**（`@codemirror/search` 面板，Ctrl+F / F3，底栏「搜索」按钮） |
| `[x]` | 编辑编码自动识别 | `charset.js`：BOM → 严格 UTF-8 → `gb18030`/`big5`/`shift_jis` → Latin-1；**按原编码写回**（BOM 保留），非 UTF-8 的反向编码表由对应 `TextDecoder` 惰性推导（无新依赖）；无法表示的字符提示改存 UTF-8；编辑器底部可手动改编码并就地重新解码（原始字节缓存，不重新下载） |
| `[~]` | 路径沙箱 + 操作审计 | filemanager：`filesniff` 记 LIST/STAT/MKDIR/REMOVE/RENAME；filetransfer：TRANSFER_* / WRITE/READ 聚合。**缺**路径沙箱（Agent 常以 root/SYSTEM 跑，见 §9） |

### 5.4 桌面 RDP / VNC

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | Console 桌面页 | `features/desktop/DesktopPage.vue` + `guacamole-common-js` → `/ws/desktop`；原生 BlobReader ACK；首个 sync 就绪；**连接时把视口宽高经 query 传 Gateway→guacd 握手**（避免首帧固定 1280×800）；ResizeObserver/sendSize；按 Display scale 换算鼠标坐标；单一原生/软件光标策略；图像解码失败显式报错，不再注入透明位图；右上角特殊按键（Ctrl+Alt+Del / Win / Alt+Tab / Ctrl+Esc / Esc）；**页内全屏**（隐藏顶栏，非浏览器 Fullscreen API）；全屏时左侧边缘把手悬停展开 `DesktopFsRail`（快捷键/剪贴板/退出）；视口 `overflow:hidden` + flex 剩余高测尺寸，避免最大化时滚动条闪 |
| `[x]` | RDP/VNC 文本剪贴板 | 双向 `text/plain`：本地 `Ctrl+V` 读取系统剪贴板→StringWriter stream/end→CLIPRDR 短延时后注入远端粘贴（guacd 此方向不回 ACK）；远端复制通过 BlobReader ACK 后自动/手动写回本地；有权限失败时的面板备用；256 KiB 上限；Word/Excel 降级保留文字、换行、制表符 |
| `[!]` | 富格式/图片/文件剪贴板 | guacd 1.5.5 RDP 仅可靠支持 Unicode/text；不发送 `text/html`、RTF、PNG、文件和 Office 私有格式，避免 FreeRDP CLIPRDR 断链；文件继续走 Agent 文件管理 |
| `[x]` | Agent TCP 隧道 | `agent/sessions/socket`；仅二进制 WS 帧；Agent 接入校验 ticket 的 sessionId/assetId/type；端口始终读资产 `desktopPort`；RDP 拨号主机：CSV 中**优先 RFC1918**，否则 `127.0.0.1`（避免虚拟网卡/非标准内网段排第一导致 dial timeout） |
| `[x]` | Gateway ↔ guacd 标准 Tunnel | `gateway/sessions/desktop`（私有拥有 `guac/`）：desktop 自有 Claims/Handler/桥接与录像生命周期；configured handshake 消费 `ready`；浏览器首帧为空 opcode UUID；内部 ping 原样响应；RDP 固定 NLA；凭据仅 `desktopUsername`/`desktopPassword`；握手读取 `width`/`height`/`dpi` 与票据中的 **`color-depth` / 画质体验开关**；长度前缀按 Unicode 字符数 |
| `[~]` | 目标机开启系统 RDP / 安装 VNC | **前置条件**（见下）；凭证用资产 `desktopUsername`/`desktopPassword` |
| `[x]` | 明确不做 Agent 自采屏 | |

**目标机前置（Phase D）：**

- Windows RDP：系统设置开启「远程桌面」；资产填 RDP 用户/密码；端口存 `desktopPort`（首装默认 3389，可改）  
- Linux VNC：安装并监听桌面端口（首装默认 `5900`，可改）；资产填 VNC 密码（用户名可空）  
- 本机 Gateway + Docker guacd：`docker compose -f deploy/docker-compose.yml --profile desktop up -d guacd`；Gateway 使用 `OPS_GUACD_ADDR=127.0.0.1:4822`、`OPS_GUAC_BRIDGE_HOST=host.docker.internal`
- 全 Docker（Gateway host 网络）：同时启用 `full`、`desktop` profile；compose 固定 `OPS_GUACD_ADDR=127.0.0.1:4822`、`OPS_GUAC_BRIDGE_HOST=host.docker.internal`；guacd 发布 `4822`；control-api/console 经 `host.docker.internal:9201` 调 Gateway INTERNAL

### 5.5 连接实施阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| A | OS 差异化按钮 + 路由 + 新标签 | `[x]` |
| B | Agent 原生 Shell | `[x]` |
| C | Agent 原生文件（列表/基础操作） | `[x]`（上传下载已拆独立 `filetransfer`） |
| D | RDP/VNC 标准 Guacamole Tunnel + guacd + Guac 客户端 | `[x]`（WIN-KVTUHJVTOE1 真机 NLA：UUID、Blob ACK、2 次 sync、13/11 张图像解码、非黑帧、resize/鼠标/键盘协议均通过） |
| E | 移除 sshd-SSH、桌面凭证字段、RBAC | `[x]` 已删 Web SSH；桌面统一 `desktop*`；RBAC 已做 |

---

## 6. 端口映射（TCP/UDP）

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | 映射清单表 `port_mappings` | tcp/udp；方向 `gateway_to_asset`（正向，默认/历史空值）或 `asset_to_gateway`（反向）；`listen_host`（正向默认 `0.0.0.0`，反向默认 `127.0.0.1`）；目标；可选 `remark`（≤256）；**仅动态端口池**（20000–21000，反向可选手动端口）；创建时选定 `listen_port`；`removed_at` 空=可恢复；运行态不进此表 |
| `[x]` | 连接记录 | 表 `port_mapping_connections` **已删**；open-record 只发短时票据（HTTP close-record **已移除**）；隧道起止由 Gateway 写 JSONL（`PORTMAP_TCP`/`PORTMAP_UDP`，`detail` 含 `direction`/`mappingId`/`listenHost`/`listenPort`/`targetHost`/`targetPort`/`clientAddr`/`bytesIn`/`bytesOut`），Java 按偏移量 ingest 进 `server_operation_records`；`GET /api/port-mappings/history` 查该表（`mappingId` 按 `detail.mappingId` 过滤）；`bytesIn`=入口客户端→目标 |
| `[x]` | 正向动态端口分配 | 创建清单时随机选 20000–21000，且 ∉（Gateway 已 Listen ∪ 未删除正向清单端口）；Gateway Listen **默认 `0.0.0.0`**（host 网络直绑宿主，compose **不**再映射该范围；生产暴露面见 §9） |
| `[x]` | 反向端口分配 | 可指定 `listenPort`（留空则 20000–21000 自动）；冲突按「资产+协议」隔离；最终以 Agent bind 结果为准；`listenHost` 可配（默认 `127.0.0.1`，可 `0.0.0.0`） |
| `[x]` | 正向 TCP：每 Accept → 一条数据 WSS | Gateway Listen → open-record 发短时 `tcp` 票据 → Agent 拨目标；多客户端=多 WSS；断连接只关该 WSS；任一方向结束后给反方向 1s 排空再关闭；纯 TCP 透明转发 |
| `[x]` | 正向 UDP：每映射一条 WSS + DATAGRAM 帧 | 帧：`[hostLen][host][port][dataLen][data]`；Agent 按入口客户端地址使用独立目标 UDP socket（2min 空闲回收），避免多客户端并发响应串流 |
| `[x]` | 反向 TCP：Agent Listen → 每 Accept 换票建 WSS | Agent 调 Gateway `POST /api/agent/portmap/open-connection`（`X-Asset-Id`/`X-Agent-Token`，校验映射归属与方向）→ Gateway Arm + 换票 → Agent 数据 WSS → Gateway Dial 目标并 Pipe；目标取库配置不信 Agent 上报 |
| `[x]` | 反向 UDP：Agent ListenPacket + 一条 WSS | 同 DATAGRAM 帧带本地客户端地址；Gateway 为每客户端独立目标 UDP socket + 空闲回收，避免串包 |
| `[x]` | Agent 上线自动恢复 | Gateway 控制 WSS online 后拉该 Agent 未删除清单：正向 Listen、反向 `portmap_listen`；Gateway 冷启动不拉清单；控制 WSS 断线时 Agent 关闭全部反向监听，防孤儿端口 |
| `[x]` | Console 映射管理页 | `/portmaps` + 侧栏菜单；顶栏本地搜索（资产/监听/目标/备注等）；创建选资产用 **AssetTreeSelect**（可搜索，显示在线状态）；创建可选方向；反向可填监听地址/可选端口；资产信息/分组/清单/运行状态/备注；连接历史抽屉；可改备注 |
| `[x]` | 资产详情内映射 | 资产详情 dialog 嵌入 `PortMapPanel`（本资产）；全量管理仍走 `/portmaps` |
| `[x]` | Java API | create（含 direction/listenHost/listenPort/remark）/ patch remark / remove / list / runtime / history；internal：按 assetId 清单、连接 open-record（只发票据，含 direction）、last-error |
| `[!]` | 反向权限与 SSRF | 沿用「可见资产即可创建」；反向会让 Gateway 拨其可达网络（Docker 下 `127.0.0.1`=容器自身）；**本期无**目标 denylist / 短租约 ACL（待做见 §9） |
| `[x]` | woopsctl 临时正向 TCP/UDP | `woopsctl forward --protocol tcp\|udp --listen host:port --target host:port`；监听在 ctl 本机，目标由 Agent 拨；Gateway 不监听业务端口、不写 `port_mappings`；TCP 每 Accept 一条数据 WSS，UDP 每命令一条 DATAGRAM WSS |
| `[x]` | woopsctl 临时反向 TCP/UDP | `woopsctl reverse --protocol tcp\|udp --listen host:port --target host:port`；Agent 监听，目标由 ctl 本机拨；轻量控制 WSS 维持临时租约，TCP 每连接独立数据 WSS，UDP 每映射一条 DATAGRAM WSS；稳定且强制 `opsctl:` 命名空间的命令级 `ephemeralId` 保证重连幂等并隔离持久 mapping UUID |
| `[x]` | 临时映射自动恢复 | woopsctl 是期望状态持有者：Gateway 重启、网络闪断、Agent 离线均不退出，1s→2s→4s（最大 30s、带抖动）持续重连；Agent/Gateway 恢复后自动重新换票/登记/监听。旧 TCP 连接可断，后续连接恢复；仅 Ctrl+C、参数错误、Token 过期/删除/无权限等永久错误退出 |
| `[x]` | 持久/临时共用引擎与审计 | Gateway 单一 `services/portmap` 共用 Agent 协议、relay、配对、字节统计与审计；DB 清单和 Deploy Token 仅是不同授权/lifecycle adapter，不启动 woopsctl 子进程。临时流量仍写 `PORTMAP_TCP/UDP` 到操作审计端口连接 tab，detail 含 `ephemeral=true`/`initiator=opsctl`/新 direction 且无 `mappingId`，故不进入持久映射历史抽屉 |

---

## 7. 运行态 JSONL spool 与会话录制

**JSONL spool（已实现）：** Gateway 不连库，运行态操作以 JSON 行追加落盘，control-api 按字节偏移量 tail。

```
{OPS_AUDIT_DIR}/{yyyy}/{mm}/{dd}/events-{instanceId}-{seq}.open    # 当前段（追加中）
{OPS_AUDIT_DIR}/{yyyy}/{mm}/{dd}/events-{instanceId}-{seq}.jsonl   # 已封段（64MiB 或跨天轮转、进程退出封段）
{OPS_AUDIT_DIR}/{yyyy}/{mm}/{dd}/recordings/{operationId}/session.cast   # 终端录像
{OPS_AUDIT_DIR}/{yyyy}/{mm}/{dd}/recordings/{operationId}/session        # guacd 桌面录像
{OPS_AUDIT_DIR}/.cursor/{yyyy-mm-dd}-{段文件名}.offset             # ingest 游标（原子写）
{OPS_AUDIT_DIR}/.cursor/quarantine.jsonl                          # 坏行隔离（仍推进偏移量）
```

- 信封：`phase` = `START`/`ACTION`/`ERROR`/`END`；`operationId` = sessionId（与录像目录同键）；`assetId`/`userId`/`username` 来自票据 claims（portmap 票据无用户，只有 assetId）
- Go：`go/internal/gateway/infra/audit`（`Writer`、`RecordingDir` / `RelPath` / `CastRecorder`）；`gateway/core/audit.go` 提供协议无关 spool/录像文件能力，功能通过窄回调提供 hooks/finalizer；`OPS_AUDIT_DIR` 为空则整体 no-op
- 录像取用：`GET /api/server-operations/{operationId}/recording` 按 `detail.recordingPath` 解析到 `OPS_AUDIT_DIR` 下（`normalize()` 后必须仍在根内，防路径穿越），`application/octet-stream` 流式返回
- Java：`OpsAuditIngestScheduler` 每 `OPS_AUDIT_SCAN_SECONDS`（默认 5s）扫**今天与昨天**（UTC）目录，单段单次最多读 4MiB，尾部残行留到下次；至少一次投递 → `ingestEnvelope` 按 `operationId`/`eventId` 幂等（END 后重放的 START 不回退状态；**已结束的 END 不再改 status**）；DB 异常不推进偏移量，坏行进隔离文件

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | 终端 asciinema cast v2 | `gateway/infra/audit/cast.go` `CastRecorder`；`gateway/sessions/shell` 将输出/输入/resize hooks 挂入通用桥接并负责 Close/finalizer；落 `recordings/{operationId}/session.cast`，END 带 sha256+size |
| `[x]` | RDP/VNC Guacamole 原生录像 | desktop 模块传 `recording-path`/`recording-name=session` 给 guacd；仅 POSIX 绝对 spool 路径启用；`RecordingDir` 叶子 0777、日期分区 0755；模块负责正常/中断 finalizer，END 算 sha256；不转 MP4 |
| `[x]` | 录像元数据 | END `detail`：`recordingPath`/`sha256`/`size`/`format`（`asciinema`/`guacamole`） |
| `[x]` | 回放 UI | 回放路由的 `el-main` 固定为可见区且禁滚动，播放器严格占满剩余宽高；终端 asciinema 在布局稳定后创建并用 `fit:both`；桌面 Guacamole SessionRecording（播放/暂停/seek；`display.scale` 按容器宽高比适配，Guacamole bounds 合成层渲染，`ResizeObserver` + `requestAnimationFrame` 即时重适配）；两者均无内外滚动条并支持页内「全屏」；Vite 打补丁修 npm `guacamole-common-js@1.5.0` 的 GUACAMOLE-1793；**勿**对 display 根节点设 `height:auto`（会黑屏只剩光标） |
| `[x]` | 保留天数清理 | `OpsAuditRetentionJob` 每天 04:15；`OPS_AUDIT_RETENTION_DAYS`（默认 90） |
| `[x]` | RUNNING 收尾 | Gateway `SIGTERM`/`CloseAudit`：在途操作写 `END`+`INTERRUPTED`（`reason=gateway_shutdown`），并 **先 flush 录像**；ingest 先到的 END 生效。**Gateway 每次启动** 调 control-api `POST /api/internal/server-operations/interrupt-running`，把库里仍 `RUNNING` 的标为中断（兜底 Windows `Stop-Process -Force` 等无法收信号的强杀）。`OpsAuditStaleRunningJob` 启动时 + 每 15 分钟把超过 `OPS_AUDIT_STALE_RUNNING_HOURS`（默认 24）仍 `RUNNING` 的标为 `INTERRUPTED` |
| `[x]` | 强制全员录制 | Shell/桌面会话一律录制；不做按角色开关 |

---

## 8. CI/CD（woopsctl；内部契约 opsctl）

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | `woopsctl` 二进制 | `go/cmd/woopsctl`（内部实现 `go/internal/opsctl`）：`upload` / `download` / `exec` / `forward` / `reverse`；env **`OPSCTL_CONFIG`** JSON（server/token/pin）；`server` 须 `https://`；pin 校 Gateway TLS；远端 exit code 透传；公开下载 `GET /bin/woopsctl/{os}/{arch}`（Docker 内置 linux amd64/arm64 + windows amd64） |
| `[x]` | 部署 Token | 表 `deploy_tokens`：绑 **单资产**、可选 `remark`、`allow_upload`/`allow_download`/`allow_exec`/`allow_forward`/`allow_reverse`（五项独立；旧 `allow_portmap` 启动时迁移后删除）、`expires_at` 可空=无限期、**直接删除**（与用户 API Token 一致；创建/删除记控制审计；旧软吊销行启动时清理）；forward/reverse 默认关闭；创建时返回 `opsctlConfig` / `opsctlConfigJson`（只一次）；SHA-256 存库；管理 API `/api/assets/{id}/deploy-tokens`；Console 资产详情右侧 **下载 woopsctl** 弹窗展示固定公开 URL（`/bin/woopsctl/{os}/{arch}`），可复制链接 / wget·curl 命令 / 本机下载（linux amd64·arm64 + windows amd64） |
| `[x]` | `woopsctl upload` / `download` / `exec` | Gateway 反代换票 → `/ws/file-transfer` 或 `/ws/exec`；上传下载与 Console 同一二进制协议（自动重连续传）；stderr 进度：换票/连接/已传总量/%/速度/已耗时/ETA（TTY 同行刷新）；Agent `sessions/exec` 流式输出；运行态：EXEC `OPERATION` + `RUN` ACTION（command/cwd/timeout，**不落 stdout/stderr**）+ END 带 `exitCode`/`durationMs` |
| `[x]` | GitLab CI 示例与文档 | `docs/woopsctl-gitlab-ci.md` |

---

## 9. 安全基线

上公网 / 过安全评审前先做本节 `[ ]`。产品残余（可见即 Agent 权限）不是实现漏，除非改 §0 / §3 ACL。

### 9.1 已具备

| 状态 | 功能 | 说明 |
|------|------|------|
| `[x]` | Agent token 哈希存储 | 注册换成长效凭据；BCrypt 存库 |
| `[x]` | 部署 Token 哈希 | SHA-256 存库；明文只创建时展示一次 |
| `[x]` | 用户密码 | BCrypt；本地登录强制 TOTP + 同一步长防重放；pending JWT 不能当会话 |
| `[x]` | Agent 凭据 header | 控制/会话/metrics WSS 用 Header；安装码公开短 TTL（15min，可多次使用，**8 字节 = 64 bit** 熵，在线爆破不可行） |
| `[x]` | 短时会话票据 | shell/文件/桌面/exec 约 90s；portmap 约 120s；发票前校验资产可见性 |
| `[x]` | 资产 API 不回传桌面密码 | 仅 `hasDesktopPassword` |
| `[x]` | 全站 TLS / 反代 | Gateway PUBLIC https/wss（Agent/woopsctl）；Console Docker HTTPS + nginx `/api/`→control-api（**不**转 `/api/internal`、`/api/sessions/internal`、`/api/opsctl`，404）、`/ws` `/i/` `/bin/`→Gateway **INTERNAL**（`:9201`）；浏览器会话 WS **同源改写** `rewriteWs`（避免自签 :9200 二次信任；`.env.example`「不再改写主机」注释已过时）。本地 Vite 开发代理仍把整个 `/api` 转到 9100（本机）。对内 control↔gateway 明文 http；Agent/woopsctl 用 pin 或公有 CA |
| `[x]` | Agent 验 Gateway | 有 pin 校 SPKI（`InsecureSkipVerify` 仅配合 pin 回调）；无 pin 走系统 CA；**禁止**关校验 |
| `[x]` | 公网 IP 不信 XFF（控制 WSS） | 上线写入用 TCP `RemoteAddr`。注册 HTTP `AgentController` **仍读 X-Forwarded-For**（仅当 9100 可从不可信网络访问时有意义；生产靠防火墙） |
| `[x]` | 录像取用防穿越 | `normalize()` 后必须仍在 `OPS_AUDIT_DIR` 根内 |
| `[x]` | 生产端口暴露 | 防火墙/安全组只放 **443**、**9200**（及正向 portmap 20000–21000）。compose 映射 9100/5432/4822、Gateway 听 `:9201` 是给宿主机/容器互调，**不改绑环回**（9201 绑环回会断 console）。无防火墙时这些口对公网可达，是部署问题不是代码洞 |
| `[x]` | Gateway 公网 / 内网 mux 分离 | **0.1.4 修**：`core` 有独立 `internalMux`；`RegisterInternal` 的路由（`/internal/portmap/{open,close,list,listening-ports}`，本身无鉴权、只供 control-api）**只挂内网 listener**，公网 `Handler()` 返回 404。此前 `cmd/gateway` 把同一 mux 同时交给 9200/9201，导致未认证公网调用者可枚举/关闭端口映射（泄露资产 UUID 与内网目标），并令 Gateway 在宿主 `0.0.0.0` 绑任意端口——放开 20000–21000 后即成未认证内网隧道。回归测试 `core/internal_routes_test.go` 锁死「内部路由不得出现在公网 handler」；新增内部路由**必须**用 `RegisterInternal` |
| `[x]` | 登录限速与 JWT 禁用即失效 | 本地登录/TOTP/GitLab 兑换：10 次失败锁 15 分钟（内存，按 IP+身份，429）。`JwtAuthFilter` 每次查库：禁用/软删 → 401；角色以库为准。发票走 `requireUser` |
| `[x]` | GitLab 回调一次性 code | 回调 JWT 存内存 60s；跳转 `/login?code=`；`POST /api/auth/gitlab/exchange` 兑换。不再把 JWT 放 query |
| `[~]` | 录像目录权限与校验和 | 日分区 `0755`、录像叶子 `0777`（供 guacd）、cast `0640`；END 记 sha256+size。**缺**独立配额 |

### 9.2 待修复（上线门槛）

| 状态 | 功能 | 说明 |
|------|------|------|
| `[~]` | 内部 API 鉴权与反代隔离 | **Console nginx 已 404：** `/api/internal`、`/api/sessions/internal`、`/api/opsctl`（`deploy/nginx.conf`；woopsctl 仍走 Gateway PUBLIC）。Gateway→Java 仍直连 `:9100`。**仍缺：** Java 内部口 `permitAll`（本机/内网直打 9100 仍可验票）；Vite 开发代理未排除；验票响应仍带桌面密码 |
| `[ ]` | 凭据密文库 | `desktop_password`、TOTP secret 现明文/半明文；票据内带桌面密码。按 §3：AES-GCM + 主密钥；Gateway 按票据向 Java 取一次性凭据，不把长期密码放进可被 internal 验票读出的 JWT |
| `[ ]` | 密码策略 | 无最小长度/复杂度；生产不拒绝未改的 bootstrap `admin123` |
| `[ ]` | 端口映射目标 denylist / 正向暴露 | 反向 SSRF：Gateway 可拨任意可达地址（含 Docker `127.0.0.1`）。正向默认 listen `0.0.0.0` 把 20000–21000 挂到宿主公网网卡。**要做：** 至少拒绝 metadata/loopback/link-local（反向）；正向 listen 生产默认本机或须显式确认。woopsctl 临时映射已有独立 scope，仍无 host/port allowlist |
| `[ ]` | 文件路径沙箱 | 见 §5.3；无沙箱时可见用户 = 目标机 Agent 用户（通常 root/SYSTEM）可读改全盘 |
| `[ ]` | 安全响应头 | nginx/Java 无 CSP / HSTS / X-Frame-Options。JWT 在 `localStorage`，XSS 即可偷会话（当前未见 `v-html`，面较小） |

### 9.3 产品残余（不做，除非改 §0 / §3）

| 状态 | 说明 |
|------|------|
| `[!]` | **可见资产 ≈ 目标机 Agent 权限。** MEMBER 可见即可 Shell / 文件 / 桌面 / exec / 一键更新 / 部署 Token / 端口映射。不做协议或动作细 ACL。人员与 `user_scopes` 是唯一收口 |
| `[!]` | GitLab 登录路径无堡垒侧 2FA（§0）；信任 IdP 是否开 2FA |
| `[!]` | 安装码 15min 不限次数（熵够，不按一次性码做） |

---

## 10. 本期明确不做

除非改本节，否则不要做：

- Redis、消息队列、微服务拆分、多 Gateway HA 调度  
- Gateway 单 WSS 多路复用 / lane / QUIC / 自定义 RPC（`proxyBridge` 仅在 Agent↔Agent TCP carrier 内用 yamux 承载彼此独立的标准 HTTP Proxy 连接，不改变一会话一 WSS）
- 专用 TSDB / ClickHouse（监控已用 **TimescaleDB** history+trends）、告警通知渠道（邮件/Webhook）、进程列表/Top、GPU/温度/磁盘 util%  
- 端口映射：Gateway 冷启动不主动拉全量清单（恢复见 §6 Agent 上线）  
- Agent 自动升级平台（控制台一键更新 + 手工重装即可）  
- SFTP 文件管理（主路径为 Agent 原生 filemanager/filetransfer）  
- 协议 / 动作细 ACL（可见即可开全部会话；见 §3 / §9.3）  
- RDP/VNC 转 MP4（一期 Guacamole 原生回放）  
- 通用命令解析级审计（会话流 + CI 命令 + 文件操作即可）  
- Agent 自采屏远程桌面（DXGI/X11 注入）  

---

## 11. 架构要点

```
Browser ──WSS──► Gateway ──WSS──► Agent
                     │
                     ├── shell：Agent 本地进程（原生）
                     ├── filemanager：目录/元数据 JSON-RPC
                     ├── filetransfer：独立数据 WSS，二进制上传/下载（临时文件 + 断点）
                     ├── rdp/vnc：隧道 → 127.0.0.1 系统端口 + guacd
                     ├── portmap 正向：Gateway 听端口 → 会话 WSS → Agent 拨目标
                     ├── portmap 反向：Agent 听端口 → 换票建会话 WSS → Gateway 拨目标
                     ├── metrics（插件）：独立 WSS → Java 扁表时序 / 预警
                     └── 控制面：Java（认证、资产、票据、RBAC、审计元数据）；控制 WSS 仅信令（含 open_session / abort_transfer）
```

- **一会话一数据 WSS**；监控另开 **metrics WSS**（可关）；Gateway **不直连 PostgreSQL**  
- 无 Redis / MinIO；运行态 JSONL + 录像落本地磁盘 `OPS_AUDIT_DIR`（Gateway 写、control-api tail）  
- Java 不碰实时字节流  
- 文件内容**不**走 filemanager，也**不**走控制 WSS  

**网闸多级内网（代理）：**

- 下层可主动连上层：A（有外网）开 `proxy.*`；B 的 `gatewayProxy=A`，WSS 经 A 出站；B 也可再开 `proxy.*` 给 C 串联。
- 只有上层能主动连下层：B 开 `proxy.*` + `proxyBridge.listen`，`gatewayProxy` 指向 B 本机 proxy；A 开 `proxyBridge.targets=[B]`（普通 `proxy.enabled` 可关）。A→B 建 carrier 后，B 的 HTTP CONNECT 反向复用载波到 A 出口；中间节点可同时 listen+targets 级联。Gateway 仍只见 B 端到端 HTTPS/WSS。

---

## 12. 维护规则

1. **开工前**只读本文。  
2. **完工后**更新状态与说明，并改「最后更新」日期。  
3. **改产品决策**先改 §0，再改代码。  
4. README 只链到本文。  

---

## 13. 相关路径

| 区域 | 路径 |
|------|------|
| 本清单（唯一） | `FEATURES.md` |
| 控制面 | `apps/control-api/`（`AssetService`/`AssetAccess`、`group/`、`session/`+`protocol/{shell,filemanager,filetransfer,exec,desktop,portmap}`、`controlaudit/`、`assetevent/`、`serverops/`、`ci/`、`common/PageSupport`） |
| 控制台 | `apps/console/src/{session,features,modules,shared,i18n}/`；`features/registry.js` 静态组合全部 feature 路由与资产会话动作；`session/` 仅通用会话 chrome；实现位于 `features/{shell,filemanager,filetransfer,exec,desktop,monitor,portmap,audit}`，其中审计细分 `{control,operations,assetevents,shared}`；`modules/` 仅 **应**保留 `assets/auth/users`（旧 `modules/{sessions,monitor,portmaps,ops-audit,audit,asset-events}` 待删，见 §1）；共享 `AssetTreeSelect`；文案 `i18n/{en,zh}.js` |
| Gateway | `go/internal/gateway/{app,core,sessions,services,plugins,infra}/`（**cmd 入口**）；`sessions/{shell,filemanager,filetransfer,exec,desktop}`、`services/{portmap,agentinstall}`、`plugins/monitor`；`infra/audit` 为跨切面设施；desktop 私有拥有 `sessions/desktop/guac`。旧顶层 `server.go` / `opsaudit` / `installscripts` 等仍在仓库，待删（见 §1） |
| 共享线协议 | `go/internal/protocol/{control,monitor,filetransfer,datagram}/`（控制/指标/FileTransfer Binary/UDP datagram） |
| 出站 TLS/代理 | `go/internal/tlsutil/`（`DialOptions` → `HTTPClient` / `WSDialer`；Agent/opsctl 共用） |
| 主机信息 | `go/internal/hostinfo/netiface/`（Agent netinfo 与 monitor 共用真实网卡筛选） |
| Agent | `go/internal/agent/{app,core,sessions,services,plugins,infra,sessionreg}/`；`app` 唯一组合，`core` 为 Runtime/Config/control WSS/session/netinfo/TLS，`sessions/{shell,filemanager,filetransfer,exec,socket}`、`services/portmap`、`plugins/{monitor,proxy}` 拥有功能，`infra/applog` 为基础设施；`cmd/agent` 只依赖 `app`+`infra/applog` |
| woopsctl | `go/cmd/woopsctl`、`go/internal/opsctl/`（内部命名保留，含 `xferclient.go`/端口映射客户端）；文档 `docs/woopsctl-gitlab-ci.md` |
| 监控控制面 | `apps/control-api/.../metrics/` |
| 用户 API Token | `apps/control-api/.../apitoken/`；Console `features/profile/` |
| 部署 | 根目录 `docker-compose.yml`（Hub 镜像）；`deploy/docker-compose.yml`（源码构建，含 guacd profile）；Gateway 自签名 TLS：`deploy/gen-gateway-tls.sh` |
