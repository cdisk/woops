package com.ops.control.session;

import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetAccess;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.protocol.IssueContext;
import com.ops.control.protocol.ProtocolRegistry;
import com.ops.control.protocol.ProtocolTicketIssuer;
import com.ops.control.protocol.desktop.DesktopTicketIssuer;
import com.ops.control.protocol.portmap.PortmapTicketIssuer;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import io.jsonwebtoken.Claims;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

@Service
public class SessionTicketService {
    private final AssetRepository assets;
    private final UserRepository users;
    private final AccessService access;
    private final ProtocolRegistry protocols;
    private final TicketSigner signer;
    private final TicketVerifier verifier;
    private final TicketResponseBuilder responses;
    private final PortmapTicketIssuer portmapTickets;

    public SessionTicketService(
            AssetRepository assets,
            UserRepository users,
            AccessService access,
            ProtocolRegistry protocols,
            TicketSigner signer,
            TicketVerifier verifier,
            TicketResponseBuilder responses,
            PortmapTicketIssuer portmapTickets) {
        this.assets = assets;
        this.users = users;
        this.access = access;
        this.protocols = protocols;
        this.signer = signer;
        this.verifier = verifier;
        this.responses = responses;
        this.portmapTickets = portmapTickets;
    }

    public Map<String, Object> createTicket(UUID userId, String username, UUID assetId, String requestedProtocol) {
        return createTicket(userId, username, assetId, requestedProtocol, null, null, null, null, null, null);
    }

    public Map<String, Object> createTicket(
            UUID userId,
            String username,
            UUID assetId,
            String requestedProtocol,
            String direction,
            String path,
            Long size,
            String fingerprint,
            String transferId,
            Boolean abort) {
        AssetEntity asset = assets.findById(assetId)
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        UserEntity user = users.findById(userId)
                .orElseThrow(() -> new IllegalArgumentException("user not found"));
        access.assertCanAccessAsset(user, asset);
        assertAgentReady(asset);

        String protocol = requestedProtocol == null || requestedProtocol.isBlank()
                ? AssetAccess.protocolOf(asset)
                : requestedProtocol.toLowerCase();
        if (!AssetAccess.allows(asset, protocol)) {
            throw new IllegalArgumentException(
                    "protocol '" + protocol + "' not allowed for this asset OS");
        }

        String tid = transferId;
        if ("filetransfer".equals(protocol) && (tid == null || tid.isBlank())) {
            tid = UUID.randomUUID().toString();
        }
        IssueContext ctx = new IssueContext(
                userId, username, asset, protocol, direction, path, size, fingerprint, tid, abort);
        return mint(ctx);
    }

    /** CI/opsctl binary file-transfer ticket (upload or download). */
    public Map<String, Object> createCiFileTransferTicket(
            UUID tokenId,
            String actor,
            AssetEntity asset,
            String direction,
            String path,
            Long size,
            String fingerprint,
            String transferId,
            Boolean abort) {
        String tid = transferId == null || transferId.isBlank()
                ? UUID.randomUUID().toString()
                : transferId;
        IssueContext ctx = new IssueContext(
                tokenId, actor, asset, "filetransfer", direction, path, size, fingerprint, tid, abort);
        return mint(ctx);
    }

    /** CI/opsctl one-shot exec ticket. */
    public Map<String, Object> createCiExecTicket(UUID tokenId, String actor, AssetEntity asset) {
        return mint(IssueContext.basic(tokenId, actor, asset, "exec"));
    }

    /** CI/opsctl ephemeral forward, reverse-control, or reverse-data portmap ticket. */
    public Map<String, Object> createCiPortmapTicket(
            UUID tokenId,
            String actor,
            AssetEntity asset,
            String action,
            String protocol,
            String ephemeralId,
            String listenHost,
            int listenPort,
            String targetHost,
            int targetPort,
            String clientAddr) {
        assertAgentReady(asset);
        String type;
        String direction;
        String wsPath;
        switch (action) {
            case "port-forward" -> {
                type = protocol;
                direction = "opsctl_to_asset";
                wsPath = "/ws/opsctl/portmap-forward/" + protocol;
            }
            case "port-reverse" -> {
                type = "opsctl_portmap_reverse";
                direction = "asset_to_opsctl";
                wsPath = "/ws/opsctl/portmap-reverse-control";
            }
            case "port-reverse-connection" -> {
                type = protocol;
                direction = "asset_to_opsctl";
                wsPath = "/ws/opsctl/portmap-reverse-data";
            }
            default -> throw new IllegalArgumentException("unsupported portmap action");
        }

        UUID sessionId = UUID.randomUUID();
        PortmapTicketIssuer.IssuedTicket issued = portmapTickets.mintCiEphemeral(
                sessionId,
                asset.getId(),
                tokenId,
                actor,
                type,
                protocol,
                ephemeralId,
                direction,
                listenHost,
                listenPort,
                targetHost,
                targetPort,
                clientAddr);
        Map<String, Object> extras = new LinkedHashMap<>();
        extras.put("ephemeralId", ephemeralId);
        extras.put("direction", direction);
        extras.put("listenHost", listenHost);
        extras.put("listenPort", listenPort);
        extras.put("targetHost", targetHost);
        extras.put("targetPort", targetPort);
        extras.put("clientAddr", clientAddr == null ? "" : clientAddr);
        return responses.build(
                sessionId,
                protocol,
                issued.ticket(),
                issued.expiresAt(),
                wsPath,
                asset,
                protocol,
                extras);
    }

    private Map<String, Object> mint(IssueContext ctx) {
        ProtocolTicketIssuer issuer = protocols.resolve(ctx.requestedProtocol());
        issuer.validateAsset(ctx.asset());
        UUID sessionId = UUID.randomUUID();
        Map<String, Object> claims = issuer.buildClaims(ctx, sessionId);
        Instant now = Instant.now();
        Instant exp = now.plusSeconds(90);
        String ticket = signer.sign(sessionId.toString(), claims, now, exp);
        return responses.build(
                sessionId,
                ctx.requestedProtocol(),
                ticket,
                exp,
                issuer.wsPath(),
                ctx.asset(),
                issuer.assetSummaryProtocol(ctx.asset(), ctx.requestedProtocol()),
                issuer.responseExtras(ctx));
    }

    private static void assertAgentReady(AssetEntity asset) {
        if (asset.getAgentTokenHash() == null || asset.getAgentTokenHash().isBlank()) {
            throw new IllegalStateException("asset has no agent");
        }
        if (!asset.isOnline()) {
            throw new IllegalStateException("agent offline");
        }
    }

    /** Short-lived ticket for port-mapping TCP/UDP tunnels (forward or reverse). */
    public String createPortmapTicket(
            UUID sessionId, UUID assetId, String protocol, String targetHost, int targetPort) {
        return createPortmapTicket(sessionId, assetId, protocol, targetHost, targetPort, "gateway_to_asset");
    }

    public String createPortmapTicket(
            UUID sessionId,
            UUID assetId,
            String protocol,
            String targetHost,
            int targetPort,
            String direction) {
        return portmapTickets.mint(sessionId, assetId, protocol, targetHost, targetPort, direction);
    }

    public Claims verify(String ticket) {
        return verifier.verify(ticket);
    }

    public Map<String, Object> verifyForGateway(String ticket) {
        Claims c = verify(ticket);
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("sessionId", c.getSubject());
        out.put("type", c.get("type", String.class));
        out.put("shellKind", c.get("shellKind", String.class));
        out.put("assetId", c.get("assetId", String.class));
        out.put("userId", c.get("userId", String.class));
        out.put("username", c.get("username", String.class));
        out.put("targetHost", c.get("targetHost", String.class));
        Object port = c.get("targetPort");
        out.put("targetPort", port instanceof Number n ? n.intValue() : null);
        out.put("direction", c.get("direction", String.class));
        out.put("protocol", c.get("protocol", String.class));
        Object ephemeral = c.get("ephemeral");
        out.put("ephemeral", ephemeral instanceof Boolean b ? b : Boolean.FALSE);
        out.put("ephemeralId", c.get("ephemeralId", String.class));
        out.put("initiator", c.get("initiator", String.class));
        out.put("listenHost", c.get("listenHost", String.class));
        Object listenPort = c.get("listenPort");
        out.put("listenPort", listenPort instanceof Number n ? n.intValue() : null);
        out.put("clientAddr", c.get("clientAddr", String.class));
        out.put("desktopUsername", c.get("desktopUsername", String.class));
        out.put("desktopPassword", c.get("desktopPassword", String.class));
        out.put("hostname", c.get("hostname", String.class));
        Object colorDepth = c.get("desktopColorDepth");
        out.put("desktopColorDepth", colorDepth instanceof Number n ? n.intValue() : null);
        out.put("desktopRdpQuality", c.get("desktopRdpQuality", String.class));
        out.put("transferId", c.get("transferId", String.class));
        out.put("path", c.get("path", String.class));
        Object transferSize = c.get("size");
        if (transferSize instanceof Number n) {
            out.put("size", n.longValue());
        } else {
            out.put("size", null);
        }
        out.put("fingerprint", c.get("fingerprint", String.class));
        Object abort = c.get("abort");
        out.put("abort", abort instanceof Boolean b ? b : Boolean.FALSE);
        return out;
    }

    /** @see DesktopTicketIssuer#preferRdpDialHost(String) */
    static String preferRdpDialHost(String privateIpCsv) {
        return DesktopTicketIssuer.preferRdpDialHost(privateIpCsv);
    }

    /** @see DesktopTicketIssuer#isRfc1918Ipv4(String) */
    static boolean isRfc1918Ipv4(String ip) {
        return DesktopTicketIssuer.isRfc1918Ipv4(ip);
    }
}
