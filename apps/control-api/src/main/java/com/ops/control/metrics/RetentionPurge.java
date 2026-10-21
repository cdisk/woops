package com.ops.control.metrics;

import org.springframework.dao.DataAccessException;
import org.springframework.jdbc.core.JdbcTemplate;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.function.Consumer;

/**
 * Timescale hypertables are purged with drop_chunks so compressed chunks are
 * removed whole. Plain PostgreSQL tables fall back to row deletes.
 */
final class RetentionPurge {
    enum Kind { CHUNKS, ROWS }

    record Result(Kind kind, int removed) {}

    private RetentionPurge() {}

    static Result decide(boolean hypertable, int chunkCount, int rowCount) {
        if (hypertable) {
            return new Result(Kind.CHUNKS, chunkCount);
        }
        return new Result(Kind.ROWS, rowCount);
    }

    static void runBoth(Runnable history, Runnable trends, Consumer<RuntimeException> onError) {
        try {
            history.run();
        } catch (RuntimeException ex) {
            onError.accept(ex);
        }
        try {
            trends.run();
        } catch (RuntimeException ex) {
            onError.accept(ex);
        }
    }

    static boolean isHypertable(JdbcTemplate jdbc, String table) {
        try {
            Boolean exists = jdbc.queryForObject(
                    """
                    select exists (
                      select 1 from timescaledb_information.hypertables
                      where hypertable_schema = current_schema()
                        and hypertable_name = ?
                    )
                    """,
                    Boolean.class,
                    table);
            return Boolean.TRUE.equals(exists);
        } catch (DataAccessException ex) {
            return false;
        }
    }

    /** @return dropped chunk count, or -1 when the table is not a hypertable */
    static int dropChunks(JdbcTemplate jdbc, String table, Instant olderThan) {
        if (!isHypertable(jdbc, table)) {
            return -1;
        }
        List<String> dropped = jdbc.query(
                dropChunksSQL(table),
                (rs, row) -> rs.getString(1),
                Timestamp.from(olderThan));
        return dropped.size();
    }

    static String dropChunksSQL(String table) {
        if (!"monitor_history".equals(table) && !"monitor_trends".equals(table)) {
            throw new IllegalArgumentException("unexpected metrics table " + table);
        }
        return "select drop_chunks('" + table + "', older_than => ?::timestamptz)::text";
    }
}
