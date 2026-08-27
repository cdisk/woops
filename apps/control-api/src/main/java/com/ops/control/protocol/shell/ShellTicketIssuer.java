package com.ops.control.protocol.shell;

import com.ops.control.asset.AssetEntity;
import com.ops.control.protocol.IssueContext;
import com.ops.control.protocol.ProtocolTicketIssuer;
import org.springframework.stereotype.Component;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Component
public class ShellTicketIssuer implements ProtocolTicketIssuer {
    @Override
    public Set<String> requestProtocols() {
        return Set.of("shell_bash", "shell_powershell", "shell_cmd");
    }

    @Override
    public String claimType() {
        return "shell";
    }

    @Override
    public void validateAsset(AssetEntity asset) {
        // OS allow-list enforced by SessionTicketService / AssetAccess before resolve.
    }

    @Override
    public Map<String, Object> buildClaims(IssueContext ctx, UUID sessionId) {
        String shellKind = shellKind(ctx.requestedProtocol());
        Map<String, Object> claims = ProtocolTicketIssuer.baseClaims(
                claimType(), sessionId, ctx.asset(), ctx.userId(), ctx.username());
        claims.put("shellKind", shellKind);
        return claims;
    }

    @Override
    public String wsPath() {
        return "/ws/shell";
    }

    @Override
    public Map<String, Object> responseExtras(IssueContext ctx) {
        Map<String, Object> extra = new LinkedHashMap<>();
        extra.put("shellKind", shellKind(ctx.requestedProtocol()));
        return extra;
    }

    private static String shellKind(String protocol) {
        return protocol.substring("shell_".length()); // bash | powershell
    }
}
