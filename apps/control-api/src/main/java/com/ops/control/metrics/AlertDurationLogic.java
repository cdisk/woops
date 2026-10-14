package com.ops.control.metrics;

import java.time.Instant;
import java.util.UUID;

/**
 * Duration helpers for alert fire/recover delays (metrics rules + offline/online).
 */
public final class AlertDurationLogic {
    public static final int DEFAULT_MINUTES = 3;

    /** Default hold before firing {@code host.online} / closing after return. */
    public static final int OFFLINE_FIRE_MINUTES = 1;
    public static final int OFFLINE_RECOVER_MINUTES = 1;

    /**
     * Synthetic rule id for {@code alert_pending} rows of offline issues
     * (incidents still store {@code rule_id=null}).
     */
    public static final UUID OFFLINE_PENDING_RULE_ID =
            UUID.fromString("00000000-0000-4000-8000-0000000000ff");

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

    /** Forced closes that skip recover delay (not NORMAL recover of offline/metrics). */
    public static boolean isImmediateCloseReason(String kind, String reason) {
        return AlertIncidentEntity.REASON_INSTANCE_GONE.equals(reason)
                || AlertIncidentEntity.REASON_ASSET_OFFLINE.equals(reason)
                || AlertIncidentEntity.REASON_RULE_CHANGED.equals(reason);
    }

    public static boolean isOfflinePendingRule(UUID ruleId) {
        return OFFLINE_PENDING_RULE_ID.equals(ruleId);
    }

    /** Map key aligned with {@link AlertIssueLogic#issueKey} for offline (null rule). */
    public static String pendingMapKey(String itemId, String instance, UUID ruleId) {
        if (isOfflinePendingRule(ruleId) || AlertIssueLogic.ITEM_HOST_ONLINE.equals(itemId)) {
            return AlertIssueLogic.issueKey(itemId, instance, null);
        }
        return AlertIssueLogic.issueKey(itemId, instance, ruleId);
    }

    public static UUID pendingRuleId(AlertIssueLogic.Issue issue) {
        if (issue == null) {
            return null;
        }
        if (issue.ruleId() != null) {
            return issue.ruleId();
        }
        if (AlertIssueLogic.KIND_OFFLINE.equals(issue.kind())
                || AlertIssueLogic.ITEM_HOST_ONLINE.equals(issue.itemId())) {
            return OFFLINE_PENDING_RULE_ID;
        }
        return null;
    }
}
