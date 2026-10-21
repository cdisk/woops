package com.ops.control.asset;

import org.junit.jupiter.api.Test;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class UpdateTopologyTest {
    private final UUID leaf = UUID.fromString("00000000-0000-0000-0000-000000000159");
    private final UUID mid = UUID.fromString("00000000-0000-0000-0000-000000000160");
    private final UUID root = UUID.fromString("00000000-0000-0000-0000-000000000145");
    private final UUID other = UUID.fromString("00000000-0000-0000-0000-000000000200");
    private final UUID otherRoot = UUID.fromString("00000000-0000-0000-0000-000000000201");

    @Test
    void setAndClearUpstream() {
        Map<UUID, UUID> set = UpdateTopology.withUpstream(Map.of(), List.of(leaf, mid), root);
        assertEquals(root, set.get(leaf));
        assertEquals(root, set.get(mid));

        Map<UUID, UUID> cleared = UpdateTopology.withUpstream(set, List.of(leaf), null);
        assertFalse(cleared.containsKey(leaf));
        assertEquals(root, cleared.get(mid));
    }

    @Test
    void rejectsSelfAndDirectCycle() {
        assertThrows(IllegalArgumentException.class,
                () -> UpdateTopology.withUpstream(Map.of(), List.of(leaf), leaf));
        Map<UUID, UUID> leafToMid = Map.of(leaf, mid);
        assertThrows(IllegalArgumentException.class,
                () -> UpdateTopology.withUpstream(leafToMid, List.of(mid), leaf));
    }

    @Test
    void rejectsIndirectCycle() {
        Map<UUID, UUID> chain = new LinkedHashMap<>();
        chain.put(leaf, mid);
        chain.put(mid, root);
        assertThrows(IllegalArgumentException.class,
                () -> UpdateTopology.withUpstream(chain, List.of(root), leaf));
    }

    @Test
    void deleteClearsReferencesToUpstream() {
        Map<UUID, UUID> chain = new LinkedHashMap<>();
        chain.put(leaf, mid);
        chain.put(mid, root);
        Map<UUID, UUID> left = UpdateTopology.withoutAsset(chain, mid);
        assertEquals(Map.of(), left);
    }

    @Test
    void planOrdersThroughUnselectedMiddle() {
        Map<UUID, UUID> chain = Map.of(leaf, mid, mid, root);
        List<UpdateTopology.PlanLayer> layers = UpdateTopology.plan(List.of(leaf, root), chain);
        assertEquals(List.of(leaf), ids(layers.get(0)));
        assertEquals(List.of(root), ids(layers.get(1)));
        assertEquals(List.of(root), layers.get(0).assets().get(0).blockIds());
    }

    @Test
    void planIsLeafToRootAndKeepsIndependentBranch() {
        Map<UUID, UUID> chain = Map.of(leaf, mid, mid, root, other, otherRoot);
        List<UpdateTopology.PlanLayer> layers = UpdateTopology.plan(List.of(leaf, mid, root, other, otherRoot), chain);
        assertEquals(2, layers.get(0).assets().size());
        assertTrue(ids(layers.get(0)).containsAll(List.of(leaf, other)));
        assertEquals(2, layers.get(1).assets().size());
        assertTrue(ids(layers.get(1)).containsAll(List.of(mid, otherRoot)));
        assertEquals(List.of(root), ids(layers.get(2)));
        assertEquals(List.of(mid, root), layers.get(0).assets().stream()
                .filter(n -> n.id().equals(leaf))
                .findFirst().orElseThrow()
                .blockIds());
        assertFalse(layers.get(0).assets().stream()
                .filter(n -> n.id().equals(other))
                .findFirst().orElseThrow()
                .blockIds().contains(root));
    }

    @Test
    void focusAndSketchHideInvisibleUpstreamButKeepVisibleAncestor() {
        Map<UUID, UUID> chain = Map.of(leaf, mid, mid, root);
        Set<UUID> visible = Set.of(leaf, root);
        Set<UUID> shown = UpdateTopology.focus(visible, chain, List.of(leaf));
        assertEquals(Set.of(leaf, root), shown);

        UpdateTopology.TopoSketch sketch = UpdateTopology.sketch(shown, visible, chain);
        String hidden = UpdateTopology.hiddenId(leaf);
        assertTrue(sketch.hiddenIds().contains(hidden));
        assertTrue(sketch.edges().contains(new UpdateTopology.TopoEdge(leaf.toString(), hidden)));
        assertTrue(sketch.edges().contains(new UpdateTopology.TopoEdge(hidden, root.toString())));
        assertTrue(sketch.edges().contains(new UpdateTopology.TopoEdge(root.toString(), UpdateTopology.GATEWAY_ID)));
        String dumped = sketch.toString();
        assertFalse(dumped.contains(mid.toString()));
    }

    @Test
    void emptyFocusShowsEveryVisibleAsset() {
        Set<UUID> visible = Set.of(leaf, root);
        assertEquals(visible, UpdateTopology.focus(visible, Map.of(leaf, mid), List.of()));
    }

    @Test
    void primaryPrivateIpUsesFirstAddress() {
        assertEquals("10.0.0.9", UpdateTopology.primaryPrivateIp("10.0.0.9, 10.1.0.9"));
        assertEquals("", UpdateTopology.primaryPrivateIp("  "));
    }

    private static List<UUID> ids(UpdateTopology.PlanLayer layer) {
        return layer.assets().stream().map(UpdateTopology.PlanNode::id).toList();
    }
}
