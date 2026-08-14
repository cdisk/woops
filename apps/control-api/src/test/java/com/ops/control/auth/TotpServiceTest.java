package com.ops.control.auth;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class TotpServiceTest {
    private final TotpService totp = new TotpService();

    @Test
    void rejectsBlankOrNonDigits() {
        String secret = totp.generateSecret();
        assertFalse(totp.verify(secret, null));
        assertFalse(totp.verify(secret, ""));
        assertFalse(totp.verify(secret, "abcdef"));
        assertFalse(totp.verify(secret, "12345"));
        assertFalse(totp.verify(secret, "1234567"));
        assertTrue(totp.matchingStep(secret, "abcdef").isEmpty());
    }

    @Test
    void acceptsCurrentCodeAndReturnsStep() throws Exception {
        String secret = totp.generateSecret();
        var generator = new dev.samstevens.totp.code.DefaultCodeGenerator(
                dev.samstevens.totp.code.HashingAlgorithm.SHA1, 6);
        long counter = new dev.samstevens.totp.time.SystemTimeProvider().getTime() / 30;
        String code = generator.generate(secret, counter);
        assertTrue(totp.verify(secret, code));
        assertTrue(totp.verify(secret, " " + code + " "));
        assertEquals(counter, totp.matchingStep(secret, code).orElseThrow());
    }

    @Test
    void acceptsPreviousStepWithinSkew() throws Exception {
        String secret = totp.generateSecret();
        var generator = new dev.samstevens.totp.code.DefaultCodeGenerator(
                dev.samstevens.totp.code.HashingAlgorithm.SHA1, 6);
        long counter = new dev.samstevens.totp.time.SystemTimeProvider().getTime() / 30;
        String previous = generator.generate(secret, counter - 1);
        assertEquals(counter - 1, totp.matchingStep(secret, previous).orElseThrow());
    }

    @Test
    void otpauthUriContainsIssuerAndSecret() {
        String secret = totp.generateSecret();
        String uri = totp.otpauthUri("admin", secret);
        assertTrue(uri.startsWith("otpauth://totp/"));
        assertTrue(uri.contains("secret=" + secret)
                || uri.contains("secret=" + java.net.URLEncoder.encode(secret, java.nio.charset.StandardCharsets.UTF_8)));
        assertTrue(uri.contains("issuer=Woops")
                || uri.contains("issuer=" + java.net.URLEncoder.encode("Woops", java.nio.charset.StandardCharsets.UTF_8)));
    }
}
