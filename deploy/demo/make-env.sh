#!/usr/bin/env bash
# 生成演示环境的 /opt/ops/.env（随机密钥，不进版本库）。
# 已存在则不动，避免重跑时把密钥换掉导致已签发的 JWT/票据失效。
# 强制重写：FORCE=1 bash make-env.sh
#
# 用法：DOMAIN=woops-demo.tool4dev.net bash make-env.sh
set -euo pipefail

DOMAIN="${DOMAIN:?需要 DOMAIN}"
OPS_DIR="${OPS_DIR:-/opt/ops}"
ENV_FILE="$OPS_DIR/.env"
TAG="${WOOPS_IMAGE_TAG:-0.1.8}"

if [ -f "$ENV_FILE" ] && [ "${FORCE:-0}" != "1" ]; then
  echo "==> $ENV_FILE 已存在，跳过（FORCE=1 可重写）"
  exit 0
fi

rand() { openssl rand -base64 36 | tr -d '\n=+/' | cut -c1-"${1:-32}"; }

ADMIN_PASSWORD="$(rand 20)"

umask 077
cat > "$ENV_FILE" <<EOF
# Woops 演示环境。由 deploy/demo/make-env.sh 生成，勿提交。
WOOPS_IMAGE_TAG=${TAG}

# 供 deploy/demo/docker-compose.demo.yml 给 Agent 容器做 extra_hosts 用
OPS_DEMO_DOMAIN=${DOMAIN}

# ========== PUBLIC ==========
# 浏览器只访问 443；9200 只服务 Agent / 安装脚本 / woopsctl。
OPS_CONSOLE_PUBLIC_HTTP=https://${DOMAIN}
OPS_CONTROL_PUBLIC_HTTP=https://${DOMAIN}
OPS_GATEWAY_PUBLIC_HTTP=https://${DOMAIN}:9200
OPS_GATEWAY_PUBLIC_WS=wss://${DOMAIN}:9200

# ========== INTERNAL ==========
OPS_GATEWAY_INTERNAL_HTTP=http://127.0.0.1:9201
OPS_CONTROL_INTERNAL_HTTP=http://127.0.0.1:9100

# ========== Gateway 监听 ==========
OPS_GATEWAY_PUBLIC_LISTEN=:9200
OPS_GATEWAY_INTERNAL_LISTEN=:9201
OPS_AGENT_BIN_DIR=bin

# ========== TLS ==========
# 证书由 acme.sh 签发装到 deploy/tls/gateway.{crt,key}（见 deploy/demo/issue-cert.sh）。
# TRUSTED_CA=1 让 tls-init 写空 pin：Agent 走系统 CA，续期换密钥也不会掉线。
OPS_TLS_TRUSTED_CA=1
OPS_GATEWAY_TLS_CERT=/tls/gateway.crt
OPS_GATEWAY_TLS_KEY=/tls/gateway.key
OPS_GATEWAY_TLS_SPKI_SHA256=

# ========== 桌面 ==========
OPS_GUACD_ADDR=127.0.0.1:4822
OPS_GUAC_BRIDGE_HOST=host.docker.internal

# ========== 密钥与初始管理员 ==========
OPS_JWT_SECRET=$(rand 48)
OPS_TICKET_SECRET=$(rand 48)
OPS_BOOTSTRAP_ADMIN_USERNAME=admin
OPS_BOOTSTRAP_ADMIN_PASSWORD=${ADMIN_PASSWORD}

# ========== 演示环境特有 ==========
# 审计与录制只留 1 天，省磁盘也少留访客痕迹。
OPS_AUDIT_RETENTION_DAYS=1
EOF

chmod 600 "$ENV_FILE"
echo "==> 已写 $ENV_FILE"
echo
echo "    管理员：admin / ${ADMIN_PASSWORD}"
echo "    （首次登录必须绑定 TOTP，请立刻记下这个密码）"
