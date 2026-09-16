package com.ops.control.apitoken;

/**
 * Maps HTTP method + path to required API token scope.
 * Null means API tokens are not allowed (JWT-only).
 */
public final class ApiTokenPathScopes {
    private static final String UUID =
            "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";

    private ApiTokenPathScopes() {}

    public static String requiredScope(String method, String path) {
        if (method == null || path == null) return null;
        String m = method.toUpperCase();

        if ("GET".equals(m) && "/api/assets".equals(path)) {
            return ApiTokenScopes.ASSETS_READ;
        }
        if ("GET".equals(m) && path.matches("/api/assets/" + UUID)) {
            return ApiTokenScopes.ASSETS_READ;
        }
        if ("GET".equals(m) && "/api/groups".equals(path)) {
            return ApiTokenScopes.ASSETS_READ;
        }

        if ("GET".equals(m) && "/api/monitor/items".equals(path)) {
            return ApiTokenScopes.METRICS_READ;
        }
        if ("GET".equals(m) && "/api/dashboard/summary".equals(path)) {
            return ApiTokenScopes.METRICS_READ;
        }
        if ("GET".equals(m) && path.matches("/api/assets/" + UUID + "/metrics/latest")) {
            return ApiTokenScopes.METRICS_READ;
        }
        if ("GET".equals(m) && path.matches("/api/assets/" + UUID + "/metrics/series")) {
            return ApiTokenScopes.METRICS_READ;
        }
        if ("POST".equals(m) && "/api/reports/metrics".equals(path)) {
            return ApiTokenScopes.METRICS_READ;
        }
        return null;
    }
}
