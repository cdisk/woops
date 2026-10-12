package com.ops.control.metrics.notify;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;

class WebhookUrlGuardTest {

    @Test
    void rejectsLoopback() {
        assertThrows(IllegalArgumentException.class, () -> WebhookUrlGuard.assertSafe("http://127.0.0.1:8080/hook"));
        assertThrows(IllegalArgumentException.class, () -> WebhookUrlGuard.assertSafe("http://localhost/hook"));
    }

    @Test
    void rejectsNonHttp() {
        assertThrows(IllegalArgumentException.class, () -> WebhookUrlGuard.assertSafe("ftp://example.com/x"));
    }

    @Test
    void allowsHttpsPublicHost() {
        assertDoesNotThrow(() -> WebhookUrlGuard.assertSafe("https://example.com/hooks/alerts"));
    }
}
