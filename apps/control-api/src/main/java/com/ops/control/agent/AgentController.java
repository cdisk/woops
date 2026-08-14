package com.ops.control.agent;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.validation.constraints.NotBlank;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api")
public class AgentController {
    private final AgentService agentService;
    private final AccessService access;

    public AgentController(AgentService agentService, AccessService access) {
        this.agentService = agentService;
        this.access = access;
    }

    public record CreateInstallCodeRequest(UUID groupId) {}

    @PostMapping("/install-codes")
    public Map<String, Object> create(
            @RequestBody CreateInstallCodeRequest body,
            Authentication auth,
            HttpServletRequest request) {
        UserEntity user = access.requireUser(auth);
        access.assertCanManageInventory(user);
        if (body == null || body.groupId() == null) {
            throw new IllegalArgumentException("groupId required");
        }
        access.assertCanManageGroup(user, body.groupId());
        return agentService.createInstallCode(user.getId(), body.groupId(), request);
    }

    @GetMapping("/install-codes")
    public List<Map<String, Object>> list(Authentication auth) {
        UserEntity user = access.requireUser(auth);
        access.assertCanManageInventory(user);
        return agentService.listInstallCodes();
    }

    @PostMapping("/install-codes/{id}/revoke")
    public Map<String, String> revoke(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        access.assertCanManageInventory(user);
        agentService.revokeInstallCode(id, user.getId(), user.getUsername());
        return Map.of("status", "revoked");
    }

    @GetMapping("/install-codes/{code}/valid")
    public Map<String, Object> valid(@PathVariable String code) {
        return Map.of("valid", agentService.isInstallCodeValid(code));
    }

    public record RegisterBody(
            @NotBlank String installCode,
            String assetId,
            String agentVersion,
            String hostname,
            String os,
            String arch,
            String publicIp,
            String privateIp
    ) {}

    @PostMapping("/agent/register")
    public Map<String, Object> register(@RequestBody RegisterBody body, HttpServletRequest request) {
        return agentService.register(
                new AgentService.RegisterRequest(
                        body.installCode(), body.assetId(), body.agentVersion(),
                        body.hostname(), body.os(), body.arch(),
                        body.publicIp(), body.privateIp()),
                clientIp(request)
        );
    }

    public record OnlineBody(
            @NotBlank String assetId,
            @NotBlank String agentToken,
            boolean online,
            String sourceIp,
            String reason,
            String connectionId,
            String gatewayInstance
    ) {}

    @PostMapping("/internal/agent/online")
    public Map<String, String> online(@RequestBody OnlineBody body) {
        var asset = agentService.authenticateAgent(body.assetId(), body.agentToken())
                .orElseThrow(() -> new IllegalArgumentException("invalid agent credentials"));
        agentService.markOnline(
                asset.getId(),
                body.online(),
                new AgentService.PresenceMeta(
                        body.sourceIp(),
                        body.reason(),
                        body.connectionId(),
                        body.gatewayInstance()));
        return Map.of("status", "ok");
    }

    public record NetInfoBody(
            @NotBlank String assetId,
            @NotBlank String agentToken,
            String privateIp
    ) {}

    @PostMapping("/internal/agent/netinfo")
    public Map<String, String> netinfo(@RequestBody NetInfoBody body) {
        var asset = agentService.authenticateAgent(body.assetId(), body.agentToken())
                .orElseThrow(() -> new IllegalArgumentException("invalid agent credentials"));
        agentService.updatePrivateIp(asset.getId(), body.privateIp());
        return Map.of("status", "ok");
    }

    public record AuthBody(@NotBlank String assetId, @NotBlank String agentToken) {}

    @PostMapping("/internal/agent/auth")
    public Map<String, Object> auth(@RequestBody AuthBody body) {
        var asset = agentService.authenticateAgent(body.assetId(), body.agentToken())
                .orElseThrow(() -> new IllegalArgumentException("invalid agent credentials"));
        return Map.of(
                "assetId", asset.getId().toString(),
                "displayName", asset.getDisplayName(),
                "hostname", asset.getHostname() == null ? "" : asset.getHostname()
        );
    }

    private static String clientIp(HttpServletRequest request) {
        String forwarded = request.getHeader("X-Forwarded-For");
        if (forwarded != null && !forwarded.isBlank()) {
            return forwarded.split(",")[0].trim();
        }
        return request.getRemoteAddr();
    }
}
