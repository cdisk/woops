#!/usr/bin/env bash
# 为演示环境签发 / 安装 Let's Encrypt 证书（standalone 模式，占用 80 端口）。
# Console(443) 与 Gateway(9200) 共用同一对 deploy/tls/gateway.{crt,key}。
#
# acme.sh 续期时复用同一把私钥，但我们并不依赖这点：.env 里置 OPS_TLS_TRUSTED_CA=1
# 后 pin 为空，Agent 走系统 CA，续期换不换密钥都不影响已上线的 Agent。
#
# 用法：DOMAIN=woops-demo.tool4dev.net bash issue-cert.sh
set -euo pipefail

DOMAIN="${DOMAIN:?需要 DOMAIN}"
OPS_DIR="${OPS_DIR:-/opt/ops}"
TLS_DIR="$OPS_DIR/deploy/tls"
ACME="/root/.acme.sh/acme.sh"

[ -x "$ACME" ] || { echo "ERROR: 未找到 $ACME，先跑 bootstrap-host.sh" >&2; exit 1; }

mkdir -p "$TLS_DIR"

# 80 端口必须空闲：standalone 要自己起监听完成 HTTP-01 校验。
if ss -lnt "( sport = :80 )" | grep -q LISTEN; then
  echo "ERROR: 80 端口被占用，standalone 校验会失败：" >&2
  ss -lntp "( sport = :80 )" >&2
  exit 1
fi

if [ ! -d "/root/.acme.sh/$DOMAIN" ] && [ ! -d "/root/.acme.sh/${DOMAIN}_ecc" ]; then
  echo "==> 首次签发 $DOMAIN"
  "$ACME" --issue -d "$DOMAIN" --standalone --keylength ec-256
else
  echo "==> $DOMAIN 已签发过，跳过 issue（续期由 acme.sh 的 cron 负责）"
fi

# reloadcmd 在每次续期后执行：重启读证书的两个服务。
# console 是 nginx（证书在 443），gateway 走 host 网络（证书在 9200）。
echo "==> 安装证书到 $TLS_DIR"
"$ACME" --install-cert -d "$DOMAIN" --ecc \
  --fullchain-file "$TLS_DIR/gateway.crt" \
  --key-file       "$TLS_DIR/gateway.key" \
  --reloadcmd      "cd $OPS_DIR && docker compose -f docker-compose.yml -f deploy/demo/docker-compose.demo.yml --env-file .env --profile full --profile desktop restart console gateway"

chmod 600 "$TLS_DIR/gateway.key"

echo "==> 证书信息"
openssl x509 -in "$TLS_DIR/gateway.crt" -noout -subject -issuer -dates
echo "==> 完成。acme.sh 已自动装好续期 cron："
crontab -l 2>/dev/null | grep -i acme || true
