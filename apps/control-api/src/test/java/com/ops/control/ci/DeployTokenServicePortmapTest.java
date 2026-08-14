package com.ops.control.ci;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.OpsProperties;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.session.SessionTicketService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class DeployTokenServicePortmapTest {
    private DeployTokenRepository tokenRepository;
    private AssetRepository assetRepository;
    private SessionTicketService ticketService;
    private DeployTokenService service;
    private DeployTokenEntity token;
    private AssetEntity asset;
    private String plaintext;

    @BeforeEach
    void setUp() throws Exception {
        tokenRepository = mock(DeployTokenRepository.class);
        assetRepository = mock(AssetRepository.class);
        ticketService = mock(SessionTicketService.class);
        service = new DeployTokenService(
                tokenRepository,
                assetRepository,
                mock(AccessService.class),
                mock(ControlAuditService.class),
                ticketService,
                mock(OpsProperties.class),
                new ObjectMapper());

        UUID tokenId = UUID.randomUUID();
        UUID assetId = UUID.randomUUID();
        String secret = "portmap-secret";
        plaintext = "ops_" + tokenId + "_" + secret;

        token = new DeployTokenEntity();
        token.setId(tokenId);
        token.setAssetId(assetId);
        token.setSecretHash(sha256Hex(secret));
        token.setAllowForward(true);
        token.setAllowReverse(true);

        asset = new AssetEntity();
        asset.setId(assetId);
        asset.setDisplayName("ci-target");
        asset.setOnline(true);
        asset.setAgentTokenHash("hash");

        when(tokenRepository.findById(tokenId)).thenReturn(Optional.of(token));
        when(assetRepository.findById(assetId)).thenReturn(Optional.of(asset));
        when(ticketService.createCiPortmapTicket(
                        eq(tokenId),
                        any(),
                        eq(asset),
                        any(),
                        any(),
                        any(),
                        any(),
                        any(Integer.class),
                        any(),
                        any(Integer.class),
                        any()))
                .thenReturn(Map.of("ticket", "signed"));
    }

    @ParameterizedTest
    @ValueSource(strings = {"port-forward", "port-reverse", "port-reverse-connection"})
    void allowsEachPortmapActionWithCompleteMeta(String action) {
        Map<String, Object> result = service.createOpsctlTicket(plaintext, action, validMeta());

        assertEquals("signed", result.get("ticket"));
        verify(ticketService).createCiPortmapTicket(
                eq(token.getId()),
                eq("token:" + token.getId().toString().substring(0, 8)),
                eq(asset),
                eq(action),
                eq("tcp"),
                eq("opsctl:ephemeral-123"),
                eq("127.0.0.1"),
                eq(18080),
                eq("10.0.0.8"),
                eq(8080),
                eq(null));
    }

    @Test
    void rejectsForwardWithoutScope() {
        token.setAllowForward(false);

        IllegalArgumentException error = assertThrows(
                IllegalArgumentException.class,
                () -> service.createOpsctlTicket(plaintext, "port-forward", validMeta()));

        assertEquals("token does not allow forward", error.getMessage());
        verify(ticketService, never()).createCiPortmapTicket(
                any(), any(), any(), any(), any(), any(), any(), any(Integer.class),
                any(), any(Integer.class), any());
    }

    @Test
    void rejectsReverseWithoutScope() {
        token.setAllowReverse(false);

        IllegalArgumentException error = assertThrows(
                IllegalArgumentException.class,
                () -> service.createOpsctlTicket(plaintext, "port-reverse", validMeta()));

        assertEquals("token does not allow reverse", error.getMessage());
    }

    @Test
    void rejectsDownloadWithoutScope() {
        token.setAllowDownload(false);

        IllegalArgumentException error = assertThrows(
                IllegalArgumentException.class,
                () -> service.createOpsctlTicket(plaintext, "download", Map.of("remotePath", "/tmp/a")));

        assertEquals("token does not allow download", error.getMessage());
    }

    @Test
    void rejectsInvalidPortmapProtocol() {
        Map<String, Object> meta = validMeta();
        meta.put("protocol", "http");

        IllegalArgumentException error = assertThrows(
                IllegalArgumentException.class,
                () -> service.createOpsctlTicket(plaintext, "port-forward", meta));

        assertEquals("meta.protocol must be tcp or udp", error.getMessage());
    }

    @Test
    void rejectsPersistentMappingIdNamespace() {
        Map<String, Object> meta = validMeta();
        meta.put("ephemeralId", UUID.randomUUID().toString());

        IllegalArgumentException error = assertThrows(
                IllegalArgumentException.class,
                () -> service.createOpsctlTicket(plaintext, "port-reverse", meta));

        assertEquals("meta.ephemeralId must use opsctl namespace", error.getMessage());
    }

    private static Map<String, Object> validMeta() {
        return new java.util.LinkedHashMap<>(Map.of(
                "protocol", "tcp",
                "ephemeralId", "opsctl:ephemeral-123",
                "listenHost", "127.0.0.1",
                "listenPort", 18080,
                "targetHost", "10.0.0.8",
                "targetPort", 8080));
    }

    private static String sha256Hex(String value) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        return HexFormat.of().formatHex(digest.digest(value.getBytes(StandardCharsets.UTF_8)));
    }
}
