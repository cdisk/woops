package com.ops.control.metrics;

import org.junit.jupiter.api.Test;

import java.time.Instant;

import static org.junit.jupiter.api.Assertions.assertEquals;

class MetricsServiceTest {
    private static final Instant FROM = Instant.parse("2026-01-01T00:00:00Z");

    @Test
    void keepsMinuteGrainThroughThreeDays() {
        assertEquals("minute", MetricsService.effectiveGrain(
                "minute", FROM, FROM.plusSeconds(3 * 24 * 60 * 60L)));
    }

    @Test
    void promotesMinuteGrainBeyondThreeDays() {
        assertEquals("day", MetricsService.effectiveGrain(
                "minute", FROM, FROM.plusSeconds(3 * 24 * 60 * 60L + 1)));
    }

    @Test
    void promotesAnyGrainBeyondOneHundredEightyDays() {
        assertEquals("month", MetricsService.effectiveGrain(
                "day", FROM, FROM.plusSeconds(181 * 24 * 60 * 60L)));
    }

    @Test
    void preservesExplicitCoarserGrain() {
        assertEquals("day", MetricsService.effectiveGrain(
                "day", FROM, FROM.plusSeconds(24 * 60 * 60L)));
        assertEquals("month", MetricsService.effectiveGrain(
                "month", FROM, FROM.plusSeconds(24 * 60 * 60L)));
    }
}
