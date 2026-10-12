package com.ops.control.metrics;

import java.util.*;

/** Reconciles monitor_latest rows for complete metric families (disk/diskio/net). */
final class MetricFamilyReconcile {
    static final Set<String> KNOWN = Set.of("disk", "diskio", "net");

    private MetricFamilyReconcile() {}

    static String familyOf(String itemId) {
        if (itemId == null || itemId.isBlank()) {
            return null;
        }
        int dot = itemId.indexOf('.');
        if (dot <= 0) {
            return null;
        }
        String prefix = itemId.substring(0, dot);
        return KNOWN.contains(prefix) ? prefix : null;
    }

    /**
     * @return set of {@code itemId + "\\0" + instance} keys that should be deleted from latest
     */
    static Set<String> goneKeys(
            Collection<MetricsService.PointIn> points,
            Collection<MonitorLatestEntity> existing,
            Collection<String> completeFamilies) {
        Set<String> families = new HashSet<>();
        if (completeFamilies != null) {
            for (String f : completeFamilies) {
                if (f != null && KNOWN.contains(f.trim().toLowerCase(Locale.ROOT))) {
                    families.add(f.trim().toLowerCase(Locale.ROOT));
                }
            }
        }
        if (families.isEmpty()) {
            return Set.of();
        }
        Set<String> present = new HashSet<>();
        if (points != null) {
            for (MetricsService.PointIn p : points) {
                if (p == null || p.itemId() == null) continue;
                String fam = familyOf(p.itemId());
                if (fam == null || !families.contains(fam)) continue;
                String inst = p.instance() == null ? "" : p.instance();
                present.add(key(p.itemId(), inst));
            }
        }
        Set<String> gone = new HashSet<>();
        if (existing != null) {
            for (MonitorLatestEntity row : existing) {
                String fam = familyOf(row.getItemId());
                if (fam == null || !families.contains(fam)) continue;
                String k = key(row.getItemId(), row.getInstance());
                if (!present.contains(k)) {
                    gone.add(k);
                }
            }
        }
        return gone;
    }

    static String key(String itemId, String instance) {
        return itemId + "\0" + (instance == null ? "" : instance);
    }
}
