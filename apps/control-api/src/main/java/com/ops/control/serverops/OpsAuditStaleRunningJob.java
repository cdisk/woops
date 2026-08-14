package com.ops.control.serverops;

import com.ops.control.common.OpsProperties;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

/**
 * Safety net for operations left {@code RUNNING} when Gateway cannot write END
 * (SIGKILL / crash). Graceful SIGTERM is handled in Gateway {@code CloseAudit};
 * Gateway process start also calls {@code /interrupt-running} for immediate cleanup.
 */
@Component
public class OpsAuditStaleRunningJob {
    private static final Logger log = LoggerFactory.getLogger(OpsAuditStaleRunningJob.class);

    private final ServerOperationAuditService operations;
    private final OpsProperties props;

    public OpsAuditStaleRunningJob(ServerOperationAuditService operations, OpsProperties props) {
        this.operations = operations;
        this.props = props;
    }

    @EventListener(ApplicationReadyEvent.class)
    public void onReady() {
        sweep();
    }

    @Scheduled(fixedDelayString = "900000", initialDelay = 60_000)
    public void sweep() {
        int hours = props.auditStaleRunningHours();
        if (hours <= 0) {
            return;
        }
        int n = operations.interruptStaleRunning(hours, "stale_running");
        if (n > 0) {
            log.info("ops-audit stale RUNNING → INTERRUPTED: {} (older than {}h)", n, hours);
        }
    }
}
