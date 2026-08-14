# woopsctl / GitLab CI

## 1. 创建部署凭据

1. 控制台打开资产详情 → 部署 Token → 创建（按需勾选 upload / download / exec / forward / reverse，可选过期时间与备注）。
2. 明文 **OPSCTL_CONFIG** 只展示一次，复制到 GitLab **masked / protected** 变量 `OPSCTL_CONFIG`。

示例 JSON：

```json
{
  "server": "https://ops.example.com:9200",
  "token": "ops_<uuid>_<secret>",
  "pin": "<gateway-tls-spki-sha256-hex>"
}
```

- `server`：Gateway 的 **https** 基址（换票仍走内部契约 `/api/opsctl/tickets`）。
- `token`：部署 Token 明文。
- `pin`：Gateway 证书 SPKI SHA-256（自签名时必填；公有 CA 可为空字符串）。与 `OPS_GATEWAY_TLS_SPKI_SHA256` 同源。

旧变量 `OPSCTL_SERVER` / `OPSCTL_TOKEN` **已删除**，不再支持。

## 2. 本地试跑

从控制台「部署 Token」旁下载 **woopsctl**（Linux / Windows amd64），或直接拉取：

```bash
# Gateway PUBLIC，或经 Console nginx 同源 /bin/
curl -fsSL "https://ops.example.com:9200/bin/woopsctl/linux/amd64" -o woopsctl
chmod +x woopsctl
```

```bash
export OPSCTL_CONFIG='{"server":"https://ops.example.com:9200","token":"ops_…","pin":"…"}'

./woopsctl upload ./dist/app.jar /opt/app/app.jar
./woopsctl download /opt/app/app.jar ./app.jar
./woopsctl exec --cwd /opt/app --timeout 2m -- 'systemctl restart myapp'
```

上传/下载过程会在 stderr 打印进度（换票、连接、已传/总量、百分比、速度、已耗时、ETA）；TTY 下传输行会同行刷新。断线后自动从 Agent 已确认 offset 续传。

## 3. 临时端口映射

部署 Token 必须勾选 `forward`。正向监听位于 woopsctl 本机，目标由 Agent 拨号：

```bash
woopsctl forward --protocol tcp --listen 127.0.0.1:15432 --target 10.0.0.8:5432
woopsctl forward --protocol udp --listen 127.0.0.1:15353 --target 10.0.0.53:53
```

反向监听位于 Agent，目标由 woopsctl 本机拨号（Token 须勾选 `reverse`）：

```bash
woopsctl reverse --protocol tcp --listen 127.0.0.1:18080 --target 127.0.0.1:8080
woopsctl reverse --protocol udp --listen 127.0.0.1:15353 --target 127.0.0.1:5353
```

命令以前台方式持续运行。Gateway 重启、网络闪断或 Agent 暂时离线时会自动重试并恢复映射，无需重启 woopsctl；已有 TCP 连接可能断开，恢复后新连接可用。只有 Ctrl+C、配置错误或 Token 失效等永久错误才退出。显式使用 `0.0.0.0` 会把监听端口暴露到对应主机网络。

## 4. GitLab CI 示例

```yaml
deploy:
  stage: deploy
  image: alpine:3.20
  variables:
    # OPSCTL_CONFIG 在 CI/CD Variables 中配置（masked / protected）
  script:
    - apk add --no-cache curl ca-certificates
    - curl -fsSL "https://ops.example.com:9200/bin/woopsctl/linux/amd64" -o /usr/local/bin/woopsctl
    - chmod +x /usr/local/bin/woopsctl
    - woopsctl upload "$CI_PROJECT_DIR/dist/app.jar" /opt/app/app.jar
    - woopsctl exec --cwd /opt/app --timeout 5m -- 'systemctl restart myapp && systemctl is-active myapp'
```

> 也可把 `woopsctl` 打进 CI 镜像或内网制品库。公开下载路径：`GET /bin/woopsctl/{os}/{arch}`（当前 Docker Gateway 内置 `linux/amd64` + `windows/amd64`）；经 Console 时同源 `/bin/` 由 nginx 反代到 Gateway。
