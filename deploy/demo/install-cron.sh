#!/usr/bin/env bash
# 装上每小时整点的 Demo 重置任务。幂等，可重复执行。
set -euo pipefail

OPS_DIR=${OPS_DIR:-/opt/ops}
LOG=/var/log/woops-demo-reset.log

cat > /etc/cron.d/woops-demo-reset <<EOF
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
0 * * * * root OPS_DIR=${OPS_DIR} /bin/bash ${OPS_DIR}/deploy/demo/reset.sh >> ${LOG} 2>&1
EOF
chmod 644 /etc/cron.d/woops-demo-reset

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
