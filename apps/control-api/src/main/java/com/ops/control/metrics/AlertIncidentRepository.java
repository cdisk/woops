package com.ops.control.metrics;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.Collection;
import java.util.List;
import java.util.UUID;

public interface AlertIncidentRepository extends JpaRepository<AlertIncidentEntity, UUID> {
    List<AlertIncidentEntity> findByAssetIdAndResolvedAtIsNull(UUID assetId);

    List<AlertIncidentEntity> findByResolvedAtIsNull();

    List<AlertIncidentEntity> findByRuleIdAndResolvedAtIsNull(UUID ruleId);

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from AlertIncidentEntity i where i.assetId = :assetId")
    int deleteByAssetId(@Param("assetId") UUID assetId);

    @Query("""
            select i from AlertIncidentEntity i
            where i.assetId = :assetId
              and i.itemId = :itemId
              and i.resolvedAt is null
            """)
    List<AlertIncidentEntity> findOpenByAssetAndItem(
            @Param("assetId") UUID assetId, @Param("itemId") String itemId);

    @Query("""
            select i from AlertIncidentEntity i
            where (:hasAsset = false or i.assetId = :assetId)
              and (:openOnly = false or i.resolvedAt is null)
              and (:resolvedOnly = false or i.resolvedAt is not null)
              and i.startedAt >= :from
              and i.startedAt < :to
              and (:assetFilterEmpty = true or i.assetId in :assetIds)
            order by i.startedAt desc
            """)
    Page<AlertIncidentEntity> search(
            @Param("hasAsset") boolean hasAsset,
            @Param("assetId") UUID assetId,
            @Param("openOnly") boolean openOnly,
            @Param("resolvedOnly") boolean resolvedOnly,
            @Param("from") Instant from,
            @Param("to") Instant to,
            @Param("assetFilterEmpty") boolean assetFilterEmpty,
            @Param("assetIds") Collection<UUID> assetIds,
            Pageable pageable);
}
