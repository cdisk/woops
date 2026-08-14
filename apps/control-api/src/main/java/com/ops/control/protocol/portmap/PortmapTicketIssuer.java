package com.ops.control.protocol.portmap;

import com.ops.control.session.TicketSigner;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

/**
 * Mints short-lived raw JWT strings for port-mapping tunnels (120s TTL).
 * Not part of {@link com.ops.control.protocol.ProtocolTicketIssuer} / browser session flow.
 */
@Component
public class PortmapTicketIssuer {
    public record IssuedTicket(String ticket, Instant expiresAt) {}

    private final TicketSigner signer;

    public PortmapTicketIssuer(TicketSigner signer) {
        this.signer = signer;
    }

    public String mint(
            UUID sessionId,
            UUID assetId,
            String protocol,
            String targetHost,
            int targetPort,
            String direction) {
        Instant now = Instant.now();
        Instant exp = now.plusSeconds(120);
        Map<String, Object> claims = new LinkedHashMap<>();
        claims.put("type", protocol);
        claims.put("assetId", assetId.toString());
        claims.put("targetHost", targetHost);
        claims.put("targetPort", targetPort);
        claims.put("direction", direction == null || direction.isBlank() ? "gateway_to_asset" : direction);
        return signer.sign(sessionId.toString(), claims, now, exp);
    }

    /** Mints an opsctl ephemeral port-mapping ticket with its response expiry. */
    public IssuedTicket mintCiEphemeral(
            UUID sessionId,
            UUID assetId,
            UUID tokenId,
            String actor,
            String type,
            String protocol,
            String ephemeralId,
            String direction,
            String listenHost,
            int listenPort,
            String targetHost,
            int targetPort,
            String clientAddr) {
        Instant now = Instant.now();
        Instant exp = now.plusSeconds(120);
        Map<String, Object> claims = new LinkedHashMap<>();
        claims.put("type", type);
        claims.put("protocol", protocol);
        claims.put("assetId", assetId.toString());
        claims.put("userId", tokenId.toString());
        claims.put("username", actor);
        claims.put("ephemeral", true);
        claims.put("ephemeralId", ephemeralId);
        claims.put("direction", direction);
        claims.put("initiator", "opsctl");
        claims.put("listenHost", listenHost);
        claims.put("listenPort", listenPort);
        claims.put("targetHost", targetHost);
        claims.put("targetPort", targetPort);
        claims.put("clientAddr", clientAddr == null ? "" : clientAddr);
        return new IssuedTicket(signer.sign(sessionId.toString(), claims, now, exp), exp);
    }
}
