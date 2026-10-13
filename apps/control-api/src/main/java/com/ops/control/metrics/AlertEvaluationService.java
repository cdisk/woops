package com.ops.control.metrics;

import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.PageSupport;
import com.ops.control.group.GroupService;
import com.ops.control.metrics.notify.AlertNotifyService;
import com.ops.control.user.UserEntity;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.*;

@Service
public class AlertEvaluationService {
    private final AssetRepository assets;
    private final MonitorLatestRepository latestRepo;
    private final MetricAlertRuleRepository rules;
    private final AssetAlertStatusRepository alertStatus;
    private final AssetAlertIgnoreRepository alertIgnores;
    private final AlertIncidentRepository incidents;
    private final AlertPendingRepository pendings;
    private final AlertNotifyService notify;
    private final GroupService groups;

    public AlertEvaluationService(
            AssetRepository assets,
            MonitorLatestRepository latestRepo,
            MetricAlertRuleRepository rules,
            AssetAlertStatusRepository alertStatus,
            AssetAlertIgnoreRepository alertIgnores,
            AlertIncidentRepository incidents,
            AlertPendingRepository pendings,
            AlertNotifyService notify,
            GroupService groups) {
        this.assets = assets;
        this.latestRepo = latestRepo;
        this.rules = rules;
        this.alertStatus = alertStatus;
        this.alertIgnores = alertIgnores;
        this.incidents = incidents;
        this.pendings = pendings;
        this.notify = notify;
        this.groups = groups;
    }

    public record RecomputeOptions(Set<String> goneLatestKeys, boolean forceNotifyOpenIgnoredItem) {
        public static RecomputeOptions none() {
            return new RecomputeOptions(Set.of(), false);
        }

        public static RecomputeOptions gone(Set<String> keys) {
            return new RecomputeOptions(keys == null ? Set.of() : keys, false);
        }
    }

    @Transactional
    public void recompute(UUID assetId) {
        recompute(assetId, RecomputeOptions.none());
    }

    @Transactional
    public void recompute(UUID assetId, RecomputeOptions options) {
        AssetEntity asset = assets.findById(assetId).orElse(null);
        if (asset == null) {
            return;
        }
        RecomputeOptions opts = options == null ? RecomputeOptions.none() : options;
        Instant now = Instant.now();

        Map<UUID, MetricAlertRuleEntity> ruleById = new HashMap<>();
        List<AlertIssueLogic.Rule> enabledRules = new ArrayList<>();
        for (MetricAlertRuleEntity r : rules.findByEnabledTrue()) {
            ruleById.put(r.getId(), r);
            enabledRules.add(new AlertIssueLogic.Rule(
                    r.getId(),
                    r.getName(),
                    r.getItemId(),
                    r.getInstance(),
                    r.getOp(),
                    r.getThreshold(),
                    r.getFireMinutes(),
                    r.getRecoverMinutes()));
        }
        List<AlertIssueLogic.Point> points = latestRepo.findByAssetIdOrderByItemIdAscInstanceAsc(assetId).stream()
                .map(row -> new AlertIssueLogic.Point(row.getItemId(), row.getInstance(), row.getValue()))
                .toList();
        List<AlertIssueLogic.Issue> metricHits = AlertIssueLogic.metricHits(points, enabledRules);
        List<AlertIssueLogic.Issue> actual = AlertIssueLogic.actualIssues(asset.isOnline(), metricHits);

        Set<String> ignored = ignoredItemIds(assetId);

        Map<String, AlertIncidentEntity> openByKey = new HashMap<>();
        for (AlertIncidentEntity inc : incidents.findByAssetIdAndResolvedAtIsNull(assetId)) {
            openByKey.put(AlertIssueLogic.issueKey(inc.getItemId(), inc.getInstance(), inc.getRuleId()), inc);
        }

        Map<String, AlertPendingEntity> pendingByKey = new HashMap<>();
        for (AlertPendingEntity p : pendings.findByAssetId(assetId)) {
            pendingByKey.put(AlertIssueLogic.issueKey(p.getItemId(), p.getInstance(), p.getRuleId()), p);
        }

        Set<String> seen = new HashSet<>();
        for (AlertIssueLogic.Issue issue : actual) {
            String key = AlertIssueLogic.issueKey(issue);
            seen.add(key);
            AlertIncidentEntity existing = openByKey.get(key);
            boolean offlineIssue = AlertIssueLogic.KIND_OFFLINE.equals(issue.kind());
            int fireMinutes = offlineIssue ? 0 : fireMinutesOf(issue.ruleId(), ruleById);

            if (existing != null) {
                // Rematch while open: cancel recover wait and refresh.
                clearPending(pendingByKey, assetId, issue);
                existing.setLastSeenAt(now);
                existing.setLatestValue(issue.value());
                existing.setLabel(issue.label());
                incidents.save(existing);
                continue;
            }

            if (fireMinutes <= 0) {
                clearPending(pendingByKey, assetId, issue);
                AlertIncidentEntity created = openIncident(assetId, issue, now);
                openByKey.put(key, created);
                notify.enqueueIssueEvent(asset, issue, "firing", now, ignored, created.getId());
                continue;
            }

            AlertPendingEntity pending = pendingByKey.get(key);
            if (pending == null || !AlertPendingEntity.PHASE_FIRE.equals(pending.getPhase())) {
                upsertPending(pendingByKey, assetId, issue, AlertPendingEntity.PHASE_FIRE, now);
                continue;
            }
            if (AlertDurationLogic.heldLongEnough(pending.getSinceAt(), now, fireMinutes)) {
                clearPending(pendingByKey, assetId, issue);
                AlertIncidentEntity created = openIncident(assetId, issue, now);
                openByKey.put(key, created);
                notify.enqueueIssueEvent(asset, issue, "firing", now, ignored, created.getId());
            } else {
                upsertPending(pendingByKey, assetId, issue, AlertPendingEntity.PHASE_FIRE, now);
            }
        }

        boolean wentOffline = !asset.isOnline();
        for (Map.Entry<String, AlertIncidentEntity> e : openByKey.entrySet()) {
            if (seen.contains(e.getKey())) {
                continue;
            }
            AlertIncidentEntity inc = e.getValue();
            String reason = resolveReason(inc, opts.goneLatestKeys(), wentOffline);
            AlertIssueLogic.Issue issue = toIssue(inc);
            boolean immediate = AlertDurationLogic.isImmediateCloseReason(inc.getKind(), reason);
            int recoverMinutes = immediate ? 0 : recoverMinutesOf(inc.getRuleId(), ruleById);

            if (recoverMinutes <= 0) {
                clearPending(pendingByKey, assetId, issue);
                closeIncident(inc, now, reason);
                notify.enqueueIssueEvent(asset, issue, "resolved", now, ignored, inc.getId());
                continue;
            }

            AlertPendingEntity pending = pendingByKey.get(e.getKey());
            Instant since;
            if (pending == null || !AlertPendingEntity.PHASE_RECOVER.equals(pending.getPhase())) {
                since = now;
                upsertPending(pendingByKey, assetId, issue, AlertPendingEntity.PHASE_RECOVER, now);
            } else {
                since = pending.getSinceAt();
                upsertPending(pendingByKey, assetId, issue, AlertPendingEntity.PHASE_RECOVER, now);
            }
            if (AlertDurationLogic.heldLongEnough(since, now, recoverMinutes)) {
                clearPending(pendingByKey, assetId, issue);
                closeIncident(inc, now, reason);
                notify.enqueueIssueEvent(asset, issue, "resolved", now, ignored, inc.getId());
            }
            // else keep open incident; recover pending tracks wait
        }

        // Drop FIRE pendings for keys no longer matching (cleared before fire matured).
        for (AlertPendingEntity p : List.copyOf(pendingByKey.values())) {
            if (!AlertPendingEntity.PHASE_FIRE.equals(p.getPhase())) {
                continue;
            }
            String key = AlertIssueLogic.issueKey(p.getItemId(), p.getInstance(), p.getRuleId());
            if (!seen.contains(key)) {
                pendings.delete(p);
                pendingByKey.remove(key);
            }
        }

        List<AlertIssueLogic.Issue> statusIssues = statusIssuesFromOpen(asset, openByKey.values(), ignored);
        writeAssetStatus(assetId, statusIssues, now);
    }

    /** After unignore: if open incidents remain for the item, re-notify firing. */
    @Transactional
    public void notifyOpenAfterUnignore(UUID assetId, String itemId) {
        AssetEntity asset = assets.findById(assetId).orElse(null);
        if (asset == null) {
            return;
        }
        String id = itemId == null ? "" : itemId.trim();
        Set<String> ignored = ignoredItemIds(assetId);
        Instant now = Instant.now();
        for (AlertIncidentEntity inc : incidents.findOpenByAssetAndItem(assetId, id)) {
            notify.enqueueIssueEvent(asset, toIssue(inc), "firing", now, ignored, inc.getId(), true);
        }
    }

    /** Close open incidents for a deleted/disabled rule. */
    @Transactional
    public void resolveForRuleChange(UUID ruleId) {
        if (ruleId == null) {
            return;
        }
        Instant now = Instant.now();
        pendings.deleteByRuleId(ruleId);
        for (AlertIncidentEntity inc : incidents.findByRuleIdAndResolvedAtIsNull(ruleId)) {
            AssetEntity asset = assets.findById(inc.getAssetId()).orElse(null);
            closeIncident(inc, now, AlertIncidentEntity.REASON_RULE_CHANGED);
            if (asset != null) {
                Set<String> ignored = ignoredItemIds(asset.getId());
                notify.enqueueIssueEvent(asset, toIssue(inc), "resolved", now, ignored, inc.getId());
                recompute(asset.getId());
            }
        }
    }

    /** Clear pending timers for a rule (threshold/duration edits) then re-evaluate. */
    @Transactional
    public void clearPendingForRule(UUID ruleId) {
        if (ruleId != null) {
            pendings.deleteByRuleId(ruleId);
        }
    }

    @Transactional(readOnly = true)
    public Map<String, Object> listIncidents(
            UserEntity user,
            boolean superAdmin,
            Set<UUID> visibleAssetIds,
            UUID assetId,
            String status,
            Instant from,
            Instant to,
            int page,
            int pageSize) {
        int size = PageSupport.clampPageSize(pageSize);
        int p = Math.max(page, 1);
        boolean openOnly = "open".equalsIgnoreCase(status) || "firing".equalsIgnoreCase(status);
        boolean resolvedOnly = "resolved".equalsIgnoreCase(status);
        boolean filterEmpty = superAdmin;
        // JPQL IN () cannot be empty; dummy unused when filterEmpty=true.
        Collection<UUID> assetIds = superAdmin
                ? List.of(new UUID(0, 0))
                : (visibleAssetIds == null ? Set.of() : visibleAssetIds);
        if (!superAdmin && assetIds.isEmpty()) {
            return PageSupport.emptyPage(p, size);
        }
        if (assetId != null && !superAdmin && !assetIds.contains(assetId)) {
            return PageSupport.emptyPage(p, size);
        }
        // Postgres cannot infer type of null Instant/UUID binds in "(:x is null or …)";
        // use sentinels like ServerOperationAuditService.
        boolean hasAsset = assetId != null;
        UUID assetIdParam = hasAsset ? assetId : new UUID(0, 0);
        Instant fromBound = from != null ? from : Instant.EPOCH;
        Instant toBound = to != null ? to : Instant.parse("9999-12-31T00:00:00Z");
        Page<AlertIncidentEntity> result = incidents.search(
                hasAsset,
                assetIdParam,
                openOnly,
                resolvedOnly,
                fromBound,
                toBound,
                filterEmpty,
                assetIds,
                PageRequest.of(p - 1, size));
        Map<UUID, String> groupNames = groups.nameById();
        Map<UUID, AssetEntity> assetCache = new HashMap<>();
        List<Map<String, Object>> items = new ArrayList<>();
        for (AlertIncidentEntity inc : result.getContent()) {
            items.add(incidentView(inc, assetCache, groupNames));
        }
        return PageSupport.pageResult(items, result.getTotalElements(), p, size);
    }

    private Map<String, Object> incidentView(
            AlertIncidentEntity inc, Map<UUID, AssetEntity> assetCache, Map<UUID, String> groupNames) {
        AssetEntity a = assetCache.computeIfAbsent(inc.getAssetId(), id -> assets.findById(id).orElse(null));
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", inc.getId().toString());
        m.put("assetId", inc.getAssetId().toString());
        m.put("assetName", a == null ? "" : (a.getDisplayName() != null ? a.getDisplayName() : a.getId().toString()));
        m.put("hostname", a == null || a.getHostname() == null ? "" : a.getHostname());
        m.put("groupId", a == null || a.getGroupId() == null ? null : a.getGroupId().toString());
        m.put("groupName", a == null || a.getGroupId() == null ? "" : groupNames.getOrDefault(a.getGroupId(), ""));
        m.put("privateIp", a == null || a.getPrivateIp() == null ? "" : a.getPrivateIp());
        m.put("publicIp", a == null || a.getPublicIp() == null ? "" : a.getPublicIp());
        m.put("itemId", inc.getItemId());
        m.put("instance", inc.getInstance());
        m.put("kind", inc.getKind());
        m.put("ruleId", inc.getRuleId() == null ? null : inc.getRuleId().toString());
        m.put("ruleName", inc.getRuleName());
        m.put("op", inc.getOp());
        m.put("threshold", inc.getThreshold());
        m.put("startValue", inc.getStartValue());
        m.put("latestValue", inc.getLatestValue());
        m.put("label", inc.getLabel());
        m.put("startedAt", inc.getStartedAt().toString());
        m.put("lastSeenAt", inc.getLastSeenAt().toString());
        m.put("resolvedAt", inc.getResolvedAt() == null ? null : inc.getResolvedAt().toString());
        m.put("resolutionReason", inc.getResolutionReason());
        m.put("open", inc.isOpen());
        return m;
    }

    private void upsertPending(
            Map<String, AlertPendingEntity> pendingByKey,
            UUID assetId,
            AlertIssueLogic.Issue issue,
            String phase,
            Instant now) {
        if (issue.ruleId() == null) {
            return;
        }
        String key = AlertIssueLogic.issueKey(issue);
        AlertPendingEntity p = pendingByKey.get(key);
        if (p == null) {
            p = new AlertPendingEntity();
            p.setAssetId(assetId);
            p.setRuleId(issue.ruleId());
            p.setItemId(issue.itemId());
            p.setInstance(issue.instance());
            p.setPhase(phase);
            p.setSinceAt(now);
            pendingByKey.put(key, p);
        } else if (!phase.equals(p.getPhase())) {
            p.setPhase(phase);
            p.setSinceAt(now);
        }
        p.setLatestValue(issue.value());
        p.setLabel(issue.label());
        p.setUpdatedAt(now);
        pendings.save(p);
    }

    private void clearPending(
            Map<String, AlertPendingEntity> pendingByKey, UUID assetId, AlertIssueLogic.Issue issue) {
        if (issue.ruleId() == null) {
            return;
        }
        String key = AlertIssueLogic.issueKey(issue);
        AlertPendingEntity p = pendingByKey.remove(key);
        if (p != null) {
            pendings.delete(p);
        } else {
            pendings.deleteOne(assetId, issue.ruleId(), issue.itemId(), issue.instance() == null ? "" : issue.instance());
        }
    }

    private static int fireMinutesOf(UUID ruleId, Map<UUID, MetricAlertRuleEntity> ruleById) {
        if (ruleId == null) {
            return 0;
        }
        MetricAlertRuleEntity r = ruleById.get(ruleId);
        return r == null ? 0 : Math.max(0, r.getFireMinutes());
    }

    private static int recoverMinutesOf(UUID ruleId, Map<UUID, MetricAlertRuleEntity> ruleById) {
        if (ruleId == null) {
            return 0;
        }
        MetricAlertRuleEntity r = ruleById.get(ruleId);
        // Rule missing (disabled mid-flight): close immediately.
        return r == null ? 0 : Math.max(0, r.getRecoverMinutes());
    }

    private static List<AlertIssueLogic.Issue> statusIssuesFromOpen(
            AssetEntity asset, Collection<AlertIncidentEntity> open, Set<String> ignored) {
        Set<String> ig = ignored == null ? Set.of() : ignored;
        if (!asset.isOnline()) {
            if (ig.contains(AlertIssueLogic.ITEM_HOST_ONLINE)) {
                return List.of();
            }
            return List.of(AlertIssueLogic.offlineIssue());
        }
        List<AlertIssueLogic.Issue> out = new ArrayList<>();
        for (AlertIncidentEntity inc : open) {
            if (!inc.isOpen()) {
                continue;
            }
            if (ig.contains(inc.getItemId())) {
                continue;
            }
            if (AlertIssueLogic.KIND_OFFLINE.equals(inc.getKind())) {
                continue;
            }
            out.add(toIssue(inc));
        }
        return out;
    }

    private AlertIncidentEntity openIncident(UUID assetId, AlertIssueLogic.Issue issue, Instant now) {
        AlertIncidentEntity inc = new AlertIncidentEntity();
        inc.setId(UUID.randomUUID());
        inc.setAssetId(assetId);
        inc.setRuleId(issue.ruleId());
        inc.setItemId(issue.itemId());
        inc.setInstance(issue.instance());
        inc.setKind(issue.kind());
        inc.setRuleName(issue.ruleName() == null ? "" : issue.ruleName());
        inc.setOp(issue.op() == null ? "" : issue.op());
        inc.setThreshold(issue.threshold());
        inc.setStartValue(issue.value());
        inc.setLatestValue(issue.value());
        inc.setLabel(issue.label());
        inc.setStartedAt(now);
        inc.setLastSeenAt(now);
        return incidents.save(inc);
    }

    private void closeIncident(AlertIncidentEntity inc, Instant now, String reason) {
        inc.setResolvedAt(now);
        inc.setResolutionReason(reason);
        inc.setLastSeenAt(now);
        incidents.save(inc);
    }

    private static String resolveReason(AlertIncidentEntity inc, Set<String> goneKeys, boolean assetOffline) {
        if (goneKeys != null && goneKeys.contains(MetricFamilyReconcile.key(inc.getItemId(), inc.getInstance()))) {
            return AlertIncidentEntity.REASON_INSTANCE_GONE;
        }
        if (assetOffline && !AlertIssueLogic.KIND_OFFLINE.equals(inc.getKind())) {
            return AlertIncidentEntity.REASON_ASSET_OFFLINE;
        }
        return AlertIncidentEntity.REASON_NORMAL;
    }

    private static AlertIssueLogic.Issue toIssue(AlertIncidentEntity inc) {
        return new AlertIssueLogic.Issue(
                inc.getItemId(),
                inc.getKind(),
                inc.getLabel(),
                inc.getInstance(),
                inc.getLatestValue() == null ? 0 : inc.getLatestValue(),
                inc.getRuleId(),
                inc.getRuleName(),
                inc.getOp(),
                inc.getThreshold());
    }

    private Set<String> ignoredItemIds(UUID assetId) {
        Set<String> out = new HashSet<>();
        for (AssetAlertIgnoreEntity ig : alertIgnores.findByAssetId(assetId)) {
            out.add(ig.getItemId());
        }
        return out;
    }

    private void writeAssetStatus(UUID assetId, List<AlertIssueLogic.Issue> visible, Instant now) {
        AssetAlertStatusEntity st = alertStatus.findById(assetId).orElseGet(() -> {
            AssetAlertStatusEntity n = new AssetAlertStatusEntity();
            n.setAssetId(assetId);
            return n;
        });
        List<String> labels = new ArrayList<>();
        for (AlertIssueLogic.Issue issue : visible) {
            if (AlertIssueLogic.KIND_OFFLINE.equals(issue.kind())) {
                labels.add("offline");
            } else if (issue.label() != null && !issue.label().isBlank()) {
                labels.add(issue.label());
            } else {
                labels.add(issue.itemId());
            }
        }
        st.setFiring(!labels.isEmpty());
        st.setSummary(labels.isEmpty() ? "" : String.join("; ", labels.stream().limit(5).toList()));
        st.setUpdatedAt(now);
        alertStatus.save(st);
    }
}
