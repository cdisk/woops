package com.ops.control.metrics;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "ops.monitor")
public record MonitorProperties(
        /** Keep minute-level history this many days. Env: OPS_MONITOR_DETAIL_RETENTION_DAYS */
        int detailRetentionDays,
        /** Keep hourly trends this many days. Env: OPS_MONITOR_TRENDS_RETENTION_DAYS */
        int trendsRetentionDays,
        /** When false, skip hourly trends rollup job. Env: OPS_MONITOR_TRENDS_ENABLED */
        boolean trendsEnabled
) {
    public MonitorProperties {
        if (detailRetentionDays <= 0) {
            detailRetentionDays = 7;
        }
        if (trendsRetentionDays <= 0) {
            trendsRetentionDays = 1095;
        }
    }
}
