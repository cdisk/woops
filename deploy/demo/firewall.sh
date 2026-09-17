#!/usr/bin/env bash
# 演示环境防火墙。幂等，可重复执行。
#
# 对外只留 22 / 80 / 443 / 9200：
#   80   acme.sh standalone 续期校验
#   443  Console
#   9200 Gateway PUBLIC（Agent / 安装脚本 / woopsctl）
# 特别要挡住的是 Gateway 的 9201 INTERNAL——它走 host 网络，直接监听在 0.0.0.0，
# 本身无鉴权，只该给 control-api 用。
#
# 另外掐断演示 Agent 容器的出网：子网 FORWARD 全丢，只剩「容器 → 宿主机」
# （那条走 INPUT 不经 FORWARD），够连 Gateway，但装不了包也传不出东西。
#
# 注意：Docker 发布的端口经 nat/FORWARD，不过 INPUT，所以这里的 INPUT 策略
# 不会误伤 console 的 443；反过来也意味着光靠 INPUT 挡不住容器发布的端口，
# 所以根 compose 发布的 5432/9100/4822 是在 docker-compose.demo.yml 里撤掉的。
set -euo pipefail

AGENT_SUBNET="${AGENT_SUBNET:-172.31.66.0/24}"

echo "==> INPUT"
# 先放开再重建，避免清空到加回规则之间把自己的 SSH 关在门外。
iptables -P INPUT ACCEPT
iptables -F INPUT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -p icmp --icmp-type echo-request -j ACCEPT
iptables -A INPUT -p tcp -m multiport --dports 22,80,443,9200 -j ACCEPT
iptables -P INPUT DROP

echo "==> DOCKER-USER（演示 Agent 出网）"
if iptables -L DOCKER-USER -n >/dev/null 2>&1; then
  iptables -F DOCKER-USER
  iptables -A DOCKER-USER -s "$AGENT_SUBNET" -d "$AGENT_SUBNET" -j RETURN
  iptables -A DOCKER-USER -s "$AGENT_SUBNET" -j DROP
  iptables -A DOCKER-USER -j RETURN
else
  echo "    DOCKER-USER 链不存在（Docker 未启动？），跳过" >&2
fi

echo "==> 当前规则"
iptables -L INPUT -n --line-numbers
iptables -L DOCKER-USER -n --line-numbers 2>/dev/null || true
