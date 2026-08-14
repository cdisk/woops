package com.ops.control.auth;

import org.junit.jupiter.api.Test;

import java.util.Map;
import java.util.concurrent.atomic.AtomicLong;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class GitlabLoginCodeStoreTest {
    @Test
    void takeIsOneShotAndExpires() {
        AtomicLong now = new AtomicLong(1_000);
        GitlabLoginCodeStore store = new GitlabLoginCodeStore(now::get);
        Map<String, Object> payload = Map.of("accessToken", "jwt-here");
        String code = store.put(payload);
        assertEquals("jwt-here", store.take(code).get("accessToken"));
        assertThrows(IllegalArgumentException.class, () -> store.take(code));

        String later = store.put(payload);
        now.addAndGet(GitlabLoginCodeStore.TTL.toMillis() + 1);
        assertThrows(IllegalArgumentException.class, () -> store.take(later));
    }
}
