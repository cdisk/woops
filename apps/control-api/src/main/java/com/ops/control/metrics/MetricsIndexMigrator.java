package com.ops.control.metrics;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;

/**
 * Builds the time-range-first series index in the background so startup and the
 * Hikari pool stay responsive while PostgreSQL creates the index concurrently.
 */
@Component
@Order(60)
public class MetricsIndexMigrator implements ApplicationRunner {
    private static final Logger log = LoggerFactory.getLogger(MetricsIndexMigrator.class);
    private static final String INDEX_NAME = "idx_monitor_history_series";
    private static final String TABLE_NAME = "monitor_history";
    private static final long ADVISORY_LOCK_KEY = 2609041730L;

    private final DataSource dataSource;

    public MetricsIndexMigrator(DataSource dataSource) {
        this.dataSource = dataSource;
    }

    @Override
    public void run(ApplicationArguments args) {
        Thread t = new Thread(this::ensureIndex, "monitor-index-migrator");
        t.setDaemon(true);
        t.start();
    }

    private void ensureIndex() {
        try (Connection conn = dataSource.getConnection()) {
            conn.setAutoCommit(true);
            if (!tableExists(conn, TABLE_NAME)) {
                log.info("Table {} not present yet; skip series index migration", TABLE_NAME);
                return;
            }
            if (!tryLock(conn)) {
                log.info("Monitor series index migration already running; skipping");
                return;
            }
            try {
                Boolean valid = indexValid(conn);
                if (Boolean.TRUE.equals(valid)) {
                    return;
                }
                if (Boolean.FALSE.equals(valid)) {
                    // Concurrent create left an invalid index; drop and rebuild.
                    try (Statement st = conn.createStatement()) {
                        st.execute("DROP INDEX CONCURRENTLY IF EXISTS " + INDEX_NAME);
                    }
                }
                log.info("Building monitor series index concurrently in background");
                try (Statement st = conn.createStatement()) {
                    st.execute(
                            "CREATE INDEX CONCURRENTLY IF NOT EXISTS " + INDEX_NAME
                                    + " ON " + TABLE_NAME + " (asset_id, item_id, time, instance)");
                }
                log.info("Monitor series index is ready");
            } finally {
                unlock(conn);
            }
        } catch (Exception e) {
            log.warn("Monitor series index migration failed; queries may be slower until fixed: {}", e.toString());
        }
    }

    private static boolean tableExists(Connection conn, String table) throws Exception {
        try (PreparedStatement ps = conn.prepareStatement(
                """
                SELECT 1 FROM information_schema.tables
                WHERE table_schema = current_schema() AND table_name = ?
                """)) {
            ps.setString(1, table);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next();
            }
        }
    }

    private static boolean tryLock(Connection conn) throws Exception {
        try (PreparedStatement ps = conn.prepareStatement("SELECT pg_try_advisory_lock(?)")) {
            ps.setLong(1, ADVISORY_LOCK_KEY);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() && rs.getBoolean(1);
            }
        }
    }

    private static void unlock(Connection conn) {
        try (PreparedStatement ps = conn.prepareStatement("SELECT pg_advisory_unlock(?)")) {
            ps.setLong(1, ADVISORY_LOCK_KEY);
            ps.executeQuery().close();
        } catch (Exception e) {
            log.warn("Failed to release monitor index advisory lock: {}", e.toString());
        }
    }

    private static Boolean indexValid(Connection conn) throws Exception {
        try (PreparedStatement ps = conn.prepareStatement(
                """
                SELECT i.indisvalid
                FROM pg_index i
                JOIN pg_class c ON c.oid = i.indexrelid
                JOIN pg_namespace n ON n.oid = c.relnamespace
                WHERE n.nspname = current_schema() AND c.relname = ?
                """)) {
            ps.setString(1, INDEX_NAME);
            try (ResultSet rs = ps.executeQuery()) {
                if (!rs.next()) {
                    return null;
                }
                return rs.getBoolean(1);
            }
        }
    }
}
