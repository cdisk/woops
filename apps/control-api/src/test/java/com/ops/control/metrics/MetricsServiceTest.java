package com.ops.control.metrics;

import org.junit.jupiter.api.Test;

import java.time.Instant;

import static org.junit.jupiter.api.Assertions.assertEquals;

class MetricsServiceTest {
    private static final Instant FROM = Instant.parse("2026-01-01T00:00:00Z");

    @Test
    void honorsRequestedGrainsRegardlessOfSpan() {
        Instant yearLater = FROM.plusSeconds(365 * 24 * 60 * 60L);
        assertEquals("minute", MetricsService.effectiveGrain("minute", FROM, yearLater, false));
        assertEquals("hour", MetricsService.effectiveGrain("hour", FROM, yearLater, false));
        assertEquals("day", MetricsService.effectiveGrain("day", FROM, yearLater, false));
        assertEquals("month", MetricsService.effectiveGrain("month", FROM, yearLater, false));
    }

    @Test
    void trendsRewriteMinuteToHourOnly() {
        Instant yearLater = FROM.plusSeconds(365 * 24 * 60 * 60L);
        assertEquals("hour", MetricsService.effectiveGrain("minute", FROM, yearLater, true));
        assertEquals("hour", MetricsService.effectiveGrain("hour", FROM, yearLater, true));
        assertEquals("day", MetricsService.effectiveGrain("day", FROM, yearLater, true));
        assertEquals("month", MetricsService.effectiveGrain("month", FROM, yearLater, true));
    }

    @Test
    void usesHistoryWhenRangeFullyInsideRetentionWindow() {
        Instant now = Instant.parse("2026-09-10T02:00:00Z");
        Instant from = now.minusSeconds(2 * 24 * 60 * 60L);
        assertEquals(false, MetricsService.useTrends(from, now, now, 7));
    }

    @Test
    void usesTrendsWhenFromIsOlderThanRetentionEvenIfSpanIsShort() {
        Instant now = Instant.parse("2026-09-10T02:00:00Z");
        Instant from = Instant.parse("2026-08-07T12:49:12.584Z");
        Instant to = Instant.parse("2026-08-14T07:07:20.344Z");
        assertEquals(true, MetricsService.useTrends(from, to, now, 7));
    }

    @Test
    void usesTrendsWhenSpanExceedsRetention() {
        Instant now = Instant.parse("2026-09-10T02:00:00Z");
        Instant from = now.minusSeconds(10 * 24 * 60 * 60L);
        assertEquals(true, MetricsService.useTrends(from, now, now, 7));
    }
}
