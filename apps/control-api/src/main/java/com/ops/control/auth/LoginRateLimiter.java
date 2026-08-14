package com.ops.control.auth;

import org.springframework.stereotype.Component;

import java.time.Duration;
import java.util.concurrent.ConcurrentHashMap;

/**
 * In-memory lockout for local login / TOTP / GitLab code exchange.
 * Single control-api instance; restart clears counters.
 */
@Component
public class LoginRateLimiter {
    static final int MAX_FAILURES = 10;
    static final Duration LOCKOUT = Duration.ofMinutes(15);

    private final ConcurrentHashMap<String, Window> windows = new ConcurrentHashMap<>();
    private final LongSupplier clock;

    public LoginRateLimiter() {
        this(System::currentTimeMillis);
    }

    LoginRateLimiter(LongSupplier clock) {
        this.clock = clock;
    }

    public static String key(String clientIp, String identity) {
        String ip = clientIp == null || clientIp.isBlank() ? "-" : clientIp.trim();
        String id = identity == null || identity.isBlank() ? "-" : identity.trim().toLowerCase();
        return ip + "\0" + id;
    }

    public void assertNotLocked(String key) {
        long now = clock.getAsLong();
        Window w = windows.get(key);
        if (w == null) {
            return;
        }
        if (w.lockUntilMs > now) {
            throw new TooManyRequestsException("too many attempts");
        }
        if (w.lockUntilMs > 0 && w.lockUntilMs <= now) {
            windows.remove(key, w);
        }
    }

    public void recordFailure(String key) {
        long now = clock.getAsLong();
        windows.compute(key, (k, existing) -> {
            Window w = existing;
            if (w == null || (w.lockUntilMs > 0 && w.lockUntilMs <= now)) {
                w = new Window();
            }
            if (w.lockUntilMs > now) {
                return w;
            }
            w.failures++;
            if (w.failures >= MAX_FAILURES) {
                w.lockUntilMs = now + LOCKOUT.toMillis();
            }
            return w;
        });
    }

    public void recordSuccess(String key) {
        windows.remove(key);
    }

    @FunctionalInterface
    interface LongSupplier {
        long getAsLong();
    }

    private static final class Window {
        int failures;
        long lockUntilMs;
    }
}
