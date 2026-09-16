package com.ops.control.apitoken;

import java.util.Arrays;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.stream.Collectors;

/** Extensible API token scopes (server-side allowlist). */
public final class ApiTokenScopes {
    public static final String METRICS_READ = "metrics:read";
    public static final String ASSETS_READ = "assets:read";

    private static final Set<String> ALLOWED = Set.of(METRICS_READ, ASSETS_READ);

    private ApiTokenScopes() {}

    /** Stable UI / docs order. */
    public static List<String> all() {
        return List.of(ASSETS_READ, METRICS_READ);
    }

    public static Set<String> parseAndValidate(Iterable<String> raw) {
        Set<String> out = new LinkedHashSet<>();
        if (raw == null) {
            throw new IllegalArgumentException("scopes required");
        }
        for (String s : raw) {
            if (s == null || s.isBlank()) continue;
            String v = s.trim().toLowerCase(Locale.ROOT);
            if (!ALLOWED.contains(v)) {
                throw new IllegalArgumentException("unsupported scope: " + s);
            }
            out.add(v);
        }
        if (out.isEmpty()) {
            throw new IllegalArgumentException("scopes required");
        }
        return out;
    }

    public static Set<String> fromStored(String csv) {
        if (csv == null || csv.isBlank()) return Set.of();
        return Arrays.stream(csv.split(","))
                .map(String::trim)
                .filter(s -> !s.isEmpty())
                .collect(Collectors.toCollection(LinkedHashSet::new));
    }

    public static String toStored(Set<String> scopes) {
        return String.join(",", scopes);
    }

    public static boolean isAllowed(String scope) {
        return scope != null && ALLOWED.contains(scope);
    }
}
