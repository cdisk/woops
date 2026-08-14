package com.ops.control.protocol.filetransfer;

import com.ops.control.asset.AssetAccess;
import com.ops.control.asset.AssetEntity;
import com.ops.control.protocol.IssueContext;
import com.ops.control.protocol.ProtocolTicketIssuer;
import org.springframework.stereotype.Component;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Component
public class FileTransferTicketIssuer implements ProtocolTicketIssuer {
    @Override
    public Set<String> requestProtocols() {
        return Set.of("filetransfer");
    }

    @Override
    public String claimType() {
        return "filetransfer";
    }

    @Override
    public void validateAsset(AssetEntity asset) {
        if (!AssetAccess.allows(asset, "filetransfer")) {
            throw new IllegalArgumentException("protocol 'filetransfer' not allowed for this asset OS");
        }
    }

    @Override
    public Map<String, Object> buildClaims(IssueContext ctx, UUID sessionId) {
        Normalized n = normalize(ctx);
        Map<String, Object> claims = ProtocolTicketIssuer.baseClaims(
                claimType(), sessionId, ctx.asset(), ctx.userId(), ctx.username());
        claims.put("transferId", n.transferId());
        claims.put("direction", n.direction());
        claims.put("path", n.path());
        claims.put("size", n.size());
        claims.put("fingerprint", n.fingerprint());
        claims.put("abort", n.abort());
        return claims;
    }

    @Override
    public String wsPath() {
        return "/ws/file-transfer";
    }

    @Override
    public Map<String, Object> responseExtras(IssueContext ctx) {
        Normalized n = normalize(ctx);
        Map<String, Object> extra = new LinkedHashMap<>();
        extra.put("transferId", n.transferId());
        extra.put("direction", n.direction());
        extra.put("path", n.path());
        extra.put("size", n.size());
        extra.put("fingerprint", n.fingerprint());
        extra.put("abort", n.abort());
        return extra;
    }

    /**
     * Caller must pre-fill a UUID {@code transferId} when blank so claims and extras stay aligned.
     */
    private static Normalized normalize(IssueContext ctx) {
        boolean isAbort = Boolean.TRUE.equals(ctx.abort());
        String dir = ctx.direction() == null ? "" : ctx.direction().trim().toLowerCase();
        if (!isAbort && !"upload".equals(dir) && !"download".equals(dir)) {
            throw new IllegalArgumentException("direction must be upload or download");
        }
        if (isAbort) {
            dir = dir.isBlank() ? "upload" : dir;
        }
        if (ctx.path() == null || ctx.path().isBlank()) {
            throw new IllegalArgumentException("path required");
        }
        String tid = ctx.transferId() == null || ctx.transferId().isBlank()
                ? UUID.randomUUID().toString()
                : ctx.transferId().trim();
        try {
            UUID.fromString(tid);
        } catch (IllegalArgumentException e) {
            throw new IllegalArgumentException("transferId must be a UUID");
        }
        long sz = ctx.size() == null ? 0L : ctx.size();
        if ("upload".equals(dir) && !isAbort && sz < 0) {
            throw new IllegalArgumentException("size must be >= 0");
        }
        String fp = ctx.fingerprint() == null ? "" : ctx.fingerprint();
        return new Normalized(tid, dir, ctx.path(), sz, fp, isAbort);
    }

    private record Normalized(
            String transferId, String direction, String path, long size, String fingerprint, boolean abort) {}
}
