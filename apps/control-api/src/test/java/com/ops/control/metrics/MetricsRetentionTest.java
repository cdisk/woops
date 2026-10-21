package com.ops.control.metrics;

import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class MetricsRetentionTest {
    @Test
    void hypertableUsesChunksAndPlainTableUsesRows() {
        RetentionPurge.Result chunks = RetentionPurge.decide(true, 3, 100);
        assertEquals(RetentionPurge.Kind.CHUNKS, chunks.kind());
        assertEquals(3, chunks.removed());

        RetentionPurge.Result rows = RetentionPurge.decide(false, 0, 100);
        assertEquals(RetentionPurge.Kind.ROWS, rows.kind());
        assertEquals(100, rows.removed());
    }

    @Test
    void dropChunksSqlIsLimitedToMetricsTables() {
        assertEquals(
                "select drop_chunks('monitor_history', older_than => ?::timestamptz)::text",
                RetentionPurge.dropChunksSQL("monitor_history"));
        assertEquals(
                "select drop_chunks('monitor_trends', older_than => ?::timestamptz)::text",
                RetentionPurge.dropChunksSQL("monitor_trends"));
        assertThrows(IllegalArgumentException.class, () -> RetentionPurge.dropChunksSQL("other"));
    }

    @Test
    void oneTableFailureDoesNotStopTheOther() {
        AtomicInteger ran = new AtomicInteger();
        AtomicInteger errors = new AtomicInteger();
        RetentionPurge.runBoth(
                () -> {
                    throw new IllegalStateException("history");
                },
                ran::incrementAndGet,
                ex -> errors.incrementAndGet());
        assertEquals(1, ran.get());
        assertEquals(1, errors.get());
    }
}
