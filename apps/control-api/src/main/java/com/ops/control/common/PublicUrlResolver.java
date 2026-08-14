package com.ops.control.common;

import jakarta.servlet.http.HttpServletRequest;

import java.net.URI;

/**
 * Resolve install / public endpoints.
 * <p>
 * Prefer configured {@code OPS_GATEWAY_PUBLIC_*} / {@code OPS_CONTROL_PUBLIC_HTTP} when they
 * point at a non-loopback host. Only when those are still loopback (local dev), rewrite from
 * the browser Origin/Host so LAN clients get a reachable IP without editing config.
 */
public final class PublicUrlResolver {
    private PublicUrlResolver() {}

    public record Endpoints(String apiBase, String gatewayHttp, String gatewayWs) {}

    public static Endpoints resolve(HttpServletRequest request, OpsProperties props) {
        if (!isLoopbackBase(props.gatewayPublicHttp()) || !isLoopbackBase(props.controlPublicHttp())) {
            return fromConfig(props);
        }

        String host = extractHost(request);
        if (host == null || host.isBlank() || isLoopbackHost(host)) {
            return fromConfig(props);
        }

        boolean https = prefersHttps(request, props.gatewayPublicHttp());
        String httpScheme = https ? "https" : "http";
        String wsScheme = https ? "wss" : "ws";
        int apiPort = portOr(props.controlPublicHttp(), https ? 443 : 9100);
        int gatewayPort = portOr(props.gatewayPublicHttp(), https ? 443 : 9200);
        return new Endpoints(
                httpScheme + "://" + host + ":" + apiPort,
                httpScheme + "://" + host + ":" + gatewayPort,
                wsScheme + "://" + host + ":" + gatewayPort
        );
    }

    private static Endpoints fromConfig(OpsProperties props) {
        return new Endpoints(
                trimSlash(props.controlPublicHttp()),
                trimSlash(props.gatewayPublicHttp()),
                trimSlash(props.gatewayPublicWs())
        );
    }

    private static boolean prefersHttps(HttpServletRequest request, String gatewayPublicHttp) {
        if (gatewayPublicHttp != null && gatewayPublicHttp.toLowerCase().startsWith("https://")) {
            return true;
        }
        return "https".equalsIgnoreCase(extractScheme(request));
    }

    static boolean isLoopbackBase(String base) {
        if (base == null || base.isBlank()) {
            return true;
        }
        try {
            String withScheme = base.contains("://") ? base : "http://" + base;
            return isLoopbackHost(URI.create(withScheme).getHost());
        } catch (Exception e) {
            return true;
        }
    }

    static boolean isLoopbackHost(String host) {
        if (host == null || host.isBlank()) {
            return true;
        }
        return "localhost".equalsIgnoreCase(host)
                || "127.0.0.1".equals(host)
                || "::1".equals(host)
                || "[::1]".equals(host);
    }

    static int portOr(String base, int fallback) {
        if (base == null || base.isBlank()) {
            return fallback;
        }
        try {
            String withScheme = base.contains("://") ? base : "http://" + base;
            int p = URI.create(withScheme).getPort();
            return p > 0 ? p : fallback;
        } catch (Exception e) {
            return fallback;
        }
    }

    private static String trimSlash(String s) {
        if (s == null) {
            return "";
        }
        return s.endsWith("/") ? s.substring(0, s.length() - 1) : s;
    }

    private static String extractHost(HttpServletRequest request) {
        String origin = request.getHeader("Origin");
        if (origin != null && !origin.isBlank()) {
            try {
                return URI.create(origin).getHost();
            } catch (Exception ignored) {
            }
        }
        String referer = request.getHeader("Referer");
        if (referer != null && !referer.isBlank()) {
            try {
                return URI.create(referer).getHost();
            } catch (Exception ignored) {
            }
        }
        String forwarded = request.getHeader("X-Forwarded-Host");
        if (forwarded != null && !forwarded.isBlank()) {
            String first = forwarded.split(",")[0].trim();
            int idx = first.indexOf(':');
            return idx > 0 ? first.substring(0, idx) : first;
        }
        String hostHeader = request.getHeader("Host");
        if (hostHeader != null && !hostHeader.isBlank()) {
            int idx = hostHeader.indexOf(':');
            return idx > 0 ? hostHeader.substring(0, idx) : hostHeader;
        }
        return null;
    }

    private static String extractScheme(HttpServletRequest request) {
        String forwarded = request.getHeader("X-Forwarded-Proto");
        if (forwarded != null && !forwarded.isBlank()) {
            return forwarded.split(",")[0].trim();
        }
        String origin = request.getHeader("Origin");
        if (origin != null && origin.startsWith("https")) {
            return "https";
        }
        return request.getScheme() == null ? "https" : request.getScheme();
    }
}
