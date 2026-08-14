package com.ops.control.session;

import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.OpsProperties;
import com.ops.control.group.GroupService;
import com.ops.control.protocol.ProtocolRegistry;
import com.ops.control.protocol.portmap.PortmapTicketIssuer;
import com.ops.control.user.UserRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;

class SessionTicketCiPortmapClaimsTest {
    private SessionTicketService tickets;
    private AssetEntity asset;
    private UUID tokenId;

    @BeforeEach
    void setUp() {
        OpsProperties props = new OpsProperties(
                "https://127.0.0.1:9100",
                "https://127.0.0.1:9200",
                "wss://127.0.0.1:9200",
                "http://127.0.0.1:9201",
                "https://127.0.0.1:5173",
                "jwt-secret-must-be-long-enough-0123456789abcd",
                "ticket-secret-must-be-long-enough-0123456789ab",
                "admin",
                "admin123",
                15,
                "",
                "./data/ops-audit",
                5,
                90,
                24,
                null);
        TicketSigner signer = new TicketSigner(props);
        signer.initForTests();
        TicketResponseBuilder responses =
                new TicketResponseBuilder(props, mock(GroupService.class));
        tickets = new SessionTicketService(
                mock(AssetRepository.class),
                mock(UserRepository.class),
                mock(AccessService.class),
                new ProtocolRegistry(List.of()),
                signer,
                new TicketVerifier(signer),
                responses,
                new PortmapTicketIssuer(signer));

        tokenId = UUID.randomUUID();
        asset = new AssetEntity();
        asset.setId(UUID.randomUUID());
        asset.setDisplayName("ci-target");
        asset.setOnline(true);
        asset.setAgentTokenHash("hash");
    }

    @Test
    void forwardTicketContainsEphemeralClaimsAndTcpPath() {
        Map<String, Object> response = issue("port-forward", "tcp", null);
        Map<String, Object> claims = tickets.verifyForGateway((String) response.get("ticket"));

        assertEquals("tcp", claims.get("type"));
        assertEquals("tcp", claims.get("protocol"));
        assertEquals("opsctl_to_asset", claims.get("direction"));
        assertEquals("", claims.get("clientAddr"));
        assertCommon(response, claims, "/ws/opsctl/portmap-forward/tcp");
    }

    @Test
    void reverseControlAndDataUseDistinctTypesAndPaths() {
        Map<String, Object> control = issue("port-reverse", "udp", "10.0.0.5:32000");
        Map<String, Object> controlClaims =
                tickets.verifyForGateway((String) control.get("ticket"));
        assertEquals("opsctl_portmap_reverse", controlClaims.get("type"));
        assertEquals("asset_to_opsctl", controlClaims.get("direction"));
        assertEquals("10.0.0.5:32000", controlClaims.get("clientAddr"));
        assertCommon(control, controlClaims, "/ws/opsctl/portmap-reverse-control");

        Map<String, Object> data = issue("port-reverse-connection", "udp", "10.0.0.5:32000");
        Map<String, Object> dataClaims = tickets.verifyForGateway((String) data.get("ticket"));
        assertEquals("udp", dataClaims.get("type"));
        assertEquals("asset_to_opsctl", dataClaims.get("direction"));
        assertCommon(data, dataClaims, "/ws/opsctl/portmap-reverse-data");
    }

    private Map<String, Object> issue(String action, String protocol, String clientAddr) {
        return tickets.createCiPortmapTicket(
                tokenId,
                "token:" + tokenId.toString().substring(0, 8),
                asset,
                action,
                protocol,
                "opsctl:ephemeral-123",
                "127.0.0.1",
                18080,
                "10.0.0.8",
                8080,
                clientAddr);
    }

    private void assertCommon(
            Map<String, Object> response, Map<String, Object> claims, String expectedPath) {
        assertEquals(asset.getId().toString(), claims.get("assetId"));
        assertEquals(tokenId.toString(), claims.get("userId"));
        assertEquals("token:" + tokenId.toString().substring(0, 8), claims.get("username"));
        assertEquals(true, claims.get("ephemeral"));
        assertEquals("opsctl:ephemeral-123", claims.get("ephemeralId"));
        assertEquals("opsctl", claims.get("initiator"));
        assertEquals("127.0.0.1", claims.get("listenHost"));
        assertEquals(18080, claims.get("listenPort"));
        assertEquals("10.0.0.8", claims.get("targetHost"));
        assertEquals(8080, claims.get("targetPort"));

        assertEquals("opsctl:ephemeral-123", response.get("ephemeralId"));
        assertTrue(String.valueOf(response.get("browserWs")).contains(expectedPath + "?ticket="));
        assertTrue(response.containsKey("sessionId"));
        assertTrue(response.containsKey("protocol"));
        assertTrue(response.containsKey("ticket"));
        Instant expiresAt = Instant.parse((String) response.get("expiresAt"));
        long ttl = Duration.between(Instant.now(), expiresAt).toSeconds();
        assertTrue(ttl >= 118 && ttl <= 120, "expected a 120s ticket, got " + ttl + "s");
    }
}
