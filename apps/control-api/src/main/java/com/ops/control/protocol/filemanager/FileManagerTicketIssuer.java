package com.ops.control.protocol.filemanager;

import com.ops.control.asset.AssetEntity;
import com.ops.control.protocol.IssueContext;
import com.ops.control.protocol.ProtocolTicketIssuer;
import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Component
public class FileManagerTicketIssuer implements ProtocolTicketIssuer {
    @Override
    public Set<String> requestProtocols() {
        return Set.of("filemanager");
    }

    @Override
    public String claimType() {
        return "filemanager";
    }

    @Override
    public void validateAsset(AssetEntity asset) {
        // Always allowed for any OS when asset has an agent.
    }

    @Override
    public Map<String, Object> buildClaims(IssueContext ctx, UUID sessionId) {
        return ProtocolTicketIssuer.baseClaims(
                claimType(), sessionId, ctx.asset(), ctx.userId(), ctx.username());
    }

    @Override
    public String wsPath() {
        return "/ws/file-manager";
    }

    @Override
    public Map<String, Object> responseExtras(IssueContext ctx) {
        return Map.of();
    }
}
