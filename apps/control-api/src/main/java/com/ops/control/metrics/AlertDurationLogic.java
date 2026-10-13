package com.ops.control.metrics;

import java.time.Instant;

/**
 * Duration helpers for metric alert fire/recover delays.
 * Offline alerts bypass this; only metric rules use minutes.
 */
public final class AlertDurationLogic {
    public static final int DEFAULT_MINUTES = 3;

    private AlertDurationLogic() {}

    public static int clampMinutes(Integer minutes) {
        if (minutes == null) {
            return DEFAULT_MINUTES;
        }
        return Math.max(0, minutes);
    }

    /** True when minutes==0 or {@code since} has been held for at least {@code minutes}. */
    public static boolean heldLongEnough(Instant since, Instant now, int minutes) {
        if (minutes <= 0) {
            return true;
        }
        if (since == null || now == null) {
            return false;
        }
        return !now.isBefore(since.plusSeconds(minutes * 60L));
    }

    public static boolean isImmediateCloseReason(String kind, String reason) {
        if (AlertIssueLogic.KIND_OFFLINE.equals(kind)) {
            return true;
        }
        return AlertIncidentEntity.REASON_INSTANCE_GONE.equals(reason)
                || AlertIncidentEntity.REASON_ASSET_OFFLINE.equals(reason)
                || AlertIncidentEntity.REASON_RULE_CHANGED.equals(reason);
    }
}
