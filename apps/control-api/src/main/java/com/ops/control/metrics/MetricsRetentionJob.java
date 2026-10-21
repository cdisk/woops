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
        RetentionPurge.runBoth(this::purgeHistory, this::purgeTrends, ex ->
                log.warn("retention purge failed", ex));
    }

    private void purgeHistory() {
        purgeOne("monitor_history", metrics::purgeHistoryOlderThanRetention);
    }

    private void purgeTrends() {
        purgeOne("monitor_trends", metrics::purgeTrendsOlderThanRetention);
    }

    private void purgeOne(String table, java.util.function.Supplier<RetentionPurge.Result> purge) {
        try {
            logOutcome(table, purge.get());
        } catch (RuntimeException ex) {
            throw new IllegalStateException(table, ex);
        }
    }

    private void logOutcome(String table, RetentionPurge.Result result) {
        if (result == null || result.removed() <= 0) {
            return;
        }
        if (result.kind() == RetentionPurge.Kind.CHUNKS) {
            log.info("dropped {} {} chunks beyond retention", result.removed(), table);
        } else {
            log.info("purged {} {} rows beyond retention", result.removed(), table);
        }
    }
}
