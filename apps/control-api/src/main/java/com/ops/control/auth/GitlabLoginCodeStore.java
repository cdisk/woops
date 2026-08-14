package com.ops.control.auth;

import org.springframework.stereotype.Component;

import java.security.SecureRandom;
import java.time.Duration;
import java.util.HexFormat;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/** One-time GitLab console login payload; never put the access JWT in the redirect URL. */
@Component
public class GitlabLoginCodeStore {
    static final Duration TTL = Duration.ofSeconds(60);

    private final ConcurrentHashMap<String, Entry> codes = new ConcurrentHashMap<>();
    private final SecureRandom random = new SecureRandom();
    private final LoginRateLimiter.LongSupplier clock;

    public GitlabLoginCodeStore() {
        this(System::currentTimeMillis);
    }

    GitlabLoginCodeStore(LoginRateLimiter.LongSupplier clock) {
        this.clock = clock;
    }

    public String put(Map<String, Object> payload) {
        prune();
        byte[] buf = new byte[24];
        random.nextBytes(buf);
        String code = HexFormat.of().formatHex(buf);
        codes.put(code, new Entry(payload, clock.getAsLong() + TTL.toMillis()));
        return code;
    }

    public Map<String, Object> take(String code) {
        if (code == null || code.isBlank()) {
            throw new IllegalArgumentException("invalid or expired code");
        }
        Entry e = codes.remove(code.trim());
        long now = clock.getAsLong();
        if (e == null || e.expiresAtMs <= now) {
            throw new IllegalArgumentException("invalid or expired code");
        }
        return e.payload;
    }

    private void prune() {
        long now = clock.getAsLong();
        codes.entrySet().removeIf(en -> en.getValue().expiresAtMs <= now);
    }

    private record Entry(Map<String, Object> payload, long expiresAtMs) {}
}
