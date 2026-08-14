package com.ops.control.ci;

import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;

/** Documents plaintext token layout: ops_&lt;tokenId&gt;_&lt;secret&gt;. */
class DeployTokenFormatTest {

    @Test
    void parsesOpsTokenLayout() {
        UUID id = UUID.randomUUID();
        String secret = "aabbccddeeff00112233445566778899";
        String plaintext = "ops_" + id + "_" + secret;

        assertTrue(plaintext.startsWith("ops_"));
        String rest = plaintext.substring("ops_".length());
        int sep = rest.indexOf('_');
        assertTrue(sep > 0);
        UUID parsed = UUID.fromString(rest.substring(0, sep));
        assertEquals(id, parsed);
        assertEquals(secret, rest.substring(sep + 1));
    }

    @Test
    void secretHashIsSha256Hex() throws Exception {
        String secret = "deadbeef";
        MessageDigest md = MessageDigest.getInstance("SHA-256");
        String hash = HexFormat.of().formatHex(md.digest(secret.getBytes(StandardCharsets.UTF_8)));
        assertEquals(64, hash.length());
        assertTrue(MessageDigest.isEqual(
                hash.getBytes(StandardCharsets.UTF_8),
                hash.getBytes(StandardCharsets.UTF_8)));
    }
}
