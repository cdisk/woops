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
 * Idempotent Timescale bootstrap after Hibernate creates plain monitor tables.
 * Packaged in the control-api image; runs on every start and no-ops when already applied.
 * <ul>
 *   <li>{@code CREATE EXTENSION timescaledb}</li>
 *   <li>{@code create_hypertable} for {@code monitor_history} / {@code monitor_trends}</li>
 *   <li>compression settings + {@code add_compression_policy}</li>
 * </ul>
 * Heavy {@code migrate_data} (rare on fresh installs) runs in a background thread
 * so the API can accept traffic.
 */
@Component
@Order(55)
public class MetricsTimescaleBootstrap implements ApplicationRunner {
    private static final Logger log = LoggerFactory.getLogger(MetricsTimescaleBootstrap.class);
    private static final long ADVISORY_LOCK_KEY = 2609041731L;

    private final DataSource dataSource;

    public MetricsTimescaleBootstrap(DataSource dataSource) {
        this.dataSource = dataSource;
    }

    @Override
    public void run(ApplicationArguments args) {
        Thread t = new Thread(this::ensure, "monitor-timescale-bootstrap");
        t.setDaemon(true);
        t.start();
    }

    private void ensure() {
        try (Connection conn = dataSource.getConnection()) {
            conn.setAutoCommit(true);
            if (!tryLock(conn)) {
                log.info("Timescale bootstrap already running; skipping");
                return;
            }
            try {
                ensureExtension(conn);
                if (!isTimescaleAvailable(conn)) {
                    log.warn("timescaledb extension unavailable; monitor tables stay as plain Postgres");
                    return;
                }
                ensureHypertable(
                        conn,
                        "monitor_history",
                        "time",
                        "INTERVAL '1 day'",
                        "asset_id,item_id,instance",
                        "time DESC",
                        "INTERVAL '1 day'");
                ensureHypertable(
                        conn,
                        "monitor_trends",
                        "hour",
                        "INTERVAL '30 days'",
                        "asset_id,item_id,instance",
                        "hour DESC",
                        "INTERVAL '7 days'");
            } finally {
                unlock(conn);
            }
        } catch (Exception e) {
            log.warn("Timescale bootstrap failed (API still runs on plain tables): {}", e.toString());
        }
    }

    private static void ensureExtension(Connection conn) throws Exception {
        try (Statement st = conn.createStatement()) {
            st.execute("CREATE EXTENSION IF NOT EXISTS timescaledb");
        }
        log.info("timescaledb extension is available");
    }

    private static boolean isTimescaleAvailable(Connection conn) throws Exception {
        try (PreparedStatement ps = conn.prepareStatement(
                "SELECT 1 FROM pg_extension WHERE extname = 'timescaledb'")) {
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next();
            }
        }
    }

    private static void ensureHypertable(
            Connection conn,
            String table,
            String timeColumn,
            String chunkInterval,
            String compressSegmentBy,
            String compressOrderBy,
            String compressAfter) throws Exception {
        if (!tableExists(conn, table)) {
            log.info("Table {} not present yet; skip Timescale setup", table);
            return;
        }
        if (!isHypertable(conn, table)) {
            log.info("Converting {} to hypertable (may take a while if the table already has data)", table);
            try (Statement st = conn.createStatement()) {
                st.execute(
                        "SELECT create_hypertable('"
                                + table
                                + "', '"
                                + timeColumn
                                + "', if_not_exists => TRUE, migrate_data => TRUE, chunk_time_interval => "
                                + chunkInterval
                                + ")");
            }
            log.info("{} is a hypertable", table);
        }
        try (Statement st = conn.createStatement()) {
            st.execute(
                    "ALTER TABLE "
                            + table
                            + " SET ("
                            + "timescaledb.compress, "
                            + "timescaledb.compress_segmentby = '"
                            + compressSegmentBy
                            + "', "
                            + "timescaledb.compress_orderby = '"
                            + compressOrderBy
                            + "')");
            st.execute(
                    "SELECT add_compression_policy('"
                            + table
                            + "', "
                            + compressAfter
                            + ", if_not_exists => TRUE)");
        }
        log.info("{} compression policy ensured (after {})", table, compressAfter);
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

    private static boolean isHypertable(Connection conn, String table) throws Exception {
        try (PreparedStatement ps = conn.prepareStatement(
                """
                SELECT 1 FROM timescaledb_information.hypertables
                WHERE hypertable_schema = current_schema() AND hypertable_name = ?
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
            log.warn("Failed to release Timescale bootstrap advisory lock: {}", e.toString());
        }
    }
}
