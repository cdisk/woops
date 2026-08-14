package com.ops.control.protocol;

import com.ops.control.asset.AssetAccess;
import com.ops.control.asset.AssetEntity;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

/** Per-protocol session ticket claims / response extras. */
public interface ProtocolTicketIssuer {
    /** Request protocols this issuer handles (e.g. {@code shell_bash}, {@code rdp}). */
    Set<String> requestProtocols();

    /** JWT {@code type} claim (e.g. {@code shell}, {@code filetransfer}). */
    String claimType();

    void validateAsset(AssetEntity asset);

    Map<String, Object> buildClaims(IssueContext ctx, UUID sessionId);

    String wsPath();

    /** Extra top-level JSON fields merged into the HTTP ticket response (may be empty). */
    Map<String, Object> responseExtras(IssueContext ctx);

    /**
     * Value for {@code asset.accessProtocol} in the HTTP response.
     * Desktop tickets use the requested protocol; others use the asset OS badge.
     */
    default String assetSummaryProtocol(AssetEntity asset, String requestedProtocol) {
        return AssetAccess.protocolOf(asset);
    }

    static Map<String, Object> baseClaims(
            String type, UUID sessionId, AssetEntity asset, UUID userId, String username) {
        Map<String, Object> claims = new LinkedHashMap<>();
        claims.put("type", type);
        claims.put("assetId", asset.getId().toString());
        claims.put("userId", userId.toString());
        claims.put("username", username);
        claims.put("sessionId", sessionId.toString());
        return claims;
    }
}
