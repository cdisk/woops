package com.ops.control.portmap;

import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/port-mappings")
public class PortMappingController {
    private final PortMappingService service;
    private final AccessService access;
    private final AssetRepository assets;
    private final PortMappingRepository mappings;

    public PortMappingController(
            PortMappingService service,
            AccessService access,
            AssetRepository assets,
            PortMappingRepository mappings) {
        this.service = service;
        this.access = access;
        this.assets = assets;
        this.mappings = mappings;
    }

    public record CreateRequest(
            UUID assetId,
            String direction,
            String protocol,
            String targetHost,
            Integer targetPort,
            String listenHost,
            Integer listenPort,
            String remark) {}

    public record UpdateRequest(String remark) {}

    @PostMapping
    public Map<String, Object> create(@RequestBody CreateRequest req, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        if (req.assetId() == null) {
            throw new IllegalArgumentException("assetId required");
        }
        AssetEntity asset = assets.findById(req.assetId())
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        int port = req.targetPort() == null ? 0 : req.targetPort();
        return service.create(
                req.assetId(),
                req.direction(),
                req.protocol(),
                req.targetHost(),
                port,
                req.listenHost(),
                req.listenPort(),
                req.remark(),
                user.getId(),
                user.getUsername());
    }

    @PatchMapping("/{id}")
    public Map<String, Object> update(@PathVariable UUID id, @RequestBody UpdateRequest req, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        PortMappingEntity m = mappings.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
        AssetEntity asset = assets.findById(m.getAssetId())
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        return service.updateRemark(id, req.remark(), user.getId(), user.getUsername());
    }

    @DeleteMapping("/{id}")
    public Map<String, String> remove(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        PortMappingEntity m = mappings.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
        AssetEntity asset = assets.findById(m.getAssetId())
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        service.remove(id, user.getId(), user.getUsername());
        return Map.of("status", "removed", "id", id.toString());
    }

    @GetMapping
    public List<Map<String, Object>> list(
            @RequestParam(required = false) UUID assetId,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        if (assetId != null) {
            AssetEntity asset = assets.findById(assetId)
                    .orElseThrow(() -> new IllegalArgumentException("asset not found"));
            access.assertCanAccessAsset(user, asset);
            return service.listActive(assetId);
        }
        List<Map<String, Object>> all = service.listActive(null);
        if (access.isSuperAdmin(user)) {
            return all;
        }
        return all.stream().filter(row -> {
            Object aid = row.get("assetId");
            if (aid == null) return false;
            return assets.findById(UUID.fromString(String.valueOf(aid)))
                    .map(a -> access.canAccessAsset(user, a))
                    .orElse(false);
        }).toList();
    }

    @GetMapping("/runtime")
    public List<Map<String, Object>> runtime(Authentication auth) {
        UserEntity user = access.requireUser(auth);
        List<Map<String, Object>> all = service.runtime();
        if (access.isSuperAdmin(user)) {
            return all;
        }
        return all.stream().filter(row -> {
            Object aid = row.get("assetId");
            if (aid == null) return false;
            return assets.findById(UUID.fromString(String.valueOf(aid)))
                    .map(a -> access.canAccessAsset(user, a))
                    .orElse(false);
        }).toList();
    }

    @GetMapping("/history")
    public List<Map<String, Object>> history(
            @RequestParam(required = false) UUID mappingId,
            @RequestParam(required = false) UUID assetId,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        if (mappingId != null) {
            PortMappingEntity m = mappings.findById(mappingId)
                    .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
            AssetEntity asset = assets.findById(m.getAssetId())
                    .orElseThrow(() -> new IllegalArgumentException("asset not found"));
            access.assertCanAccessAsset(user, asset);
            return service.history(mappingId, null);
        }
        if (assetId != null) {
            AssetEntity asset = assets.findById(assetId)
                    .orElseThrow(() -> new IllegalArgumentException("asset not found"));
            access.assertCanAccessAsset(user, asset);
            return service.history(null, assetId);
        }
        List<Map<String, Object>> all = service.history(null, null);
        if (access.isSuperAdmin(user)) {
            return all;
        }
        return all.stream().filter(row -> {
            Object aid = row.get("assetId");
            if (aid == null) return false;
            return assets.findById(UUID.fromString(String.valueOf(aid)))
                    .map(a -> access.canAccessAsset(user, a))
                    .orElse(false);
        }).toList();
    }
}
