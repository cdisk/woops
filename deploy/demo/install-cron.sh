#!/usr/bin/env bash
# 装上每小时整点的 Demo 重置任务。幂等，可重复执行。
set -euo pipefail

OPS_DIR=${OPS_DIR:-/opt/ops}
LOG=/var/log/woops-demo-reset.log
LOCK=/tmp/woops-demo-reset.lock

# flock 不是可选的：reset.sh 会对 compose project 做 stop / recreate，两个实例
# 同时跑会互相抢容器改名，留下形如 <12位id>_ops-demo-broker-1 的残壳，之后每个
# 整点都 "name is already in use" 而失败。手动 compose 操作也要拿同一把锁。
cat > /etc/cron.d/woops-demo-reset <<EOF
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
0 * * * * root OPS_DIR=${OPS_DIR} /usr/bin/flock -n ${LOCK} /bin/bash ${OPS_DIR}/deploy/demo/reset.sh >> ${LOG} 2>&1
EOF
chmod 644 /etc/cron.d/woops-demo-reset

# 早期装过一条没有 flock 的 root crontab 条目，和上面这条同时在整点触发：两个
# reset 并发跑，既是残壳的来源，也让日志里每行都出现两遍。见到就摘掉。
if crontab -l 2>/dev/null | grep -q 'deploy/demo/reset\.sh'; then
  # grep -v 把所有行都滤掉时返回 1，pipefail 下会连带 set -e 中止，所以兜一下。
  kept=$(crontab -l 2>/dev/null | grep -v 'deploy/demo/reset\.sh' || true)
  printf '%s' "${kept:+$kept$'\n'}" | crontab -
  echo "已从 root crontab 摘掉重复的 reset 条目（改由 /etc/cron.d 单独负责）"
fi

cat > /etc/logrotate.d/woops-demo-reset <<EOF
${LOG} {
    weekly
    rotate 4
    compress
    missingok
    notifempty
}
EOF

echo "已安装 /etc/cron.d/woops-demo-reset（每小时整点），日志 ${LOG}"
