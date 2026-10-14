package com.ops.control.metrics;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.util.UUID;

/**
 * Offline assets do not ingest metrics, so FIRE/RECOVER pending for {@code host.online}
 * would never mature without a periodic recompute sweep.
 */
@Component
public class AlertPendingSweepJob {
    private static final Logger log = LoggerFactory.getLogger(AlertPendingSweepJob.class);

    private final AlertPendingRepository pendings;
    private final AlertEvaluationService alertEval;

    public AlertPendingSweepJob(AlertPendingRepository pendings, AlertEvaluationService alertEval) {
        this.pendings = pendings;
        this.alertEval = alertEval;
    }

    @Scheduled(fixedDelayString = "15000", initialDelay = 20_000)
    public void sweep() {
        try {
            for (UUID assetId : pendings.findDistinctAssetIds()) {
                if (assetId != null) {
                    alertEval.recompute(assetId);
                }
            }
        } catch (Exception e) {
            log.warn("alert pending sweep failed: {}", e.toString());
        }
    }
}
