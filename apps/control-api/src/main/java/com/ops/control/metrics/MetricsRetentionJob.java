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

    /** Daily purge of history beyond detail retention and trends beyond trends retention. */
    @Scheduled(cron = "0 30 3 * * *")
    public void purge() {
        int history = metrics.purgeHistoryOlderThanRetention();
        if (history > 0) {
            log.info("purged {} monitor_history rows beyond detail retention", history);
        }
        int trends = metrics.purgeTrendsOlderThanRetention();
        if (trends > 0) {
            log.info("purged {} monitor_trends rows beyond trends retention", trends);
        }
    }
}
