package com.ops.control.portmap;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.UUID;

public interface PortMappingRepository extends JpaRepository<PortMappingEntity, UUID> {
    List<PortMappingEntity> findByRemovedAtIsNullOrderByCreatedAtDesc();

    List<PortMappingEntity> findByAssetIdAndRemovedAtIsNullOrderByCreatedAtDesc(UUID assetId);

    List<PortMappingEntity> findByAssetIdOrderByCreatedAtDesc(UUID assetId);

    /** Legacy null direction counts as forward (Gateway listen). */
    @Query("""
            select m.listenPort from PortMappingEntity m
            where m.removedAt is null
              and (m.direction is null or m.direction = '' or m.direction = 'gateway_to_asset')
            """)
    List<Integer> findActiveForwardListenPorts();

    @Query("""
            select m.listenPort from PortMappingEntity m
            where m.removedAt is null
              and m.assetId = :assetId
              and lower(m.protocol) = lower(:protocol)
              and m.direction = 'asset_to_gateway'
            """)
    List<Integer> findActiveReverseListenPorts(@Param("assetId") UUID assetId, @Param("protocol") String protocol);

    /** All active listen ports (any direction) — kept for callers that need a broad set. */
    @Query("select m.listenPort from PortMappingEntity m where m.removedAt is null")
    List<Integer> findActiveListenPorts();
}
