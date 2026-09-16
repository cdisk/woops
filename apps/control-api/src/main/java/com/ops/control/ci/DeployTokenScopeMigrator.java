package com.ops.control.ci;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.annotation.Order;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

/**
 * One-shot: split legacy scopes into upload/download/exec/forward/reverse.
 * <ul>
 *   <li>If {@code allow_portmap} still exists: download←upload, forward/reverse←portmap, then drop portmap.</li>
 *   <li>Else: download←upload (old upload scope bundled download).</li>
 * </ul>
 */
@Component
@Order(50)
public class DeployTokenScopeMigrator implements ApplicationRunner {
    private static final Logger log = LoggerFactory.getLogger(DeployTokenScopeMigrator.class);
    private static final String PATCH_ID = "deploy_token_scopes_v2";

    private final JdbcTemplate jdbc;

    public DeployTokenScopeMigrator(JdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    @Override
    public void run(ApplicationArguments args) {
        jdbc.execute(
                """
                CREATE TABLE IF NOT EXISTS ops_schema_patches (
                  id varchar(64) PRIMARY KEY,
                  applied_at timestamptz NOT NULL
                )
                """);
        migrateScopes();
        purgeSoftRevoked();
    }

    private void migrateScopes() {
        Integer applied = jdbc.queryForObject(
                "SELECT COUNT(*) FROM ops_schema_patches WHERE id = ?",
                Integer.class,
                PATCH_ID);
        if (applied != null && applied > 0) {
            return;
        }

        Boolean hasPortmap = jdbc.query(
                """
                SELECT EXISTS (
                  SELECT 1 FROM information_schema.columns
                  WHERE table_schema = current_schema()
                    AND table_name = 'deploy_tokens'
                    AND column_name = 'allow_portmap'
                )
                """,
                rs -> rs.next() && rs.getBoolean(1));

        int n;
        if (Boolean.TRUE.equals(hasPortmap)) {
            n = jdbc.update(
                    """
                    UPDATE deploy_tokens SET
                      allow_download = allow_upload,
                      allow_forward = COALESCE(allow_portmap, false),
                      allow_reverse = COALESCE(allow_portmap, false)
                    """);
            jdbc.execute("ALTER TABLE deploy_tokens DROP COLUMN IF EXISTS allow_portmap");
            log.info("Migrated deploy_tokens from allow_portmap ({} row(s))", n);
        } else {
            n = jdbc.update("UPDATE deploy_tokens SET allow_download = allow_upload");
            log.info("Migrated deploy_tokens download←upload ({} row(s); no allow_portmap column)", n);
        }

        jdbc.update(
                "INSERT INTO ops_schema_patches(id, applied_at) VALUES (?, NOW())",
                PATCH_ID);
    }

    /** One-shot: hard-delete rows previously soft-revoked via revoked_at. */
    private void purgeSoftRevoked() {
        final String purgePatch = "deploy_token_purge_revoked_v1";
        Integer applied = jdbc.queryForObject(
                "SELECT COUNT(*) FROM ops_schema_patches WHERE id = ?",
                Integer.class,
                purgePatch);
        if (applied != null && applied > 0) {
            return;
        }
        Boolean hasRevokedAt = jdbc.query(
                """
                SELECT EXISTS (
                  SELECT 1 FROM information_schema.columns
                  WHERE table_schema = current_schema()
                    AND table_name = 'deploy_tokens'
                    AND column_name = 'revoked_at'
                )
                """,
                rs -> rs.next() && rs.getBoolean(1));
        int n = 0;
        if (Boolean.TRUE.equals(hasRevokedAt)) {
            n = jdbc.update("DELETE FROM deploy_tokens WHERE revoked_at IS NOT NULL");
            if (n > 0) {
                log.info("Purged {} soft-revoked deploy_tokens", n);
            }
        }
        jdbc.update(
                "INSERT INTO ops_schema_patches(id, applied_at) VALUES (?, NOW())",
                purgePatch);
    }
}
