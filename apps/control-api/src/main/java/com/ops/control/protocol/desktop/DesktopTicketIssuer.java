package com.ops.control.protocol.desktop;

import com.ops.control.asset.AssetEntity;
import com.ops.control.protocol.IssueContext;
import com.ops.control.protocol.ProtocolTicketIssuer;
import org.springframework.stereotype.Component;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Component
public class DesktopTicketIssuer implements ProtocolTicketIssuer {
    @Override
    public Set<String> requestProtocols() {
        return Set.of("rdp", "vnc");
    }

    @Override
    public String claimType() {
        // Overridden per request in buildClaims (rdp | vnc).
        return "desktop";
    }

    @Override
    public void validateAsset(AssetEntity asset) {
        // Password/port checked in buildClaims with protocol-specific messages (RDP/VNC).
    }

    @Override
    public Map<String, Object> buildClaims(IssueContext ctx, UUID sessionId) {
        String protocol = ctx.requestedProtocol();
        ensureDesktopReady(ctx.asset(), protocol);

        String targetHost = "127.0.0.1";
        if ("rdp".equals(protocol)) {
            targetHost = preferRdpDialHost(ctx.asset().getPrivateIp());
        }
        int targetPort = ctx.asset().getDesktopPort();
        String password = ctx.asset().getDesktopPassword();

        Map<String, Object> claims = ProtocolTicketIssuer.baseClaims(
                protocol, sessionId, ctx.asset(), ctx.userId(), ctx.username());
        claims.put("targetHost", targetHost);
        claims.put("targetPort", targetPort);
        claims.put("desktopUsername", ctx.asset().getDesktopUsername() == null ? "" : ctx.asset().getDesktopUsername());
        claims.put("desktopPassword", password);
        claims.put("hostname", ctx.asset().getHostname() == null ? "" : ctx.asset().getHostname());
        if ("rdp".equals(protocol)) {
            claims.put("desktopColorDepth", normalizeColorDepth(ctx.asset().getDesktopColorDepth()));
            claims.put("desktopRdpQuality", normalizeRdpQuality(ctx.asset().getDesktopRdpQuality()));
        }
        return claims;
    }

    @Override
    public String wsPath() {
        return "/ws/desktop";
    }

    @Override
    public Map<String, Object> responseExtras(IssueContext ctx) {
        String protocol = ctx.requestedProtocol();
        ensureDesktopReady(ctx.asset(), protocol);
        String targetHost = "127.0.0.1";
        if ("rdp".equals(protocol)) {
            targetHost = preferRdpDialHost(ctx.asset().getPrivateIp());
        }
        Map<String, Object> extra = new LinkedHashMap<>();
        extra.put("targetHost", targetHost);
        extra.put("targetPort", ctx.asset().getDesktopPort());
        return extra;
    }

    @Override
    public String assetSummaryProtocol(AssetEntity asset, String requestedProtocol) {
        return requestedProtocol;
    }

    private static void ensureDesktopReady(AssetEntity asset, String protocol) {
        String password = asset.getDesktopPassword();
        if (password == null || password.isBlank()) {
            throw new IllegalStateException(
                    protocol.toUpperCase() + " 需要目标机登录密码，请先在资产「详情」中填写后再连接");
        }
        if (asset.getDesktopPort() <= 0) {
            throw new IllegalStateException(
                    protocol.toUpperCase() + " 端口无效，请先在资产「详情」中配置桌面端口");
        }
    }

    /**
     * Pick Agent-side RDP dial address from {@code assets.private_ip} CSV.
     * Only RFC1918 addresses are used; virtual/VPN/non-standard intranet IPs
     * fall back to loopback (Agent is on the same host as the RDP service).
     */
    public static String preferRdpDialHost(String privateIpCsv) {
        if (privateIpCsv == null || privateIpCsv.isBlank()) {
            return "127.0.0.1";
        }
        for (String part : privateIpCsv.trim().split("[,\\s]+")) {
            String ip = part.trim();
            if (ip.isEmpty()) {
                continue;
            }
            if (isRfc1918Ipv4(ip)) {
                return ip;
            }
        }
        return "127.0.0.1";
    }

    /** RFC1918 only (not APIPA / loopback). */
    public static boolean isRfc1918Ipv4(String ip) {
        if (ip == null || ip.isBlank()) {
            return false;
        }
        String s = ip.trim();
        if (s.startsWith("10.")) {
            return s.length() > 3;
        }
        if (s.startsWith("192.168.")) {
            return s.length() > 8;
        }
        return s.matches("172\\.(1[6-9]|2[0-9]|3[0-1])\\.\\d{1,3}\\.\\d{1,3}");
    }

    private static int normalizeColorDepth(int depth) {
        return switch (depth) {
            case 8, 16, 24, 32 -> depth;
            default -> 16;
        };
    }

    private static String normalizeRdpQuality(String quality) {
        if (quality == null || quality.isBlank()) {
            return "low";
        }
        return switch (quality.trim().toLowerCase()) {
            case "low", "medium", "high" -> quality.trim().toLowerCase();
            default -> "low";
        };
    }
}
