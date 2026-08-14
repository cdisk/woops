package com.ops.control.protocol.exec;

import com.ops.control.asset.AssetAccess;
import com.ops.control.asset.AssetEntity;
import com.ops.control.protocol.IssueContext;
import com.ops.control.protocol.ProtocolTicketIssuer;
import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Component
public class ExecTicketIssuer implements ProtocolTicketIssuer {
    @Override
    public Set<String> requestProtocols() {
        return Set.of("exec");
    }

    @Override
    public String claimType() {
        return "exec";
    }

    @Override
    public void validateAsset(AssetEntity asset) {
        if (!AssetAccess.allows(asset, "exec")) {
            throw new IllegalArgumentException("protocol 'exec' not allowed for this asset OS");
        }
    }

    @Override
    public Map<String, Object> buildClaims(IssueContext ctx, UUID sessionId) {
        return ProtocolTicketIssuer.baseClaims(
                claimType(), sessionId, ctx.asset(), ctx.userId(), ctx.username());
    }

    @Override
    public String wsPath() {
        return "/ws/exec";
    }

    @Override
    public Map<String, Object> responseExtras(IssueContext ctx) {
        return Map.of();
    }
}
