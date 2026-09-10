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
import java.time.Duration;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.*;

@Service
public class MetricsService {
    private final AgentService agents;
    private final AssetRepository assets;
    private final MonitorDataRepository dataRepo;
    private final MonitorTrendsRepository trendsRepo;
    private final MonitorLatestRepository latestRepo;
    private final MonitorItemDefRepository itemDefs;
    private final MetricAlertRuleRepository rules;
    private final AssetAlertStatusRepository alertStatus;
    private final AssetAlertIgnoreRepository alertIgnores;
    private final ControlAuditService audit;
    private final GroupService groups;
    private final JdbcTemplate jdbc;
    private final MonitorProperties monitor;

    public MetricsService(
            AgentService agents,
            AssetRepository assets,
            MonitorDataRepository dataRepo,
            MonitorTrendsRepository trendsRepo,
            MonitorLatestRepository latestRepo,
            MonitorItemDefRepository itemDefs,
            MetricAlertRuleRepository rules,
            AssetAlertStatusRepository alertStatus,
            AssetAlertIgnoreRepository alertIgnores,
            ControlAuditService audit,
            GroupService groups,
            JdbcTemplate jdbc,
            MonitorProperties monitor) {
        this.agents = agents;
        this.assets = assets;
        this.dataRepo = dataRepo;
        this.trendsRepo = trendsRepo;
        this.latestRepo = latestRepo;
        this.itemDefs = itemDefs;
        this.rules = rules;
        this.alertStatus = alertStatus;
        this.alertIgnores = alertIgnores;
        this.audit = audit;
        this.groups = groups;
        this.jdbc = jdbc;
        this.monitor = monitor;
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
                insert into monitor_history (asset_id, item_id, instance, time, metric_value)
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
        boolean useTrends = useTrends(from, to);
        String g = effectiveGrain(grain, from, to, useTrends);
        String inst = instance == null ? "" : instance;
        Map<String, List<Map<String, Object>>> series = new LinkedHashMap<>();
        Map<String, MonitorItemDefEntity> defs = new HashMap<>();
        for (MonitorItemDefEntity d : itemDefs.findAll()) {
            defs.put(d.getItemId(), d);
        }
        String placeholders = String.join(",", Collections.nCopies(itemIds.size(), "?"));
        String sql;
        List<Object> args = new ArrayList<>();
        if (useTrends) {
            sql = """
                    select item_id, date_trunc(?, hour) as bucket, instance, avg(avg_value) as metric_value
                    from monitor_trends
                    where asset_id = ? and item_id in (%s)
                      and hour >= ? and hour < ?
                      and (? = '' or instance = ?)
                    group by item_id, bucket, instance
                    order by item_id asc, bucket asc, instance asc
                    """.formatted(placeholders);
        } else {
            sql = """
                    select item_id, date_trunc(?, time) as bucket, instance, avg(metric_value) as metric_value
                    from monitor_history
                    where asset_id = ? and item_id in (%s)
                      and time >= ? and time < ?
                      and (? = '' or instance = ?)
                    group by item_id, bucket, instance
                    order by item_id asc, bucket asc, instance asc
                    """.formatted(placeholders);
        }
        args.add(g);
        args.add(assetId);
        args.addAll(itemIds);
        args.add(Timestamp.from(from));
        args.add(Timestamp.from(to));
        args.add(inst);
        args.add(inst);
        jdbc.query(sql, rs -> {
            String itemId = rs.getString("item_id");
            String rowInstance = rs.getString("instance");
            String key = itemId + (rowInstance == null || rowInstance.isEmpty() ? "" : ("|" + rowInstance));
            Timestamp bucket = rs.getTimestamp("bucket");
            series.computeIfAbsent(key, k -> new ArrayList<>()).add(Map.of(
                    "time", bucket.toInstant().toString(),
                    "value", rs.getDouble("metric_value")
            ));
        }, args.toArray());
        // Keep response series ordered by the requested chart order rather than SQL lexical order.
        Map<String, List<Map<String, Object>>> orderedSeries = new LinkedHashMap<>();
        for (String itemId : itemIds) {
            series.forEach((key, points) -> {
                if (key.equals(itemId) || key.startsWith(itemId + "|")) {
                    orderedSeries.put(key, points);
                }
            });
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
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("grain", g);
        out.put("source", useTrends ? "trends" : "history");
        out.put("from", from.toString());
        out.put("to", to.toString());
        out.put("items", meta);
        out.put("series", orderedSeries);
        return out;
    }

    boolean useTrends(Instant from, Instant to) {
        return useTrends(from, to, Instant.now(), monitor.detailRetentionDays());
    }

    /**
     * History only keeps the recent retention window. Use trends when the requested
     * range starts before that window (e.g. brush-select last month) or spans longer
     * than retention — not merely when (to-from) looks "long".
     */
    static boolean useTrends(Instant from, Instant to, Instant now, int detailRetentionDays) {
        Instant historyStart = now.minus(detailRetentionDays, ChronoUnit.DAYS);
        if (from.isBefore(historyStart)) {
            return true;
        }
        return Duration.between(from, to).compareTo(Duration.ofDays(detailRetentionDays)) > 0;
    }

    static String normalizeGrain(String grain) {
        if (grain == null) return "minute";
        return switch (grain.toLowerCase(Locale.ROOT)) {
            case "hour" -> "hour";
            case "day" -> "day";
            case "month" -> "month";
            default -> "minute";
        };
    }

    static String effectiveGrain(String requested, Instant from, Instant to) {
        return effectiveGrain(requested, from, to, false);
    }

    /**
     * Honor the requested grain. Only rewrite when the source cannot serve it:
     * trends have no minute points, so minute → hour.
     * Defaults / recommendations belong to the UI, not forced here.
     */
    static String effectiveGrain(String requested, Instant from, Instant to, boolean useTrends) {
        String grain = normalizeGrain(requested);
        if (useTrends && "minute".equals(grain)) {
            return "hour";
        }
        return grain;
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
    public int purgeHistoryOlderThanRetention() {
        Instant before = Instant.now().minus(monitor.detailRetentionDays(), ChronoUnit.DAYS);
        return dataRepo.deleteOlderThan(before);
    }

    @Transactional
    public int purgeTrendsOlderThanRetention() {
        Instant before = Instant.now().minus(monitor.trendsRetentionDays(), ChronoUnit.DAYS);
        return trendsRepo.deleteOlderThan(before);
    }

    /**
     * Aggregate the previous completed UTC hour from monitor_history into monitor_trends.
     * Idempotent via ON CONFLICT DO UPDATE.
     */
    @Transactional
    public int rollupPreviousHour() {
        Instant hourStart = Instant.now().truncatedTo(ChronoUnit.HOURS).minus(1, ChronoUnit.HOURS);
        Instant hourEnd = hourStart.plus(1, ChronoUnit.HOURS);
        return jdbc.update(
                """
                insert into monitor_trends (asset_id, item_id, instance, hour, min_value, max_value, avg_value, sample_count)
                select asset_id, item_id, instance,
                       ?::timestamptz as hour,
                       min(metric_value), max(metric_value), avg(metric_value), count(*)::int
                from monitor_history
                where time >= ? and time < ?
                group by asset_id, item_id, instance
                on conflict (asset_id, item_id, instance, hour) do update set
                  min_value = excluded.min_value,
                  max_value = excluded.max_value,
                  avg_value = excluded.avg_value,
                  sample_count = excluded.sample_count
                """,
                Timestamp.from(hourStart),
                Timestamp.from(hourStart),
                Timestamp.from(hourEnd));
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

