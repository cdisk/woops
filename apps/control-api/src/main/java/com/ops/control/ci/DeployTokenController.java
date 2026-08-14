package com.ops.control.ci;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/assets/{assetId}/deploy-tokens")
public class DeployTokenController {
    private final DeployTokenService service;
    private final AccessService access;

    public DeployTokenController(DeployTokenService service, AccessService access) {
        this.service = service;
        this.access = access;
    }

    public record CreateRequest(
            String remark,
            Boolean allowUpload,
            Boolean allowDownload,
            Boolean allowExec,
            Boolean allowForward,
            Boolean allowReverse,
            String expiresAt) {}

    @GetMapping
    public List<Map<String, Object>> list(@PathVariable UUID assetId, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return service.list(assetId, user);
    }

    @PostMapping
    public Map<String, Object> create(
            @PathVariable UUID assetId,
            @RequestBody CreateRequest req,
            Authentication auth,
            HttpServletRequest request) {
        UserEntity user = access.requireUser(auth);
        Instant expiresAt = null;
        if (req.expiresAt() != null && !req.expiresAt().isBlank()) {
            expiresAt = Instant.parse(req.expiresAt());
        }
        return service.create(
                assetId,
                req.remark(),
                Boolean.TRUE.equals(req.allowUpload()),
                Boolean.TRUE.equals(req.allowDownload()),
                Boolean.TRUE.equals(req.allowExec()),
                Boolean.TRUE.equals(req.allowForward()),
                Boolean.TRUE.equals(req.allowReverse()),
                expiresAt,
                user,
                request);
    }

    @DeleteMapping("/{tokenId}")
    public Map<String, String> revoke(
            @PathVariable UUID assetId,
            @PathVariable UUID tokenId,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return service.revoke(assetId, tokenId, user);
    }
}
