package com.ops.control.metrics;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Component
public class MetricsRetentionJob {
    private static final Logger log = LoggerFactory.getLogger(MetricsRetentionJob.class);
    private final MetricsService metrics;

    public MetricsRetentionJob(MetricsService metrics) {
        this.metrics = metrics;
    }

    /** Daily purge of points older than 3 years. */
    @Scheduled(cron = "0 30 3 * * *")
    public void purge() {
        int n = metrics.purgeOlderThanThreeYears();
        if (n > 0) {
            log.info("purged {} monitor_data rows older than 3 years", n);
        }
    }
}
