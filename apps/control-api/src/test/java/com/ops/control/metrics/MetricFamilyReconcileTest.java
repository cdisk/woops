package com.ops.control.metrics;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class MetricFamilyReconcileTest {

    @Test
    void withoutCompleteFamiliesDoesNotDelete() {
        MonitorLatestEntity old = latest("disk.used_percent", "/old");
        Set<String> gone = MetricFamilyReconcile.goneKeys(
                List.of(new MetricsService.PointIn("disk.used_percent", "/", 10)),
                List.of(old),
                List.of());
        assertTrue(gone.isEmpty());
    }

    @Test
    void completeDiskRemovesMissingMount() {
        MonitorLatestEntity goneRow = latest("disk.used_percent", "/old");
        MonitorLatestEntity keepCpu = latest("cpu.usage_percent", "");
        Set<String> gone = MetricFamilyReconcile.goneKeys(
                List.of(new MetricsService.PointIn("disk.used_percent", "/", 10)),
                List.of(goneRow, keepCpu),
                List.of("disk"));
        assertEquals(Set.of(MetricFamilyReconcile.key("disk.used_percent", "/old")), gone);
    }

    @Test
    void incompleteFamilyDoesNotRemovePeers() {
        MonitorLatestEntity oldNic = latest("net.rx_bytes", "eth0");
        Set<String> gone = MetricFamilyReconcile.goneKeys(
                List.of(new MetricsService.PointIn("disk.used_percent", "/", 10)),
                List.of(oldNic),
                List.of("disk"));
        assertTrue(gone.isEmpty());
    }

    private static MonitorLatestEntity latest(String itemId, String instance) {
        MonitorLatestEntity e = new MonitorLatestEntity();
        e.setAssetId(UUID.randomUUID());
        e.setItemId(itemId);
        e.setInstance(instance);
        e.setValue(1);
        e.setTime(java.time.Instant.now());
        return e;
    }
}
