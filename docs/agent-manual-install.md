# Agent 手动安装（Linux）

> 适用于：**目标机不能跑 `curl | bash`**（无出网、只能 U 盘/跳板拷贝）、**Proxy Bridge 下游 B**、或需要手工控步骤的装机。  
> 在线一键安装仍见 [README §4 方法 1](../README.md#4-接入一台主机agent)。

---

## 1. 适用场景

| 场景 | 说明 |
|------|------|
| 完全离线 | 在有网机器下载二进制 + 安装码，拷到目标机 |
| 只能经 Bridge 出 Gateway | B 开 `proxy.*` + `proxyBridge.listen`，A 开 `proxyBridge.targets`（见 [§6](#6-编写-agentyaml)） |
| 架构不匹配 | 目标机 `uname -m` 为 `aarch64` 须用 **arm64** 包，`x86_64` 用 **amd64**（Docker Gateway 镜像内两者均有） |

手动安装等价于 `woops-agent install` 做的事：**放二进制 → 写/合并 `agent.yaml`（刷新 gateway+pin，保留本机 proxy）→ 写 `install-code` → 注册服务 → Agent 自注册**。也可直接对已拷到目标机的二进制执行：

```bash
chmod +x /path/to/woops-agent
/path/to/woops-agent install -gateway https://woops.example.com:9200 -code '<安装码>' -pin '<hex pin>'
```

---

## 2. 安装码从哪来

1. 控制台 → **资产** → 左侧选中目标**分组**（「全部」不能发码）→ **生成安装链接**。
2. 弹窗顶部有 **「安装码」** 一行，点 **复制** 即可（离线手工装用这个）。
3. 也可从安装链接 URL 中取中间段：  
   `https://<gateway>/i/<安装码>/agent/linux/amd64` → `<安装码>` 即所需字符串。
4. 安装码约 **15 分钟**有效；过期在控制台重新生成，替换目标机 `/etc/woops-agent/install-code` 后重启 Agent。
5. 须绑定分组；注册成功后 Agent 会删除 `install-code`，并写入 `asset-id`、`agent-token`。

---

## 3. 获取 Gateway 与 TLS pin

从 Woops 服务器 `.env` 或运维处取得（与安装脚本一致）：

- **Gateway 公网地址**：`OPS_GATEWAY_PUBLIC_HTTP`，如 `https://woops.example.com:9200`（保留 `https://`）
- **SPKI pin**：`OPS_GATEWAY_TLS_SPKI_SHA256`（64 位十六进制，自签证书必填）

写入 `agent.yaml` 的 `gateway`、`gatewayTlsSpkiSha256`。

---

## 4. 获取 Agent 二进制

产物名：`woops-agent-linux-amd64` 或 `woops-agent-linux-arm64`（静态链接，无需额外依赖）。

### 4.1 从 Docker Gateway 容器复制（推荐）

在部署 Gateway 的宿主机上：

```bash
# 查看容器名（一般为 deploy-gateway-1）
docker ps --filter name=gateway

# amd64
docker cp deploy-gateway-1:/app/bin/woops-agent-linux-amd64 ./woops-agent-linux-amd64

# arm64（ARM 服务器必用）
docker cp deploy-gateway-1:/app/bin/woops-agent-linux-arm64 ./woops-agent-linux-arm64

chmod +x woops-agent-linux-*
file woops-agent-linux-arm64   # 应含 ARM aarch64
file woops-agent-linux-amd64   # 应含 x86-64
```

`.gz`  sidecar 同目录，离线拷 raw 二进制即可。

### 4.2 有网时从 Gateway 下载（需有效安装码）

```bash
CODE='粘贴安装码'
PIN_B64='sha256//…'   # 与 .env pin 对应的 base64，见 gen-gateway-tls.sh 输出
GW='https://woops.example.com:9200'

# amd64
curl -fsSL -k --pinnedpubkey "$PIN_B64" \
  "$GW/i/$CODE/agent/linux/amd64?format=gz" | gzip -dc > woops-agent-linux-amd64

# arm64
curl -fsSL -k --pinnedpubkey "$PIN_B64" \
  "$GW/i/$CODE/agent/linux/arm64?format=gz" | gzip -dc > woops-agent-linux-arm64

chmod +x woops-agent-linux-*
```

### 4.3 本机交叉编译（可选）

```bash
cd go
docker run --rm -v "$PWD:/src" -w /src golang:1.26-alpine sh -c '
  apk add --no-cache git && go mod download &&
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags="-s -w -X main.Version=$(date +%y%m%d%H%M)" -trimpath \
    -o /src/bin/woops-agent-linux-arm64 ./cmd/agent
'
```

---

## 5. 目标机目录与文件

| 路径 | 权限 | 说明 |
|------|------|------|
| `/usr/local/bin/woops-agent` | `0755` | 二进制（**必须可执行**） |
| `/etc/woops-agent/agent.yaml` | `0640` | Gateway、proxy、proxyBridge 等 |
| `/etc/woops-agent/install-code` | `0600` | 一行安装码（首次安装） |
| `/etc/woops-agent/asset-id` | `0600` | 注册后自动生成，重装可保留 |
| `/etc/woops-agent/agent-token` | `0600` | 注册后自动生成，重装会轮换 |
| `/var/log/woops-agent/woops-agent.log` | — | 控制面、注册、bootstrap |
| `/var/log/woops-agent/proxy.log` | — | proxy + proxyBridge（启用时） |

---

## 6. 编写 agent.yaml

参考 [`go/agent.example.yaml`](../go/agent.example.yaml)。

### 6.1 能直连 Gateway（普通内网）

```yaml
gateway: "https://woops.example.com:9200"
gatewayTlsSpkiSha256: "ab6d63fc5186acb791dd07513806954b9448de9860a5825d37e3ae3c2a7fd56d"
```

### 6.2 Bridge 下游 B（无出网，经 A 的 Bridge）

B 上 **须** 同时开本机 proxy，且 `gatewayProxy` 指向本机 proxy：

```yaml
gateway: "https://woops.example.com:9200"
gatewayTlsSpkiSha256: "…"

gatewayProxy: "http://proxyuser:changeme@127.0.0.1:3128"

proxy:
  enabled: true
  listen: "0.0.0.0:3128"
  username: "proxyuser"
  password: "changeme"
  allowGlobal: false

proxyBridge:
  enabled: true
  listen: "0.0.0.0:3999"
  key: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  allowGlobal: false
```

- `key`：64 位十六进制，与 A 的 `targets[].key` **一致**（`openssl rand -hex 32`）。
- **先** 在 A 配好 `proxyBridge.targets` 并重启 A，**再** 启 B，否则注册会 `parent_unavailable`。

### 6.3 Bridge 上游 A（主动连 B）

```yaml
gateway: "https://woops.example.com:9200"
gatewayTlsSpkiSha256: "…"

proxy:
  enabled: false

proxyBridge:
  enabled: true
  allowGlobal: false
  targets:
    - address: "10.0.1.2:3999"
      key: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    # 多台 B 继续追加：
    # - address: "10.0.1.3:3999"
    #   key: "fedcba…"
```

---

## 7. 编写 install-code

内容**仅一行**安装码，无引号、无空格：

```bash
install -d -m 0750 /etc/woops-agent
printf '%s\n' '你的安装码' > /etc/woops-agent/install-code
chmod 0600 /etc/woops-agent/install-code
cat -A /etc/woops-agent/install-code   # 确认只有一行
```

若已有 `asset-id`（重装）：保留该文件，Agent 会用旧 id 迁组并轮换 token；仍建议写入**新**安装码以指定分组。

---

## 8. 安装二进制与 systemd

```bash
install -d -m 0750 /etc/woops-agent /var/log/woops-agent

# 按架构选二进制
install -m 0755 woops-agent-linux-arm64 /usr/local/bin/woops-agent
# 或: install -m 0755 woops-agent-linux-amd64 /usr/local/bin/woops-agent

install -m 0640 agent.yaml /etc/woops-agent/agent.yaml

cat > /etc/systemd/system/woops-agent.service <<'EOF'
[Unit]
Description=Woops Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=OPS_AGENT_CONFIG=/etc/woops-agent/agent.yaml
ExecStart=/usr/local/bin/woops-agent -config /etc/woops-agent/agent.yaml
WorkingDirectory=/
Restart=always
RestartSec=3
KillMode=process
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable woops-agent
systemctl start woops-agent
```

无 systemd 时：

```bash
nohup /usr/local/bin/woops-agent -config /etc/woops-agent/agent.yaml >>/var/log/woops-agent/woops-agent.log 2>&1 &
```

---

## 9. 启动后检查

```bash
# 服务状态
systemctl status woops-agent

# 端口（Bridge B 示例：3999 bridge，3128 proxy）
ss -ntlp | grep -E '3999|3128|woops-agent'

# 日志
journalctl -u woops-agent -f
tail -f /var/log/woops-agent/woops-agent.log
tail -f /var/log/woops-agent/proxy.log
```

### 成功标志

| 检查项 | 期望 |
|--------|------|
| `ss` | proxy / bridge 端口在 LISTEN |
| `proxy.log` | `proxy bridge parent connected`（B 上，A 已连） |
| 凭据 | 存在 `asset-id`、`agent-token`，**无** `install-code` |
| 控制台 | 对应分组主机 **在线** |

### 常见日志

| 日志 | 含义 |
|------|------|
| `203/EXEC` 或 `Permission denied` | 二进制无 `+x`，或架构错误（amd64 装到 arm64） |
| `address already in use` | 端口冲突；`systemctl stop` + `pkill -9 -x woops-agent` 后重启 |
| `parent_unavailable` | A 的 Bridge 尚未连上 B |
| `register rejected: 400` | 安装码无效/过期；换码并重写 `install-code` |
| `register request failed` | 仍无通路到 Gateway（proxy/bridge 未就绪） |

---

## 10. 重装与升级

| 操作 | 做法 |
|------|------|
| 保留身份重装 | 保留 `asset-id`，换新 `install-code`，替换二进制，重启 |
| 仅升级二进制 | 覆盖 `/usr/local/bin/woops-agent`，`chmod 755`，`systemctl restart` |
| 在线一键更新 | 控制台 **一键更新**（仍走 exec + 安装脚本，非本文档流程） |

---

## 11. 相关文档

- [README §4 — 在线安装](../README.md#4-接入一台主机agent)
- [`go/agent.example.yaml`](../go/agent.example.yaml) — 配置注释
- [FEATURES.md](../FEATURES.md) — Proxy Bridge 与自注册契约
