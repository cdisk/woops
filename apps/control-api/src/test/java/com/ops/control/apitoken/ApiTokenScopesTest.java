package com.ops.control.apitoken;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

class ApiTokenScopesTest {
    @Test
    void acceptsAssetsAndMetricsRead() {
        assertEquals(
                Set.of("assets:read", "metrics:read"),
                ApiTokenScopes.parseAndValidate(List.of("assets:read", "metrics:read")));
    }

    @Test
    void rejectsUnknownScope() {
        assertThrows(IllegalArgumentException.class,
                () -> ApiTokenScopes.parseAndValidate(List.of("assets:write")));
    }

    @Test
    void rejectsEmpty() {
        assertThrows(IllegalArgumentException.class,
                () -> ApiTokenScopes.parseAndValidate(List.of()));
    }

    @Test
    void pathScopesMapReads() {
        assertEquals(ApiTokenScopes.ASSETS_READ,
                ApiTokenPathScopes.requiredScope("GET", "/api/assets"));
        assertEquals(ApiTokenScopes.ASSETS_READ,
                ApiTokenPathScopes.requiredScope("GET", "/api/assets/11111111-1111-1111-1111-111111111111"));
        assertEquals(ApiTokenScopes.ASSETS_READ,
                ApiTokenPathScopes.requiredScope("GET", "/api/groups"));
        assertEquals(ApiTokenScopes.METRICS_READ,
                ApiTokenPathScopes.requiredScope("GET", "/api/monitor/items"));
        assertEquals(ApiTokenScopes.METRICS_READ,
                ApiTokenPathScopes.requiredScope("GET", "/api/assets/11111111-1111-1111-1111-111111111111/metrics/series"));
        assertEquals(ApiTokenScopes.METRICS_READ,
                ApiTokenPathScopes.requiredScope("POST", "/api/reports/metrics"));
        assertNull(ApiTokenPathScopes.requiredScope("PATCH", "/api/assets/11111111-1111-1111-1111-111111111111"));
        assertNull(ApiTokenPathScopes.requiredScope("POST", "/api/profile/api-tokens"));
        assertNull(ApiTokenPathScopes.requiredScope("POST", "/api/assets/x/alert-ignores"));
        assertNull(ApiTokenPathScopes.requiredScope("GET", "/api/monitor/alert-rules"));
    }

    @Test
    void tokenHashIsStable() {
        String a = ApiTokenService.sha256Hex("secret");
        String b = ApiTokenService.sha256Hex("secret");
        assertEquals(a, b);
        assertEquals(64, a.length());
        assertNotEquals(a, ApiTokenService.sha256Hex("other"));
    }
}
