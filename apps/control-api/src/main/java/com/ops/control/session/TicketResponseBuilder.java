package com.ops.control.session;

import com.ops.control.asset.AssetEntity;
import com.ops.control.common.OpsProperties;
import com.ops.control.group.GroupService;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

/** Builds the HTTP ticket envelope returned by session / CI ticket APIs. */
@Component
public class TicketResponseBuilder {
    private final OpsProperties props;
    private final GroupService groups;

    public TicketResponseBuilder(OpsProperties props, GroupService groups) {
        this.props = props;
        this.groups = groups;
    }

    public Map<String, Object> build(
            UUID sessionId,
            String protocol,
            String ticket,
            Instant expiresAt,
            String wsPath,
            AssetEntity asset,
            String assetSummaryProtocol,
            Map<String, Object> extras) {
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("sessionId", sessionId.toString());
        out.put("protocol", protocol);
        if (extras != null) {
            out.putAll(extras);
        }
        out.put("ticket", ticket);
        out.put("expiresAt", expiresAt.toString());
        out.put("browserWs", props.gatewayPublicWs() + wsPath + "?ticket=" + ticket);
        out.put("implemented", true);
        out.put("asset", assetSummary(asset, assetSummaryProtocol));
        return out;
    }

    private Map<String, Object> assetSummary(AssetEntity asset, String accessProtocol) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", asset.getId().toString());
        m.put("displayName", asset.getDisplayName());
        m.put("hostname", asset.getHostname() == null ? "" : asset.getHostname());
        m.put("accessProtocol", accessProtocol);
        m.put("os", asset.getOs() == null ? "" : asset.getOs());
        m.put("publicIp", asset.getPublicIp() == null ? "" : asset.getPublicIp());
        m.put("privateIp", asset.getPrivateIp() == null ? "" : asset.getPrivateIp());
        m.put("groupId", asset.getGroupId() == null ? null : asset.getGroupId().toString());
        String groupName = "";
        if (asset.getGroupId() != null) {
            groupName = groups.find(asset.getGroupId()).map(g -> g.getName() == null ? "" : g.getName()).orElse("");
        }
        m.put("groupName", groupName);
        return m;
    }
}
