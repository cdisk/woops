package com.ops.control.metrics;

import com.ops.control.access.AccessService;
import com.ops.control.apitoken.ApiTokenAuth;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.*;
import java.util.stream.Collectors;

@RestController
@RequestMapping("/api")
public class MetricsController {
    private final MetricsService metrics;
    private final AccessService access;
    private final AssetRepository assets;
    private final ControlAuditService audit;

    public MetricsController(
            MetricsService metrics,
            AccessService access,
            AssetRepository assets,
            ControlAuditService audit) {
        this.metrics = metrics;
        this.access = access;
        this.assets = assets;
        this.audit = audit;
    }

    @GetMapping("/dashboard/summary")
    public Map<String, Object> dashboard(Authentication auth) {
        UserEntity user = access.requireUser(auth);
        Map<String, Object> full = metrics.dashboardSummary();
        if (access.isSuperAdmin(user)) {
            return full;
        }
        Set<UUID> assetIds = assets.findAll().stream()
                .filter(a -> access.canAccessAsset(user, a))
                .map(AssetEntity::getId)
                .collect(Collectors.toSet());
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> firing = (List<Map<String, Object>>) full.getOrDefault("abnormalAssets", List.of());
        List<Map<String, Object>> filteredFiring = firing.stream()
                .filter(m -> {
                    Object id = m.get("assetId");
                    return id != null && assetIds.contains(UUID.fromString(String.valueOf(id)));
                })
                .toList();
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> ignored = (List<Map<String, Object>>) full.getOrDefault("ignoredAlerts", List.of());
        List<Map<String, Object>> filteredIgnored = ignored.stream()
                .filter(m -> {
                    Object id = m.get("assetId");
                    return id != null && assetIds.contains(UUID.fromString(String.valueOf(id)));
                })
                .toList();
        long online = assets.findAll().stream()
                .filter(a -> assetIds.contains(a.getId()) && a.isOnline())
                .count();
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("assetTotal", assetIds.size());
        out.put("assetOnline", online);
        out.put("assetAbnormal", filteredFiring.size());
        out.put("abnormalAssets", filteredFiring);
        out.put("ignoredAlerts", filteredIgnored);
        return out;
    }

    public record IgnoreBody(String itemId) {}

    @PostMapping("/assets/{id}/alert-ignores")
    public Map<String, Object> ignoreAlert(
            @PathVariable UUID id,
            @RequestBody IgnoreBody body,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        String itemId = body == null ? null : body.itemId();
        return metrics.ignoreAlert(user, id, itemId);
    }

    @DeleteMapping("/assets/{id}/alert-ignores")
    public Map<String, String> unignoreAlert(
            @PathVariable UUID id,
            @RequestParam String itemId,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        metrics.unignoreAlert(user, id, itemId);
        return Map.of("status", "deleted");
    }

    @GetMapping("/monitor/items")
    public List<Map<String, Object>> items(Authentication auth) {
        access.requireUser(auth);
        return metrics.listItemDefs();
    }

    @GetMapping("/assets/{id}/metrics/latest")
    public List<Map<String, Object>> latest(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        return metrics.latest(id);
    }

    @GetMapping("/assets/{id}/metrics/series")
    public Map<String, Object> series(
            @PathVariable UUID id,
            @RequestParam(required = false) String keys,
            @RequestParam(required = false) String from,
            @RequestParam(required = false) String to,
            @RequestParam(required = false, defaultValue = "minute") String grain,
            @RequestParam(required = false, defaultValue = "") String instance,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        Instant toTs = parseOr(to, Instant.now());
        Instant fromTs = parseOr(from, toTs.minus(24, ChronoUnit.HOURS));
        List<String> itemIds = null;
        if (keys != null && !keys.isBlank()) {
            itemIds = Arrays.stream(keys.split(",")).map(String::trim).filter(s -> !s.isEmpty()).toList();
        }
        return metrics.series(id, itemIds, fromTs, toTs, grain, instance);
    }

    public record ReportMetricsBody(
            UUID assetId,
            List<String> keys,
            String from,
            String to,
            String grain,
            String instance) {}

    @PostMapping("/reports/metrics")
    public Map<String, Object> reportMetrics(@RequestBody ReportMetricsBody body, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        if (body == null || body.assetId() == null) {
            throw new IllegalArgumentException("assetId required");
        }
        AssetEntity asset = assets.findById(body.assetId())
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        Instant toTs = parseOr(body.to(), Instant.now());
        Instant fromTs = parseOr(body.from(), toTs.minus(7, ChronoUnit.DAYS));
        List<String> itemIds = body.keys() == null ? List.of() : body.keys().stream()
                .filter(Objects::nonNull)
                .map(String::trim)
                .filter(s -> !s.isEmpty())
                .toList();
        String grain = body.grain() == null || body.grain().isBlank() ? "hour" : body.grain();
        String instance = body.instance() == null ? "" : body.instance();
        Map<String, Object> out = metrics.reportMetrics(
                asset.getId(),
                asset.getDisplayName(),
                itemIds,
                fromTs,
                toTs,
                grain,
                instance);

        Map<String, Object> detail = new LinkedHashMap<>();
        if (auth.getDetails() instanceof ApiTokenAuth tokenAuth) {
            detail.put("tokenId", tokenAuth.tokenId().toString());
            detail.put("tokenName", tokenAuth.name());
            detail.put("scopes", tokenAuth.scopes());
        }
        detail.put("assetId", asset.getId().toString());
        detail.put("from", fromTs.toString());
        detail.put("to", toTs.toString());
        detail.put("keys", itemIds);
        detail.put("grain", out.get("grain"));
        audit.record(
                ControlAuditService.CAT_MONITOR,
                ControlAuditService.ACT_REPORT_READ,
                user.getId(),
                user.getUsername(),
                asset.getId(),
                null,
                ControlAuditService.jsonDetail(detail));
        return out;
    }

    @GetMapping("/monitor/alert-rules")
    public List<Map<String, Object>> listRules(Authentication auth) {
        access.assertCanManageAlertRules(access.requireUser(auth));
        return metrics.listRules();
    }

    public record RuleBody(String name, String itemId, String instance, String op, Double threshold, Boolean enabled) {}

    @PostMapping("/monitor/alert-rules")
    public Map<String, Object> createRule(@RequestBody RuleBody body, Authentication auth) {
        access.assertCanManageAlertRules(access.requireUser(auth));
        if (body.name() == null || body.name().isBlank() || body.itemId() == null || body.itemId().isBlank() || body.threshold() == null) {
            throw new IllegalArgumentException("name, itemId, threshold required");
        }
        return metrics.createRule(body.name(), body.itemId(), body.instance(), body.op(), body.threshold(), body.enabled());
    }

    @PatchMapping("/monitor/alert-rules/{id}")
    public Map<String, Object> updateRule(@PathVariable UUID id, @RequestBody RuleBody body, Authentication auth) {
        access.assertCanManageAlertRules(access.requireUser(auth));
        return metrics.updateRule(id, body.name(), body.itemId(), body.instance(), body.op(), body.threshold(), body.enabled());
    }

    @DeleteMapping("/monitor/alert-rules/{id}")
    public Map<String, String> deleteRule(@PathVariable UUID id, Authentication auth) {
        access.assertCanManageAlertRules(access.requireUser(auth));
        metrics.deleteRule(id);
        return Map.of("status", "deleted");
    }

    private static Instant parseOr(String s, Instant fallback) {
        if (s == null || s.isBlank()) return fallback;
        try {
            return Instant.parse(s);
        } catch (Exception e) {
            return fallback;
        }
    }
}
