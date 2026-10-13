package com.ops.control.metrics;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.UUID;

/**
 * Dashboard abnormality: metric-rule hits plus offline.
 * Ignore is per asset + monitor itemId (offline uses {@link #ITEM_HOST_ONLINE}).
 */
public final class AlertIssueLogic {
    static final String ITEM_HOST_ONLINE = "host.online";
    static final String KIND_OFFLINE = "offline";
    static final String KIND_METRIC = "metric";

    public record Point(String itemId, String instance, double value) {}

    public record Rule(
            UUID id,
            String name,
            String itemId,
            String instance,
            String op,
            double threshold,
            int fireMinutes,
            int recoverMinutes) {
        public Rule(UUID id, String name, String itemId, String instance, String op, double threshold) {
            this(id, name, itemId, instance, op, threshold, 0, 0);
        }
    }

    public record Issue(
            String itemId,
            String kind,
            String label,
            String instance,
            double value,
            UUID ruleId,
            String ruleName,
            String op,
            Double threshold) {}

    private AlertIssueLogic() {}

    static List<Issue> metricHits(List<Point> points, List<Rule> rules) {
        List<Issue> hits = new ArrayList<>();
        if (rules == null || points == null) {
            return hits;
        }
        for (Rule rule : rules) {
            for (Point p : points) {
                if (rule.itemId() == null || !rule.itemId().equals(p.itemId())) {
                    continue;
                }
                String inst = p.instance() == null ? "" : p.instance();
                String ruleInst = rule.instance() == null ? "" : rule.instance();
                if (!ruleInst.isEmpty() && !ruleInst.equals(inst)) {
                    continue;
                }
                if (!matches(rule.op(), p.value(), rule.threshold())) {
                    continue;
                }
                String label = rule.name() == null ? p.itemId() : rule.name();
                if (!inst.isEmpty()) {
                    label = label + " (" + inst + ")";
                }
                hits.add(new Issue(
                        p.itemId(),
                        KIND_METRIC,
                        label + "=" + formatNum(p.value()),
                        inst,
                        p.value(),
                        rule.id(),
                        rule.name() == null ? "" : rule.name(),
                        rule.op() == null ? "gt" : rule.op(),
                        rule.threshold()));
            }
        }
        return hits;
    }

    /** Objective issues: ignore is not applied. Offline assets only expose host.online. */
    static List<Issue> actualIssues(boolean online, List<Issue> metricHits) {
        return visibleIssues(online, metricHits, Set.of());
    }

    /**
     * Offline assets only surface the online-status issue (stale metric alerts stay hidden).
     * Ignored itemIds are dropped.
     */
    static List<Issue> visibleIssues(boolean online, List<Issue> metricHits, Set<String> ignoredItemIds) {
        Set<String> ignored = ignoredItemIds == null ? Set.of() : ignoredItemIds;
        if (!online) {
            if (ignored.contains(ITEM_HOST_ONLINE)) {
                return List.of();
            }
            return List.of(offlineIssue());
        }
        if (metricHits == null || metricHits.isEmpty()) {
            return List.of();
        }
        return metricHits.stream().filter(h -> !ignored.contains(h.itemId())).toList();
    }

    static Issue offlineIssue() {
        return new Issue(ITEM_HOST_ONLINE, KIND_OFFLINE, "", "", 0, null, "", "", null);
    }

    static boolean matches(String op, double value, double threshold) {
        return switch (op == null ? "gt" : op) {
            case "gte" -> value >= threshold;
            case "lt" -> value < threshold;
            case "lte" -> value <= threshold;
            default -> value > threshold;
        };
    }

    static String formatNum(double v) {
        if (Math.abs(v - Math.rint(v)) < 1e-9) {
            return String.valueOf((long) Math.rint(v));
        }
        return String.format(Locale.ROOT, "%.2f", v);
    }

    static String kindOf(String itemId) {
        return ITEM_HOST_ONLINE.equals(itemId) ? KIND_OFFLINE : KIND_METRIC;
    }

    static String issueKey(Issue issue) {
        return issueKey(issue.itemId(), issue.instance(), issue.ruleId());
    }

    static String issueKey(String itemId, String instance, UUID ruleId) {
        String inst = instance == null ? "" : instance;
        String rule = ruleId == null ? "-" : ruleId.toString();
        return itemId + "\0" + inst + "\0" + rule;
    }
}
