package com.ops.control.metrics;

import com.ops.control.agent.AgentService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.GroupService;
import com.ops.control.user.UserEntity;
import jakarta.transaction.Transactional;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;

import java.sql.Timestamp;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.*;

@Service
public class MetricsService {
    private final AgentService agents;
    private final AssetRepository assets;
    private final MonitorDataRepository dataRepo;
    private final MonitorLatestRepository latestRepo;
    private final MonitorItemDefRepository itemDefs;
    private final MetricAlertRuleRepository rules;
    private final AssetAlertStatusRepository alertStatus;
    private final AssetAlertIgnoreRepository alertIgnores;
    private final ControlAuditService audit;
    private final GroupService groups;
    private final JdbcTemplate jdbc;

    public MetricsService(
            AgentService agents,
            AssetRepository assets,
            MonitorDataRepository dataRepo,
            MonitorLatestRepository latestRepo,
            MonitorItemDefRepository itemDefs,
            MetricAlertRuleRepository rules,
            AssetAlertStatusRepository alertStatus,
            AssetAlertIgnoreRepository alertIgnores,
            ControlAuditService audit,
            GroupService groups,
            JdbcTemplate jdbc) {
        this.agents = agents;
        this.assets = assets;
        this.dataRepo = dataRepo;
        this.latestRepo = latestRepo;
        this.itemDefs = itemDefs;
        this.rules = rules;
        this.alertStatus = alertStatus;
        this.alertIgnores = alertIgnores;
        this.audit = audit;
        this.groups = groups;
        this.jdbc = jdbc;
    }

    public record PointIn(String itemId, String instance, double value) {}

    @Transactional
    public void ingest(String assetIdStr, String agentToken, String collectedAt, List<PointIn> points) {
        AssetEntity asset = agents.authenticateAgent(assetIdStr, agentToken)
                .orElseThrow(() -> new IllegalArgumentException("invalid agent credentials"));
        Instant ts = parseTime(collectedAt);
        UUID assetId = asset.getId();
        if (points == null || points.isEmpty()) {
            return;
        }

        jdbc.batchUpdate(
                """
                insert into monitor_data (asset_id, item_id, instance, time, metric_value)
                values (?, ?, ?, ?, ?)
                on conflict (asset_id, item_id, instance, time) do update set metric_value = excluded.metric_value
                """,
                points,
                200,
                (ps, p) -> {
                    String instance = p.instance() == null ? "" : p.instance();
                    ps.setObject(1, assetId);
                    ps.setString(2, p.itemId());
                    ps.setString(3, instance);
                    ps.setTimestamp(4, Timestamp.from(ts));
                    ps.setDouble(5, p.value());
                });

        jdbc.batchUpdate(
                """
                insert into monitor_latest (asset_id, item_id, instance, time, metric_value)
                values (?, ?, ?, ?, ?)
                on conflict (asset_id, item_id, instance) do update
                  set time = excluded.time, metric_value = excluded.metric_value
                """,
                points,
                200,
                (ps, p) -> {
                    String instance = p.instance() == null ? "" : p.instance();
                    ps.setObject(1, assetId);
                    ps.setString(2, p.itemId());
                    ps.setString(3, instance);
                    ps.setTimestamp(4, Timestamp.from(ts));
                    ps.setDouble(5, p.value());
                });

        evaluateAlerts(assetId, points);
    }

    private void evaluateAlerts(UUID assetId, List<PointIn> points) {
        List<MetricAlertRuleEntity> enabled = rules.findByEnabledTrue();
        if (enabled.isEmpty()) {
            AssetAlertStatusEntity st = alertStatus.findById(assetId).orElseGet(() -> {
                AssetAlertStatusEntity n = new AssetAlertStatusEntity();
                n.setAssetId(assetId);
                return n;
            });
            st.setFiring(false);
            st.setSummary("");
            st.setUpdatedAt(Instant.now());
            alertStatus.save(st);
            return;
        }
        List<String> hits = new ArrayList<>();
        for (MetricAlertRuleEntity rule : enabled) {
            for (PointIn p : points) {
                if (!rule.getItemId().equals(p.itemId())) continue;
                String inst = p.instance() == null ? "" : p.instance();
                if (!rule.getInstance().isEmpty() && !rule.getInstance().equals(inst)) continue;
                if (AlertIssueLogic.matches(rule.getOp(), p.value(), rule.getThreshold())) {
                    String label = rule.getName();
                    if (!inst.isEmpty()) label = label + " (" + inst + ")";
                    hits.add(label + "=" + AlertIssueLogic.formatNum(p.value()));
                }
            }
        }
        AssetAlertStatusEntity st = alertStatus.findById(assetId).orElseGet(() -> {
            AssetAlertStatusEntity n = new AssetAlertStatusEntity();
            n.setAssetId(assetId);
            return n;
        });
        st.setFiring(!hits.isEmpty());
        st.setSummary(hits.isEmpty() ? "" : String.join("; ", hits.stream().limit(5).toList()));
        st.setUpdatedAt(Instant.now());
        alertStatus.save(st);
    }

    public List<Map<String, Object>> latest(UUID assetId) {
        Map<String, MonitorItemDefEntity> defs = new HashMap<>();
        for (MonitorItemDefEntity d : itemDefs.findAll()) {
            defs.put(d.getItemId(), d);
        }
        List<Map<String, Object>> out = new ArrayList<>();
        for (MonitorLatestEntity row : latestRepo.findByAssetIdOrderByItemIdAscInstanceAsc(assetId)) {
            MonitorItemDefEntity def = defs.get(row.getItemId());
            Map<String, Object> m = new LinkedHashMap<>();
            m.put("itemId", row.getItemId());
            m.put("instance", row.getInstance());
            m.put("name", def != null ? def.getName() : row.getItemId());
            m.put("unit", def != null ? def.getUnit() : "");
            m.put("time", row.getTime().toString());
            m.put("value", row.getValue());
            out.add(m);
        }
        return out;
    }

    public Map<String, Object> series(UUID assetId, List<String> itemIds, Instant from, Instant to,
                                      String grain, String instance) {
        if (itemIds == null || itemIds.isEmpty()) {
            itemIds = itemDefs.findByChartDefaultTrueOrderByItemIdAsc().stream()
                    .map(MonitorItemDefEntity::getItemId).toList();
        }
        String g = normalizeGrain(grain);
        String inst = instance == null ? "" : instance;
        // Native any(text[]) is awkward via Spring; query per itemId for simplicity.
        Map<String, List<Map<String, Object>>> series = new LinkedHashMap<>();
        Map<String, MonitorItemDefEntity> defs = new HashMap<>();
        for (MonitorItemDefEntity d : itemDefs.findAll()) {
            defs.put(d.getItemId(), d);
        }
        for (String itemId : itemIds) {
            List<Map<String, Object>> points = jdbc.query(
                    """
                    select date_trunc(?, time) as bucket, instance, avg(metric_value) as metric_value
                    from monitor_data
                    where asset_id = ? and item_id = ?
                      and time >= ? and time < ?
                      and (? = '' or instance = ?)
                    group by bucket, instance
                    order by bucket asc, instance asc
                    """,
                    (rs, i) -> {
                        Map<String, Object> m = new LinkedHashMap<>();
                        Timestamp bucket = rs.getTimestamp("bucket");
                        m.put("time", bucket.toInstant().toString());
                        m.put("instance", rs.getString("instance"));
                        m.put("value", rs.getDouble("metric_value"));
                        return m;
                    },
                    g, assetId, itemId, Timestamp.from(from), Timestamp.from(to), inst, inst);
            // Split by instance into named series keys
            Map<String, List<Map<String, Object>>> byInst = new LinkedHashMap<>();
            for (Map<String, Object> p : points) {
                String key = itemId + (p.get("instance") == null || p.get("instance").toString().isEmpty()
                        ? "" : ("|" + p.get("instance")));
                byInst.computeIfAbsent(key, k -> new ArrayList<>()).add(Map.of(
                        "time", p.get("time"),
                        "value", p.get("value")
                ));
            }
            for (var e : byInst.entrySet()) {
                series.put(e.getKey(), e.getValue());
            }
        }
        List<Map<String, Object>> meta = new ArrayList<>();
        for (String itemId : itemIds) {
            MonitorItemDefEntity def = defs.get(itemId);
            Map<String, Object> m = new LinkedHashMap<>();
            m.put("itemId", itemId);
            m.put("name", def != null ? def.getName() : itemId);
            m.put("unit", def != null ? def.getUnit() : "");
            meta.add(m);
        }
        return Map.of(
                "grain", g,
                "from", from.toString(),
                "to", to.toString(),
                "items", meta,
                "series", series
        );
    }

    private static String normalizeGrain(String grain) {
        if (grain == null) return "minute";
        return switch (grain.toLowerCase(Locale.ROOT)) {
            case "day" -> "day";
            case "month" -> "month";
            default -> "minute";
        };
    }

    public Map<String, Object> dashboardSummary() {
        List<AssetEntity> allAssets = assets.findAll();
        Map<UUID, AssetEntity> assetById = new HashMap<>();
        for (AssetEntity a : allAssets) {
            assetById.put(a.getId(), a);
        }
        long total = allAssets.size();
        long online = allAssets.stream().filter(AssetEntity::isOnline).count();

        Map<UUID, Set<String>> ignoredByAsset = new HashMap<>();
        List<AssetAlertIgnoreEntity> ignoreRows = alertIgnores.findAllByOrderByIgnoredAtDesc();
        for (AssetAlertIgnoreEntity ig : ignoreRows) {
            ignoredByAsset.computeIfAbsent(ig.getAssetId(), k -> new HashSet<>()).add(ig.getItemId());
        }

        List<AlertIssueLogic.Rule> enabledRules = rules.findByEnabledTrue().stream()
                .map(r -> new AlertIssueLogic.Rule(r.getName(), r.getItemId(), r.getInstance(), r.getOp(), r.getThreshold()))
                .toList();
        Map<UUID, List<AlertIssueLogic.Point>> latestByAsset = new HashMap<>();
        for (MonitorLatestEntity row : latestRepo.findAll()) {
            latestByAsset.computeIfAbsent(row.getAssetId(), k -> new ArrayList<>())
                    .add(new AlertIssueLogic.Point(row.getItemId(), row.getInstance(), row.getValue()));
        }

        Map<UUID, String> groupNames = groups.nameById();
        List<Map<String, Object>> firing = new ArrayList<>();
        for (AssetEntity a : allAssets) {
            List<AlertIssueLogic.Issue> metricHits = AlertIssueLogic.metricHits(
                    latestByAsset.getOrDefault(a.getId(), List.of()), enabledRules);
            List<AlertIssueLogic.Issue> visible = AlertIssueLogic.visibleIssues(
                    a.isOnline(), metricHits, ignoredByAsset.getOrDefault(a.getId(), Set.of()));
            if (visible.isEmpty()) {
                continue;
            }
            firing.add(abnormalView(a, visible, groupNames));
        }
        firing.sort(Comparator
                .comparing((Map<String, Object> m) -> Boolean.TRUE.equals(m.get("online")))
                .thenComparing(m -> String.valueOf(m.getOrDefault("displayName", "")), String.CASE_INSENSITIVE_ORDER));

        Map<String, String> itemNames = itemNameMap();
        List<Map<String, Object>> ignoredAlerts = new ArrayList<>();
        for (AssetAlertIgnoreEntity ig : ignoreRows) {
            AssetEntity a = assetById.get(ig.getAssetId());
            if (a == null) {
                continue;
            }
            ignoredAlerts.add(ignoreView(a, ig, itemNames, groupNames));
        }

        Map<String, Object> out = new LinkedHashMap<>();
        out.put("assetTotal", total);
        out.put("assetOnline", online);
        out.put("assetAbnormal", firing.size());
        out.put("abnormalAssets", firing);
        out.put("ignoredAlerts", ignoredAlerts);
        return out;
    }

    @Transactional
    public Map<String, Object> ignoreAlert(UserEntity user, UUID assetId, String itemId) {
        AssetEntity asset = assets.findById(assetId).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        String id = normalizeItemId(itemId);
        AssetAlertIgnoreId pk = new AssetAlertIgnoreId(assetId, id);
        AssetAlertIgnoreEntity row = alertIgnores.findById(pk).orElse(null);
        if (row == null) {
            row = new AssetAlertIgnoreEntity();
            row.setAssetId(assetId);
            row.setItemId(id);
            row.setIgnoredAt(Instant.now());
            row.setIgnoredBy(user.getId());
            alertIgnores.save(row);
            audit.record(
                    ControlAuditService.CAT_MONITOR,
                    ControlAuditService.ACT_IGNORE,
                    user.getId(),
                    user.getUsername(),
                    asset.getId(),
                    asset.getGroupId(),
                    null,
                    ControlAuditService.jsonDetail(Map.of("itemId", id)));
        }
        return ignoreView(asset, row, itemNameMap(), groups.nameById());
    }

    @Transactional
    public void unignoreAlert(UserEntity user, UUID assetId, String itemId) {
        AssetEntity asset = assets.findById(assetId).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        String id = normalizeItemId(itemId);
        AssetAlertIgnoreId pk = new AssetAlertIgnoreId(assetId, id);
        if (alertIgnores.existsById(pk)) {
            alertIgnores.deleteById(pk);
            audit.record(
                    ControlAuditService.CAT_MONITOR,
                    ControlAuditService.ACT_UNIGNORE,
                    user.getId(),
                    user.getUsername(),
                    asset.getId(),
                    asset.getGroupId(),
                    null,
                    ControlAuditService.jsonDetail(Map.of("itemId", id)));
        }
    }

    private Map<String, Object> abnormalView(
            AssetEntity a, List<AlertIssueLogic.Issue> issues, Map<UUID, String> groupNames) {
        List<Map<String, Object>> issueViews = new ArrayList<>();
        List<String> labels = new ArrayList<>();
        for (AlertIssueLogic.Issue issue : issues) {
            Map<String, Object> m = new LinkedHashMap<>();
            m.put("itemId", issue.itemId());
            m.put("kind", issue.kind());
            m.put("label", issue.label());
            m.put("instance", issue.instance());
            issueViews.add(m);
            if (AlertIssueLogic.KIND_OFFLINE.equals(issue.kind())) {
                labels.add("offline");
            } else if (issue.label() != null && !issue.label().isBlank()) {
                labels.add(issue.label());
            } else {
                labels.add(issue.itemId());
            }
        }
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("assetId", a.getId().toString());
        m.put("displayName", a.getDisplayName() != null ? a.getDisplayName() : a.getId().toString());
        m.put("hostname", a.getHostname() != null ? a.getHostname() : "");
        putGroupFields(m, a, groupNames);
        m.put("online", a.isOnline());
        m.put("summary", String.join("; ", labels));
        m.put("issues", issueViews);
        return m;
    }

    private Map<String, Object> ignoreView(
            AssetEntity a, AssetAlertIgnoreEntity ig, Map<String, String> itemNames, Map<UUID, String> groupNames) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("assetId", a.getId().toString());
        m.put("displayName", a.getDisplayName() != null ? a.getDisplayName() : a.getId().toString());
        m.put("hostname", a.getHostname() != null ? a.getHostname() : "");
        putGroupFields(m, a, groupNames);
        m.put("online", a.isOnline());
        m.put("itemId", ig.getItemId());
        m.put("kind", AlertIssueLogic.kindOf(ig.getItemId()));
        m.put("itemName", itemNames.getOrDefault(ig.getItemId(), ig.getItemId()));
        m.put("ignoredAt", ig.getIgnoredAt() != null ? ig.getIgnoredAt().toString() : "");
        return m;
    }

    private static void putGroupFields(Map<String, Object> m, AssetEntity a, Map<UUID, String> groupNames) {
        m.put("groupId", a.getGroupId() == null ? null : a.getGroupId().toString());
        m.put("groupName", a.getGroupId() == null ? "" : groupNames.getOrDefault(a.getGroupId(), ""));
    }

    private Map<String, String> itemNameMap() {
        Map<String, String> names = new HashMap<>();
        for (MonitorItemDefEntity d : itemDefs.findAll()) {
            names.put(d.getItemId(), d.getName());
        }
        return names;
    }

    private static String normalizeItemId(String itemId) {
        if (itemId == null || itemId.isBlank()) {
            throw new IllegalArgumentException("itemId required");
        }
        String id = itemId.trim();
        if (id.length() > 128) {
            throw new IllegalArgumentException("itemId too long");
        }
        return id;
    }

    public List<Map<String, Object>> listItemDefs() {
        return itemDefs.findAll().stream().sorted(Comparator.comparing(MonitorItemDefEntity::getItemId))
                .map(d -> {
                    Map<String, Object> m = new LinkedHashMap<>();
                    m.put("itemId", d.getItemId());
                    m.put("name", d.getName());
                    m.put("unit", d.getUnit());
                    m.put("chartDefault", d.isChartDefault());
                    return m;
                }).toList();
    }

    public List<Map<String, Object>> listRules() {
        return rules.findAllByOrderByNameAsc().stream().map(this::ruleView).toList();
    }

    @Transactional
    public Map<String, Object> createRule(String name, String itemId, String instance, String op, double threshold, Boolean enabled) {
        MetricAlertRuleEntity r = new MetricAlertRuleEntity();
        r.setId(UUID.randomUUID());
        r.setName(name);
        r.setItemId(itemId);
        r.setInstance(instance == null ? "" : instance);
        r.setOp(op == null || op.isBlank() ? "gt" : op);
        r.setThreshold(threshold);
        r.setEnabled(enabled == null || enabled);
        r.setCreatedAt(Instant.now());
        r.setUpdatedAt(Instant.now());
        return ruleView(rules.save(r));
    }

    @Transactional
    public Map<String, Object> updateRule(UUID id, String name, String itemId, String instance, String op, Double threshold, Boolean enabled) {
        MetricAlertRuleEntity r = rules.findById(id).orElseThrow(() -> new IllegalArgumentException("rule not found"));
        if (name != null && !name.isBlank()) r.setName(name);
        if (itemId != null && !itemId.isBlank()) r.setItemId(itemId);
        if (instance != null) r.setInstance(instance);
        if (op != null && !op.isBlank()) r.setOp(op);
        if (threshold != null) r.setThreshold(threshold);
        if (enabled != null) r.setEnabled(enabled);
        r.setUpdatedAt(Instant.now());
        return ruleView(rules.save(r));
    }

    @Transactional
    public void deleteRule(UUID id) {
        rules.deleteById(id);
    }

    private Map<String, Object> ruleView(MetricAlertRuleEntity r) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", r.getId().toString());
        m.put("name", r.getName());
        m.put("itemId", r.getItemId());
        m.put("instance", r.getInstance());
        m.put("op", r.getOp());
        m.put("threshold", r.getThreshold());
        m.put("enabled", r.isEnabled());
        m.put("updatedAt", r.getUpdatedAt().toString());
        return m;
    }

    @Transactional
    public int purgeOlderThanThreeYears() {
        Instant before = Instant.now().minus(365 * 3L, ChronoUnit.DAYS);
        return dataRepo.deleteOlderThan(before);
    }

    private static Instant parseTime(String collectedAt) {
        if (collectedAt == null || collectedAt.isBlank()) return Instant.now();
        try {
            return Instant.parse(collectedAt);
        } catch (Exception e) {
            return Instant.now();
        }
    }
}

