package com.ops.control.auth;

import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicLong;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;

class LoginRateLimiterTest {
    @Test
    void locksAfterMaxFailuresAndClearsOnSuccess() {
        AtomicLong now = new AtomicLong(1_000_000);
        LoginRateLimiter limiter = new LoginRateLimiter(now::get);
        String key = LoginRateLimiter.key("10.0.0.1", "admin");

        for (int i = 0; i < LoginRateLimiter.MAX_FAILURES; i++) {
            limiter.assertNotLocked(key);
            limiter.recordFailure(key);
        }
        assertThrows(TooManyRequestsException.class, () -> limiter.assertNotLocked(key));

        now.addAndGet(LoginRateLimiter.LOCKOUT.toMillis() + 1);
        assertDoesNotThrow(() -> limiter.assertNotLocked(key));

        limiter.recordFailure(key);
        limiter.recordSuccess(key);
        assertDoesNotThrow(() -> limiter.assertNotLocked(key));
    }

    @Test
    void successResetsFailureCount() {
        AtomicLong now = new AtomicLong(1);
        LoginRateLimiter limiter = new LoginRateLimiter(now::get);
        String key = LoginRateLimiter.key("10.0.0.1", "admin");
        for (int i = 0; i < LoginRateLimiter.MAX_FAILURES - 1; i++) {
            limiter.recordFailure(key);
        }
        limiter.recordSuccess(key);
        for (int i = 0; i < LoginRateLimiter.MAX_FAILURES - 1; i++) {
            limiter.recordFailure(key);
        }
        assertDoesNotThrow(() -> limiter.assertNotLocked(key));
    }
}
