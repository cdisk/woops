package com.ops.control.protocol;

import com.ops.control.asset.AssetEntity;

import java.util.UUID;

/** Inputs for minting a protocol session ticket. */
public record IssueContext(
        UUID userId,
        String username,
        AssetEntity asset,
        String requestedProtocol,
        String direction,
        String path,
        Long size,
        String fingerprint,
        String transferId,
        Boolean abort) {

    public static IssueContext basic(UUID userId, String username, AssetEntity asset, String requestedProtocol) {
        return new IssueContext(userId, username, asset, requestedProtocol, null, null, null, null, null, null);
    }
}
