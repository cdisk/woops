package com.ops.control.metrics;

import org.junit.jupiter.api.Test;

import java.time.Instant;

import static org.junit.jupiter.api.Assertions.*;

class AlertDurationLogicTest {

    @Test
    void zeroMinutesIsImmediate() {
        Instant now = Instant.parse("2026-10-13T02:00:00Z");
        assertTrue(AlertDurationLogic.heldLongEnough(null, now, 0));
        assertTrue(AlertDurationLogic.heldLongEnough(now, now, 0));
    }

    @Test
    void spikeUnderThreeMinutesDoesNotHold() {
        Instant since = Instant.parse("2026-10-13T02:00:00Z");
        Instant oneMin = since.plusSeconds(60);
        Instant twoMin = since.plusSeconds(120);
        assertFalse(AlertDurationLogic.heldLongEnough(since, oneMin, 3));
        assertFalse(AlertDurationLogic.heldLongEnough(since, twoMin, 3));
        assertFalse(AlertDurationLogic.heldLongEnough(null, oneMin, 3));
    }

    @Test
    void threeMinutesHolds() {
        Instant since = Instant.parse("2026-10-13T02:00:00Z");
        Instant at = since.plusSeconds(180);
        Instant after = since.plusSeconds(181);
        assertTrue(AlertDurationLogic.heldLongEnough(since, at, 3));
        assertTrue(AlertDurationLogic.heldLongEnough(since, after, 3));
    }

    @Test
    void recoverSameAsFireTiming() {
        Instant since = Instant.parse("2026-10-13T02:00:00Z");
        assertFalse(AlertDurationLogic.heldLongEnough(since, since.plusSeconds(179), 3));
        assertTrue(AlertDurationLogic.heldLongEnough(since, since.plusSeconds(180), 3));
    }

    @Test
    void offlineAndSpecialReasonsCloseImmediately() {
        assertTrue(AlertDurationLogic.isImmediateCloseReason(
                AlertIssueLogic.KIND_OFFLINE, AlertIncidentEntity.REASON_NORMAL));
        assertTrue(AlertDurationLogic.isImmediateCloseReason(
                AlertIssueLogic.KIND_METRIC, AlertIncidentEntity.REASON_INSTANCE_GONE));
        assertTrue(AlertDurationLogic.isImmediateCloseReason(
                AlertIssueLogic.KIND_METRIC, AlertIncidentEntity.REASON_ASSET_OFFLINE));
        assertTrue(AlertDurationLogic.isImmediateCloseReason(
                AlertIssueLogic.KIND_METRIC, AlertIncidentEntity.REASON_RULE_CHANGED));
        assertFalse(AlertDurationLogic.isImmediateCloseReason(
                AlertIssueLogic.KIND_METRIC, AlertIncidentEntity.REASON_NORMAL));
    }

    @Test
    void clampMinutes() {
        assertEquals(3, AlertDurationLogic.clampMinutes(null));
        assertEquals(0, AlertDurationLogic.clampMinutes(0));
        assertEquals(5, AlertDurationLogic.clampMinutes(5));
        assertEquals(0, AlertDurationLogic.clampMinutes(-1));
    }
}
