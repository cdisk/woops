package com.ops.control.asset;

import java.util.ArrayList;
import java.util.Collection;
import java.util.Comparator;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

/**
 * Explicit update-upstream graph: cycle checks, leaf-to-root plan, and the
 * visible topology. Update order and the picture share this walk so a chain
 * through an unselected (or invisible) hop still orders the same way.
 */
public final class UpdateTopology {
    public static final String GATEWAY_ID = "gateway";

    private UpdateTopology() {}

    public static String hiddenId(UUID visibleAssetId) {
        return "hidden:" + visibleAssetId;
    }

    /** First comma-separated private address; empty when unset. */
    public static String primaryPrivateIp(String csv) {
        if (csv == null || csv.isBlank()) {
            return "";
        }
        int comma = csv.indexOf(',');
        String first = comma < 0 ? csv : csv.substring(0, comma);
        return first.trim();
    }

    /**
     * Copy {@code current} and point each id at {@code upstream} (null clears).
     * Throws when the edit would self-point or close a cycle.
     */
    public static Map<UUID, UUID> withUpstream(Map<UUID, UUID> current, Collection<UUID> assetIds, UUID upstream) {
        Map<UUID, UUID> next = new LinkedHashMap<>();
        if (current != null) {
            next.putAll(current);
        }
        List<UUID> ids = distinct(assetIds);
        if (ids.isEmpty()) {
            throw new IllegalArgumentException("assetIds required");
        }
        if (upstream != null && ids.contains(upstream)) {
            throw new IllegalArgumentException("asset cannot be its own update upstream");
        }
        for (UUID id : ids) {
            if (upstream == null) {
                next.remove(id);
            } else {
                next.put(id, upstream);
            }
        }
        assertNoCycleFrom(next, ids);
        return next;
    }

    /** Drop the deleted asset and any upstream pointer aimed at it. */
    public static Map<UUID, UUID> withoutAsset(Map<UUID, UUID> current, UUID deleted) {
        Map<UUID, UUID> next = new LinkedHashMap<>();
        if (current == null || deleted == null) {
            return next;
        }
        for (Map.Entry<UUID, UUID> e : current.entrySet()) {
            if (deleted.equals(e.getKey()) || deleted.equals(e.getValue())) {
                continue;
            }
            next.put(e.getKey(), e.getValue());
        }
        return next;
    }

    public static void assertNoCycleFrom(Map<UUID, UUID> upstreamOf, Collection<UUID> starts) {
        if (starts == null) {
            return;
        }
        for (UUID start : starts) {
            if (start == null) {
                continue;
            }
            Set<UUID> seen = new HashSet<>();
            UUID cursor = start;
            while (cursor != null) {
                if (!seen.add(cursor)) {
                    throw new IllegalArgumentException("update upstream would create a cycle");
                }
                cursor = upstreamOf == null ? null : upstreamOf.get(cursor);
            }
        }
    }

    public record PlanNode(UUID id, List<UUID> blockIds) {}

    public record PlanLayer(List<PlanNode> assets) {}

    /**
     * Layers run leaf to root. {@code blockIds} are the selected ancestors that
     * must be skipped when this asset's update fails. Unselected hops are walked
     * but not placed in a layer.
     */
    public static List<PlanLayer> plan(Collection<UUID> selectedIds, Map<UUID, UUID> upstreamOf) {
        LinkedHashSet<UUID> selected = new LinkedHashSet<>(distinct(selectedIds));
        if (selected.isEmpty()) {
            throw new IllegalArgumentException("assetIds required");
        }
        Map<UUID, Integer> indegree = new HashMap<>();
        Map<UUID, List<UUID>> successors = new HashMap<>();
        Map<UUID, List<UUID>> blockIds = new HashMap<>();
        Set<String> edges = new HashSet<>();
        for (UUID id : selected) {
            indegree.putIfAbsent(id, 0);
            List<UUID> blocks = new ArrayList<>();
            UUID previous = id;
            Set<UUID> seen = new HashSet<>();
            seen.add(id);
            UUID cursor = upstreamOf == null ? null : upstreamOf.get(id);
            while (cursor != null) {
                if (!seen.add(cursor)) {
                    throw new IllegalArgumentException("update upstream would create a cycle");
                }
                if (selected.contains(cursor)) {
                    blocks.add(cursor);
                    if (edges.add(previous + ">" + cursor)) {
                        indegree.merge(cursor, 1, Integer::sum);
                        successors.computeIfAbsent(previous, k -> new ArrayList<>()).add(cursor);
                    }
                    previous = cursor;
                }
                cursor = upstreamOf.get(cursor);
            }
            blockIds.put(id, List.copyOf(blocks));
        }

        List<UUID> ready = new ArrayList<>();
        for (UUID id : selected) {
            if (indegree.getOrDefault(id, 0) == 0) {
                ready.add(id);
            }
        }
        ready.sort(Comparator.comparing(UUID::toString));
        Set<UUID> remaining = new HashSet<>(selected);
        List<PlanLayer> layers = new ArrayList<>();
        while (!ready.isEmpty()) {
            List<PlanNode> nodes = new ArrayList<>();
            for (UUID id : ready) {
                nodes.add(new PlanNode(id, blockIds.getOrDefault(id, List.of())));
                remaining.remove(id);
            }
            layers.add(new PlanLayer(List.copyOf(nodes)));
            List<UUID> next = new ArrayList<>();
            for (UUID id : ready) {
                for (UUID upstream : successors.getOrDefault(id, List.of())) {
                    int left = indegree.merge(upstream, -1, Integer::sum);
                    if (left == 0) {
                        next.add(upstream);
                    }
                }
            }
            next.sort(Comparator.comparing(UUID::toString));
            ready = next;
        }
        if (!remaining.isEmpty()) {
            throw new IllegalArgumentException("update upstream would create a cycle");
        }
        return List.copyOf(layers);
    }

    /**
     * Visible assets on the focus chain (ancestors and descendants), walking
     * through hops the caller cannot see. Empty focus returns every visible id.
     */
    public static Set<UUID> focus(Set<UUID> visible, Map<UUID, UUID> upstreamOf, Collection<UUID> focusIds) {
        Set<UUID> canSee = visible == null ? Set.of() : visible;
        List<UUID> requested = distinct(focusIds);
        if (requested.isEmpty()) {
            return Set.copyOf(canSee);
        }
        Set<UUID> seeds = new HashSet<>();
        for (UUID id : requested) {
            if (canSee.contains(id)) {
                seeds.add(id);
            }
        }
        if (seeds.isEmpty()) {
            return Set.of();
        }
        Set<UUID> out = new HashSet<>(seeds);
        for (UUID id : seeds) {
            walkVisibleAncestors(id, canSee, upstreamOf, out);
        }
        boolean grew = true;
        int guard = canSee.size() + 1;
        while (grew && guard-- > 0) {
            grew = false;
            for (UUID id : canSee) {
                if (out.contains(id)) {
                    continue;
                }
                if (reaches(id, out, upstreamOf)) {
                    out.add(id);
                    walkVisibleAncestors(id, canSee, upstreamOf, out);
                    grew = true;
                }
            }
        }
        return Set.copyOf(out);
    }

    public record TopoEdge(String from, String to) {}

    public record TopoSketch(List<TopoEdge> edges, List<String> hiddenIds, boolean gateway) {}

    /**
     * Edges point downstream → upstream → gateway. An invisible stretch collapses
     * to one anonymous node id derived from the visible asset, never the hidden
     * asset's id.
     */
    public static TopoSketch sketch(Set<UUID> shown, Set<UUID> visible, Map<UUID, UUID> upstreamOf) {
        List<UUID> ids = new ArrayList<>(shown == null ? List.of() : shown);
        ids.sort(Comparator.comparing(UUID::toString));
        List<TopoEdge> edges = new ArrayList<>();
        List<String> hidden = new ArrayList<>();
        boolean gateway = false;
        Set<UUID> canSee = visible == null ? Set.of() : visible;
        for (UUID id : ids) {
            String from = id.toString();
            UUID cursor = upstreamOf == null ? null : upstreamOf.get(id);
            Set<UUID> seen = new HashSet<>();
            seen.add(id);
            boolean crossedHidden = false;
            UUID named = null;
            while (cursor != null && seen.add(cursor)) {
                if (canSee.contains(cursor) && shown.contains(cursor)) {
                    named = cursor;
                    break;
                }
                crossedHidden = true;
                cursor = upstreamOf.get(cursor);
            }
            if (named != null && crossedHidden) {
                String hid = hiddenId(id);
                hidden.add(hid);
                edges.add(new TopoEdge(from, hid));
                edges.add(new TopoEdge(hid, named.toString()));
            } else if (named != null) {
                edges.add(new TopoEdge(from, named.toString()));
            } else if (crossedHidden) {
                String hid = hiddenId(id);
                hidden.add(hid);
                edges.add(new TopoEdge(from, hid));
            } else {
                gateway = true;
                edges.add(new TopoEdge(from, GATEWAY_ID));
            }
        }
        return new TopoSketch(List.copyOf(edges), List.copyOf(hidden), gateway);
    }

    private static void walkVisibleAncestors(UUID start, Set<UUID> visible, Map<UUID, UUID> upstreamOf, Set<UUID> out) {
        UUID cursor = upstreamOf == null ? null : upstreamOf.get(start);
        Set<UUID> seen = new HashSet<>();
        seen.add(start);
        while (cursor != null && seen.add(cursor)) {
            if (visible.contains(cursor)) {
                out.add(cursor);
            }
            cursor = upstreamOf.get(cursor);
        }
    }

    private static boolean reaches(UUID start, Set<UUID> targets, Map<UUID, UUID> upstreamOf) {
        UUID cursor = upstreamOf == null ? null : upstreamOf.get(start);
        Set<UUID> seen = new HashSet<>();
        seen.add(start);
        while (cursor != null && seen.add(cursor)) {
            if (targets.contains(cursor)) {
                return true;
            }
            cursor = upstreamOf.get(cursor);
        }
        return false;
    }

    private static List<UUID> distinct(Collection<UUID> ids) {
        if (ids == null || ids.isEmpty()) {
            return List.of();
        }
        LinkedHashSet<UUID> out = new LinkedHashSet<>();
        for (UUID id : ids) {
            if (id != null) {
                out.add(id);
            }
        }
        return List.copyOf(out);
    }
}
