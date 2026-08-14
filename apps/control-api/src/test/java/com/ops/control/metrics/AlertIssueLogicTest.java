package com.ops.control.metrics;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AlertIssueLogicTest {

    private static final AlertIssueLogic.Rule CPU = new AlertIssueLogic.Rule(
            "CPU high", "cpu.usage_percent", "", "gt", 90);
    private static final AlertIssueLogic.Rule MEM = new AlertIssueLogic.Rule(
            "Mem high", "mem.used_percent", "", "gt", 90);

    @Test
    void offlineIsAbnormalUnlessIgnored() {
        List<AlertIssueLogic.Issue> cpuHit = AlertIssueLogic.metricHits(
                List.of(new AlertIssueLogic.Point("cpu.usage_percent", "", 99)),
                List.of(CPU));
        List<AlertIssueLogic.Issue> visible = AlertIssueLogic.visibleIssues(false, cpuHit, Set.of());
        assertEquals(1, visible.size());
        assertEquals(AlertIssueLogic.ITEM_HOST_ONLINE, visible.get(0).itemId());
        assertEquals(AlertIssueLogic.KIND_OFFLINE, visible.get(0).kind());

        assertTrue(AlertIssueLogic.visibleIssues(false, cpuHit, Set.of(AlertIssueLogic.ITEM_HOST_ONLINE)).isEmpty());
    }

    @Test
    void ignoreIsPerItem() {
        List<AlertIssueLogic.Issue> hits = AlertIssueLogic.metricHits(
                List.of(
                        new AlertIssueLogic.Point("cpu.usage_percent", "", 95),
                        new AlertIssueLogic.Point("mem.used_percent", "", 96)
                ),
                List.of(CPU, MEM));
        List<AlertIssueLogic.Issue> visible = AlertIssueLogic.visibleIssues(
                true, hits, Set.of("cpu.usage_percent"));
        assertEquals(1, visible.size());
        assertEquals("mem.used_percent", visible.get(0).itemId());
    }

    @Test
    void onlineWithoutHitsIsNotAbnormal() {
        assertTrue(AlertIssueLogic.visibleIssues(true, List.of(), Set.of()).isEmpty());
    }
}
