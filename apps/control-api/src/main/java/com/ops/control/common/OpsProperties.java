package com.ops.control.common;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * URL naming:
 * <ul>
 *   <li>{@code *Public*} — browser / Agent / opsctl facing (https / wss)</li>
 *   <li>{@code *Internal*} — service-to-service on the private network (http)</li>
 * </ul>
 */
@ConfigurationProperties(prefix = "ops")
public record OpsProperties(
        /** Public control-api base (GitLab callback default, PublicUrlResolver apiBase). Env: OPS_CONTROL_PUBLIC_HTTP */
        String controlPublicHttp,
        /** Public Gateway HTTPS base (install links, OPSCTL_CONFIG.server). Env: OPS_GATEWAY_PUBLIC_HTTP */
        String gatewayPublicHttp,
        /** Public Gateway WSS base (browserWs tickets). Env: OPS_GATEWAY_PUBLIC_WS */
        String gatewayPublicWs,
        /** Internal Gateway HTTP for GatewayClient (portmap). Env: OPS_GATEWAY_INTERNAL_HTTP */
        String gatewayInternalHttp,
        /** Public console base (OAuth redirect). Env: OPS_CONSOLE_PUBLIC_HTTP */
        String consolePublicHttp,
        String jwtSecret,
        String ticketSecret,
        String bootstrapAdminUsername,
        String bootstrapAdminPassword,
        int installCodeTtlMinutes,
        /** Optional Gateway cert SPKI SHA-256 (hex). Env: OPS_GATEWAY_TLS_SPKI_SHA256 */
        String gatewayTlsSpkiSha256,
        /** Spool directory where Gateway drops runtime activity JSONL. Env: OPS_AUDIT_DIR */
        String auditDir,
        /** Spool scan interval in seconds. Env: OPS_AUDIT_SCAN_SECONDS */
        int auditScanSeconds,
        /** Retention for server_operation_records, in days. Env: OPS_AUDIT_RETENTION_DAYS */
        int auditRetentionDays,
        /**
         * Mark RUNNING operations older than this many hours as INTERRUPTED
         * (safety net when Gateway is SIGKILL'd and cannot write END).
         * 0 disables. Env: OPS_AUDIT_STALE_RUNNING_HOURS
         */
        int auditStaleRunningHours,
        Gitlab gitlab
) {
    public String gatewayTlsSpkiSha256Normalized() {
        if (gatewayTlsSpkiSha256 == null) {
            return "";
        }
        String s = gatewayTlsSpkiSha256.trim();
        if (s.startsWith("sha256://") || s.startsWith("sha256//")) {
            s = s.substring(s.startsWith("sha256://") ? 9 : 8);
        } else if (s.startsWith("sha256:") || s.startsWith("sha256/")) {
            s = s.substring(7);
        }
        return s.trim();
    }

    /** Prefer internal Gateway URL; fall back to public if unset (dev). */
    public String gatewayInternalHttpOrPublic() {
        if (gatewayInternalHttp != null && !gatewayInternalHttp.isBlank()) {
            return gatewayInternalHttp.trim();
        }
        return gatewayPublicHttp == null ? "" : gatewayPublicHttp.trim();
    }

    public record Gitlab(
            String baseUrl,
            String clientId,
            String clientSecret,
            String redirectUri,
            /** GitLab username(s) that become SUPER_ADMIN. Comma-separated. Env: OPS_GITLAB_ADMIN_USER */
            String adminUser
    ) {
        public boolean configured() {
            return notBlank(baseUrl) && notBlank(clientId) && notBlank(clientSecret);
        }

        private static boolean notBlank(String s) {
            return s != null && !s.isBlank();
        }
    }

    /** True if GitLab username is listed in OPS_GITLAB_ADMIN_USER (case-insensitive). */
    public boolean isGitlabAdminUser(String username) {
        if (username == null || username.isBlank() || gitlab == null || gitlab.adminUser() == null) {
            return false;
        }
        String needle = username.trim();
        for (String part : gitlab.adminUser().split(",")) {
            if (part != null && needle.equalsIgnoreCase(part.trim())) {
                return true;
            }
        }
        return false;
    }

    public boolean gitlabEnabled() {
        return gitlab != null && gitlab.configured();
    }

    /** Local password login when GitLab OAuth is not configured. */
    public boolean localLoginEnabled() {
        return !gitlabEnabled();
    }

    public String gitlabBaseUrl() {
        if (gitlab == null || gitlab.baseUrl() == null) return "";
        String u = gitlab.baseUrl().trim();
        while (u.endsWith("/")) {
            u = u.substring(0, u.length() - 1);
        }
        return u;
    }

    public String gitlabRedirectUri() {
        if (gitlab != null && gitlab.redirectUri() != null && !gitlab.redirectUri().isBlank()) {
            return gitlab.redirectUri().trim();
        }
        String base = controlPublicHttp == null ? "https://127.0.0.1:9100" : controlPublicHttp.trim();
        while (base.endsWith("/")) {
            base = base.substring(0, base.length() - 1);
        }
        return base + "/api/auth/gitlab/callback";
    }

    public String consoleBase() {
        String c = consolePublicHttp == null || consolePublicHttp.isBlank()
                ? "https://127.0.0.1:5173"
                : consolePublicHttp.trim();
        while (c.endsWith("/")) {
            c = c.substring(0, c.length() - 1);
        }
        return c;
    }
}
