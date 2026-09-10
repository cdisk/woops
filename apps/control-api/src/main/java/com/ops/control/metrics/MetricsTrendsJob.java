package com.ops.control.metrics;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Component
@ConditionalOnProperty(prefix = "ops.monitor", name = "trends-enabled", havingValue = "true", matchIfMissing = true)
public class MetricsTrendsJob {
    private static final Logger log = LoggerFactory.getLogger(MetricsTrendsJob.class);
    private final MetricsService metrics;

    public MetricsTrendsJob(MetricsService metrics) {
        this.metrics = metrics;
    }

    /** Roll up the previous completed UTC hour into monitor_trends. */
    @Scheduled(cron = "0 5 * * * *")
    public void rollup() {
        int n = metrics.rollupPreviousHour();
        if (n > 0) {
            log.info("rolled up {} monitor_trends rows for previous hour", n);
        }
    }
}
