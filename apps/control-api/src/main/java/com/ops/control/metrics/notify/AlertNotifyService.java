package com.ops.control.metrics.notify;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.GroupService;
import com.ops.control.group.ServerGroupEntity;
import com.ops.control.group.ServerGroupRepository;
import com.ops.control.metrics.AlertIssueLogic;
import com.ops.control.user.UserEntity;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.util.*;

@Service
public class AlertNotifyService {
    private static final Logger log = LoggerFactory.getLogger(AlertNotifyService.class);
    public static final String ACT_NOTIFY_WEBHOOK_CREATE = "NOTIFY_WEBHOOK_CREATE";
    public static final String ACT_NOTIFY_WEBHOOK_UPDATE = "NOTIFY_WEBHOOK_UPDATE";
    public static final String ACT_NOTIFY_WEBHOOK_REMOVE = "NOTIFY_WEBHOOK_REMOVE";
    public static final String ACT_NOTIFY_WEBHOOK_TEST = "NOTIFY_WEBHOOK_TEST";

    private static final int MAX_ATTEMPTS = 8;
    private static final Duration HTTP_TIMEOUT = Duration.ofSeconds(10);

    private final AlertNotifyWebhookRepository webhooks;
    private final AlertNotifyOutboxRepository outbox;
    private final AssetRepository assets;
    private final ServerGroupRepository groups;
    private final GroupService groupService;
    private final AlertRecipientResolver recipients;
    private final ControlAuditService audit;
    private final ObjectMapper json;
    private final HttpClient http = HttpClient.newBuilder().connectTimeout(HTTP_TIMEOUT).build();

    public AlertNotifyService(
            AlertNotifyWebhookRepository webhooks,
            AlertNotifyOutboxRepository outbox,
            AssetRepository assets,
            ServerGroupRepository groups,
            GroupService groupService,
            AlertRecipientResolver recipients,
            ControlAuditService audit,
            ObjectMapper json) {
        this.webhooks = webhooks;
        this.outbox = outbox;
        this.assets = assets;
        this.groups = groups;
        this.groupService = groupService;
        this.recipients = recipients;
        this.audit = audit;
        this.json = json;
    }

    public List<Map<String, Object>> listWebhooks() {
        return webhooks.findAllByOrderByNameAsc().stream().map(this::view).toList();
    }

    @Transactional
    public Map<String, Object> createWebhook(
            UserEntity actor,
            String name,
            String url,
            String bearerToken,
            Boolean enabled,
            String scopeType,
            UUID scopeId) {
        WebhookUrlGuard.assertSafe(url);
        String scope = normalizeScope(scopeType, scopeId);
        AlertNotifyWebhookEntity w = new AlertNotifyWebhookEntity();
        w.setId(UUID.randomUUID());
        w.setName(requireName(name));
        w.setUrl(url.trim());
        w.setBearerToken(bearerToken == null ? "" : bearerToken.trim());
        w.setEnabled(enabled == null || enabled);
        w.setScopeType(scope);
        w.setScopeId(AlertNotifyWebhookEntity.SCOPE_ALL.equals(scope) ? null : scopeId);
        w.setCreatedAt(Instant.now());
        w.setUpdatedAt(Instant.now());
        AlertNotifyWebhookEntity saved = webhooks.save(w);
        audit.record(
                ControlAuditService.CAT_MONITOR,
                ACT_NOTIFY_WEBHOOK_CREATE,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(scopeDetail(saved)));
        return view(saved);
    }

    @Transactional
    public Map<String, Object> updateWebhook(
            UserEntity actor,
            UUID id,
            String name,
            String url,
            String bearerToken,
            Boolean enabled,
            String scopeType,
            UUID scopeId) {
        AlertNotifyWebhookEntity w = webhooks.findById(id).orElseThrow(() -> new IllegalArgumentException("webhook not found"));
        if (name != null && !name.isBlank()) {
            w.setName(name.trim());
        }
        if (url != null && !url.isBlank()) {
            WebhookUrlGuard.assertSafe(url);
            w.setUrl(url.trim());
        }
        if (bearerToken != null && !bearerToken.isBlank()) {
            w.setBearerToken(bearerToken.trim());
        }
        if (enabled != null) {
            w.setEnabled(enabled);
        }
        if (scopeType != null && !scopeType.isBlank()) {
            String scope = normalizeScope(scopeType, scopeId);
            w.setScopeType(scope);
            w.setScopeId(AlertNotifyWebhookEntity.SCOPE_ALL.equals(scope) ? null : scopeId);
        }
        w.setUpdatedAt(Instant.now());
        AlertNotifyWebhookEntity saved = webhooks.save(w);
        audit.record(
                ControlAuditService.CAT_MONITOR,
                ACT_NOTIFY_WEBHOOK_UPDATE,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(scopeDetail(saved)));
        return view(saved);
    }

    @Transactional
    public void deleteWebhook(UserEntity actor, UUID id) {
        AlertNotifyWebhookEntity w = webhooks.findById(id).orElseThrow(() -> new IllegalArgumentException("webhook not found"));
        webhooks.delete(w);
        audit.record(
                ControlAuditService.CAT_MONITOR,
                ACT_NOTIFY_WEBHOOK_REMOVE,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(scopeDetail(w)));
    }

    @Transactional
    public Map<String, Object> testWebhook(UserEntity actor, UUID id) {
        AlertNotifyWebhookEntity w = webhooks.findById(id).orElseThrow(() -> new IllegalArgumentException("webhook not found"));
        // Full sample shaped like a real firing payload so bridges can validate all fields.
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("event", "test");
        payload.put("assetId", "3fa85f64-5717-4562-b3fc-2c963f66afa6");
        payload.put("assetName", "web-01");
        payload.put("groupName", "prod");
        payload.put("privateIp", "10.0.0.5");
        payload.put("itemId", "disk.used_percent");
        payload.put("instance", "/");
        payload.put("kind", "metric");
        payload.put("label", "Disk high (/)=99");
        payload.put("time", Instant.now().toString());
        payload.put("incidentId", "7c9e6679-7425-40de-944b-e07fc1f90ae7");
        Map<String, Object> self = new LinkedHashMap<>();
        self.put("userId", actor.getId().toString());
        self.put("username", actor.getUsername());
        self.put("nickname", actor.getNickname() == null ? "" : actor.getNickname());
        self.put("authSource", actor.getAuthSource() == null ? "local" : actor.getAuthSource());
        payload.put("recipients", List.of(self));
        deliverNow(w, payload);
        audit.record(
                ControlAuditService.CAT_MONITOR,
                ACT_NOTIFY_WEBHOOK_TEST,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of("webhookId", id.toString(), "name", w.getName())));
        return Map.of("status", "ok");
    }

    /**
     * Enqueue webhook deliveries for an incident transition.
     * Skips when the item is globally ignored.
     */
    @Transactional
    public void enqueueIssueEvent(
            AssetEntity asset,
            AlertIssueLogic.Issue issue,
            String event,
            Instant time,
            Set<String> ignoredItemIds,
            UUID incidentId) {
        enqueueIssueEvent(asset, issue, event, time, ignoredItemIds, incidentId, false);
    }

    @Transactional
    public void enqueueIssueEvent(
            AssetEntity asset,
            AlertIssueLogic.Issue issue,
            String event,
            Instant time,
            Set<String> ignoredItemIds,
            UUID incidentId,
            boolean force) {
        if (asset == null || issue == null || event == null) {
            return;
        }
        if (!force && ignoredItemIds != null && ignoredItemIds.contains(issue.itemId())) {
            return;
        }
        List<AlertRecipientResolver.Recipient> recips = recipients.resolve(asset);
        Map<UUID, String> groupNames = groupService.nameById();
        String groupName = asset.getGroupId() == null ? "" : groupNames.getOrDefault(asset.getGroupId(), "");
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("event", event);
        payload.put("assetId", asset.getId().toString());
        payload.put("assetName", asset.getDisplayName() != null ? asset.getDisplayName() : asset.getId().toString());
        payload.put("groupName", groupName);
        payload.put("privateIp", asset.getPrivateIp() == null ? "" : asset.getPrivateIp());
        payload.put("itemId", issue.itemId());
        payload.put("instance", issue.instance() == null ? "" : issue.instance());
        payload.put("kind", issue.kind());
        payload.put("label", issue.label() == null ? "" : issue.label());
        payload.put("time", time.toString());
        if (incidentId != null) {
            payload.put("incidentId", incidentId.toString());
        }
        List<Map<String, Object>> recipientViews = new ArrayList<>();
        for (AlertRecipientResolver.Recipient r : recips) {
            Map<String, Object> m = new LinkedHashMap<>();
            m.put("userId", r.userId().toString());
            m.put("username", r.username());
            m.put("nickname", r.nickname());
            m.put("authSource", r.authSource());
            recipientViews.add(m);
        }
        payload.put("recipients", recipientViews);

        String payloadJson;
        try {
            payloadJson = json.writeValueAsString(payload);
        } catch (Exception e) {
            log.warn("serialize alert payload failed: {}", e.toString());
            return;
        }

        Instant now = Instant.now();
        for (AlertNotifyWebhookEntity w : webhooks.findByEnabledTrue()) {
            if (!scopeMatches(w, asset)) {
                continue;
            }
            String dedupe = w.getId() + "|" + event + "|" + (incidentId == null ? "" : incidentId) + "|" + issue.itemId()
                    + "|" + (issue.instance() == null ? "" : issue.instance()) + "|"
                    + (issue.ruleId() == null ? "-" : issue.ruleId());
            if (force) {
                dedupe = dedupe + "|force|" + time.toEpochMilli();
            }
            if (outbox.existsByDedupeKey(dedupe)) {
                continue;
            }
            AlertNotifyOutboxEntity row = new AlertNotifyOutboxEntity();
            row.setId(UUID.randomUUID());
            row.setWebhookId(w.getId());
            row.setAssetId(asset.getId());
            row.setDedupeKey(dedupe);
            row.setPayloadJson(payloadJson);
            row.setStatus(AlertNotifyOutboxEntity.STATUS_PENDING);
            row.setAttempts(0);
            row.setNextAttemptAt(now);
            row.setCreatedAt(now);
            row.setUpdatedAt(now);
            outbox.save(row);
        }
    }

    @Transactional
    public int processDue(int limit) {
        Instant now = Instant.now();
        List<AlertNotifyOutboxEntity> due = outbox.findDue(now);
        int n = 0;
        for (AlertNotifyOutboxEntity row : due) {
            if (n >= limit) {
                break;
            }
            processOne(row, now);
            n++;
        }
        return n;
    }

    private void processOne(AlertNotifyOutboxEntity row, Instant now) {
        Optional<AlertNotifyWebhookEntity> wh = webhooks.findById(row.getWebhookId());
        if (wh.isEmpty() || !wh.get().isEnabled()) {
            row.setStatus(AlertNotifyOutboxEntity.STATUS_CANCELLED);
            row.setUpdatedAt(now);
            outbox.save(row);
            return;
        }
        AlertNotifyWebhookEntity w = wh.get();
        if (row.getAssetId() != null) {
            AssetEntity asset = assets.findById(row.getAssetId()).orElse(null);
            if (asset == null || !scopeMatches(w, asset)) {
                row.setStatus(AlertNotifyOutboxEntity.STATUS_CANCELLED);
                row.setUpdatedAt(now);
                outbox.save(row);
                return;
            }
        }
        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> payload = json.readValue(row.getPayloadJson(), Map.class);
            deliverNow(w, payload);
            row.setStatus(AlertNotifyOutboxEntity.STATUS_SENT);
            row.setLastError("");
            row.setUpdatedAt(now);
            outbox.save(row);
        } catch (Exception e) {
            int attempts = row.getAttempts() + 1;
            row.setAttempts(attempts);
            row.setLastError(trimErr(e.getMessage()));
            row.setUpdatedAt(now);
            if (attempts >= MAX_ATTEMPTS) {
                row.setStatus(AlertNotifyOutboxEntity.STATUS_DEAD);
            } else {
                row.setStatus(AlertNotifyOutboxEntity.STATUS_FAILED);
                long delaySec = Math.min(3600, (long) Math.pow(2, Math.min(attempts, 10)));
                row.setNextAttemptAt(now.plusSeconds(delaySec));
            }
            outbox.save(row);
            log.warn("alert webhook delivery failed id={} attempts={}: {}", row.getId(), attempts, e.toString());
        }
    }

    private void deliverNow(AlertNotifyWebhookEntity w, Map<String, Object> payload) {
        WebhookUrlGuard.assertSafe(w.getUrl());
        String body;
        try {
            body = json.writeValueAsString(payload);
        } catch (Exception e) {
            throw new IllegalStateException("payload serialize failed", e);
        }
        HttpRequest.Builder b = HttpRequest.newBuilder()
                .uri(URI.create(w.getUrl()))
                .timeout(HTTP_TIMEOUT)
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(body));
        if (w.hasToken()) {
            b.header("Authorization", "Bearer " + w.getBearerToken());
        }
        HttpResponse<String> resp;
        try {
            resp = http.send(b.build(), HttpResponse.BodyHandlers.ofString());
        } catch (Exception e) {
            throw new IllegalStateException(e.getMessage() == null ? e.toString() : e.getMessage(), e);
        }
        int code = resp.statusCode();
        if (code < 200 || code >= 300) {
            throw new IllegalStateException("HTTP " + code);
        }
    }

    boolean scopeMatches(AlertNotifyWebhookEntity w, AssetEntity asset) {
        if (w == null || asset == null) {
            return false;
        }
        String type = w.getScopeType() == null ? AlertNotifyWebhookEntity.SCOPE_ALL : w.getScopeType();
        if (AlertNotifyWebhookEntity.SCOPE_ALL.equals(type)) {
            return true;
        }
        if (AlertNotifyWebhookEntity.SCOPE_ASSET.equals(type)) {
            return asset.getId().equals(w.getScopeId());
        }
        if (AlertNotifyWebhookEntity.SCOPE_GROUP.equals(type)) {
            if (w.getScopeId() == null || asset.getGroupId() == null) {
                return false;
            }
            return groupDescendants(w.getScopeId()).contains(asset.getGroupId());
        }
        return false;
    }

    private Set<UUID> groupDescendants(UUID root) {
        Map<UUID, List<UUID>> children = new HashMap<>();
        for (ServerGroupEntity g : groups.findAll()) {
            children.computeIfAbsent(g.getParentId(), k -> new ArrayList<>()).add(g.getId());
        }
        Set<UUID> out = new HashSet<>();
        Deque<UUID> q = new ArrayDeque<>();
        q.add(root);
        while (!q.isEmpty()) {
            UUID id = q.poll();
            if (!out.add(id)) continue;
            for (UUID c : children.getOrDefault(id, List.of())) {
                q.add(c);
            }
        }
        return out;
    }

    private String normalizeScope(String scopeType, UUID scopeId) {
        String t = scopeType == null || scopeType.isBlank()
                ? AlertNotifyWebhookEntity.SCOPE_ALL
                : scopeType.trim().toUpperCase(Locale.ROOT);
        return switch (t) {
            case AlertNotifyWebhookEntity.SCOPE_ALL -> AlertNotifyWebhookEntity.SCOPE_ALL;
            case AlertNotifyWebhookEntity.SCOPE_GROUP -> {
                if (scopeId == null) {
                    throw new IllegalArgumentException("scopeId required for GROUP");
                }
                groups.findById(scopeId).orElseThrow(() -> new IllegalArgumentException("group not found"));
                yield AlertNotifyWebhookEntity.SCOPE_GROUP;
            }
            case AlertNotifyWebhookEntity.SCOPE_ASSET -> {
                if (scopeId == null) {
                    throw new IllegalArgumentException("scopeId required for ASSET");
                }
                assets.findById(scopeId).orElseThrow(() -> new IllegalArgumentException("asset not found"));
                yield AlertNotifyWebhookEntity.SCOPE_ASSET;
            }
            default -> throw new IllegalArgumentException("invalid scopeType");
        };
    }

    private Map<String, Object> view(AlertNotifyWebhookEntity w) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", w.getId().toString());
        m.put("name", w.getName());
        m.put("url", w.getUrl());
        m.put("hasToken", w.hasToken());
        m.put("enabled", w.isEnabled());
        m.put("scopeType", w.getScopeType());
        m.put("scopeId", w.getScopeId() == null ? null : w.getScopeId().toString());
        m.put("createdAt", w.getCreatedAt().toString());
        m.put("updatedAt", w.getUpdatedAt().toString());
        return m;
    }

    private static Map<String, Object> scopeDetail(AlertNotifyWebhookEntity w) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("webhookId", w.getId().toString());
        m.put("name", w.getName());
        m.put("scopeType", w.getScopeType());
        if (w.getScopeId() != null) {
            m.put("scopeId", w.getScopeId().toString());
        }
        m.put("enabled", w.isEnabled());
        return m;
    }

    private static String requireName(String name) {
        if (name == null || name.isBlank()) {
            throw new IllegalArgumentException("name required");
        }
        return name.trim();
    }

    private static String trimErr(String s) {
        if (s == null) return "";
        return s.length() > 1000 ? s.substring(0, 1000) : s;
    }
}
