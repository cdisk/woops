#!/usr/bin/env bash
# 演示环境宿主机初始化：Docker、socat（acme.sh standalone 需要）、acme.sh。
# 幂等，可重复执行。用法：bash bootstrap-host.sh
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

echo "==> 基础包"
apt-get update -qq
apt-get install -y -qq ca-certificates curl socat git cron jq openssl >/dev/null

if ! command -v docker >/dev/null 2>&1; then
  echo "==> Docker"
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  codename="$(. /etc/os-release && echo "$VERSION_CODENAME")"
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian ${codename} stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin >/dev/null
  systemctl enable --now docker
else
  echo "==> Docker 已存在，跳过"
fi

if [ ! -d /root/.acme.sh ]; then
  echo "==> acme.sh"
  curl -fsSL https://get.acme.sh | sh -s email="${ACME_EMAIL:-admin@tool4dev.net}" >/dev/null
  /root/.acme.sh/acme.sh --set-default-ca --server letsencrypt >/dev/null
else
  echo "==> acme.sh 已存在，跳过"
fi

systemctl enable --now cron >/dev/null 2>&1 || true

echo "==> 版本"
docker --version
docker compose version
/root/.acme.sh/acme.sh --version | tail -2
echo "==> 完成"
