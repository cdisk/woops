package com.ops.control.metrics;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * Dashboard abnormality: metric-rule hits plus offline.
 * Ignore is per asset + monitor itemId (offline uses {@link #ITEM_HOST_ONLINE}).
 */
final class AlertIssueLogic {
    static final String ITEM_HOST_ONLINE = "host.online";
    static final String KIND_OFFLINE = "offline";
    static final String KIND_METRIC = "metric";

    record Point(String itemId, String instance, double value) {}

    record Rule(String name, String itemId, String instance, String op, double threshold) {}

    record Issue(String itemId, String kind, String label, String instance) {}

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
                hits.add(new Issue(p.itemId(), KIND_METRIC, label + "=" + formatNum(p.value()), inst));
            }
        }
        return hits;
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
            return List.of(new Issue(ITEM_HOST_ONLINE, KIND_OFFLINE, "", ""));
        }
        if (metricHits == null || metricHits.isEmpty()) {
            return List.of();
        }
        return metricHits.stream().filter(h -> !ignored.contains(h.itemId())).toList();
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
}
