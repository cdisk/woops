package com.ops.control.asset;

import com.ops.control.access.AccessService;
import com.ops.control.access.ForbiddenException;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.GroupService;
import com.ops.control.metrics.AlertIncidentRepository;
import com.ops.control.metrics.AlertPendingRepository;
import com.ops.control.metrics.AssetAlertIgnoreRepository;
import com.ops.control.metrics.AssetAlertStatusRepository;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserScopeRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.nullable;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AssetUpdateUpstreamTest {
    @Mock AssetRepository assets;
    @Mock GroupService groups;
    @Mock AccessService access;
    @Mock ControlAuditService audit;
    @Mock UserScopeRepository userScopes;
    @Mock AssetAlertIgnoreRepository alertIgnores;
    @Mock AssetAlertStatusRepository alertStatus;
    @Mock AlertIncidentRepository alertIncidents;
    @Mock AlertPendingRepository alertPendings;

    private AssetService service;
    private UserEntity user;
    private AssetEntity leaf;
    private AssetEntity mid;
    private AssetEntity root;

    @BeforeEach
    void setUp() {
        service = new AssetService(
                assets, groups, access, audit, userScopes,
                alertIgnores, alertStatus, alertIncidents, alertPendings);
        user = new UserEntity();
        user.setId(UUID.randomUUID());
        user.setUsername("ops");
        leaf = asset("159");
        mid = asset("160");
        root = asset("145");
    }

    @Test
    void setsAndClearsUpstream() {
        when(assets.findAll()).thenReturn(List.of(leaf, mid, root));
        service.setUpdateUpstream(user, new AssetService.UpdateUpstreamRequest(List.of(leaf.getId()), root.getId(), false));
        assertEquals(root.getId(), leaf.getUpdateUpstreamAssetId());

        service.setUpdateUpstream(user, new AssetService.UpdateUpstreamRequest(List.of(leaf.getId()), null, true));
        assertNull(leaf.getUpdateUpstreamAssetId());
        verify(audit, org.mockito.Mockito.atLeast(2)).record(
                any(), any(), any(), any(), any(), nullable(UUID.class), nullable(UUID.class), any());
    }

    @Test
    void rejectsDirectAndIndirectCycles() {
        leaf.setUpdateUpstreamAssetId(mid.getId());
        when(assets.findAll()).thenReturn(List.of(leaf, mid, root));
        assertThrows(IllegalArgumentException.class, () -> service.setUpdateUpstream(
                user, new AssetService.UpdateUpstreamRequest(List.of(mid.getId()), leaf.getId(), false)));

        mid.setUpdateUpstreamAssetId(root.getId());
        assertThrows(IllegalArgumentException.class, () -> service.setUpdateUpstream(
                user, new AssetService.UpdateUpstreamRequest(List.of(root.getId()), leaf.getId(), false)));
        verify(assets, never()).save(any());
    }

    @Test
    void rejectsUpstreamOutsideScope() {
        when(assets.findAll()).thenReturn(List.of(leaf, mid, root));
        doAnswer(inv -> {
            AssetEntity asset = inv.getArgument(1);
            if (mid.getId().equals(asset.getId())) {
                throw new ForbiddenException("asset not in scope");
            }
            return null;
        }).when(access).assertCanAccessAsset(any(), any());

        assertThrows(ForbiddenException.class, () -> service.setUpdateUpstream(
                user, new AssetService.UpdateUpstreamRequest(List.of(leaf.getId()), mid.getId(), false)));
        verify(assets, never()).save(any());
        assertNull(leaf.getUpdateUpstreamAssetId());
    }

    @Test
    void planWalksUnselectedMiddle() {
        leaf.setUpdateUpstreamAssetId(mid.getId());
        mid.setUpdateUpstreamAssetId(root.getId());
        when(assets.findAll()).thenReturn(List.of(leaf, mid, root));

        Map<String, Object> plan = service.updatePlan(user, List.of(leaf.getId(), root.getId()));
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> layers = (List<Map<String, Object>>) plan.get("layers");
        assertEquals(2, layers.size());
        assertEquals(leaf.getId().toString(), firstId(layers.get(0)));
        assertEquals(root.getId().toString(), firstId(layers.get(1)));
    }

    @Test
    void topologyHidesInvisibleUpstream() {
        leaf.setUpdateUpstreamAssetId(mid.getId());
        mid.setUpdateUpstreamAssetId(root.getId());
        mid.setPrivateIp("10.9.9.9");
        mid.setDisplayName("secret-hop");
        when(assets.findAll()).thenReturn(List.of(leaf, mid, root));
        when(access.canAccessAsset(user, leaf)).thenReturn(true);
        when(access.canAccessAsset(user, root)).thenReturn(true);
        when(access.canAccessAsset(user, mid)).thenReturn(false);

        Map<String, Object> topo = service.updateTopology(user, List.of(leaf.getId()));
        String dumped = topo.toString();
        assertFalse(dumped.contains(mid.getId().toString()));
        assertFalse(dumped.contains("secret-hop"));
        assertFalse(dumped.contains("10.9.9.9"));
        assertTrue(dumped.contains(root.getId().toString()));
        assertTrue(dumped.contains(UpdateTopology.hiddenId(leaf.getId())));
    }

    @Test
    void deleteClearsUpstreamReferences() {
        when(assets.findById(mid.getId())).thenReturn(Optional.of(mid));
        service.delete(user, mid.getId());
        verify(assets).clearUpdateUpstream(mid.getId());
        verify(assets).delete(mid);
    }

    private static String firstId(Map<String, Object> layer) {
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> assets = (List<Map<String, Object>>) layer.get("assets");
        return String.valueOf(assets.get(0).get("id"));
    }

    private static AssetEntity asset(String name) {
        AssetEntity asset = new AssetEntity();
        asset.setId(UUID.randomUUID());
        asset.setDisplayName(name);
        return asset;
    }
}
