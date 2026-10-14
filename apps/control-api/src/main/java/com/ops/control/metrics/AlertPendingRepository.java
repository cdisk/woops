package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.UUID;

public interface AlertPendingRepository extends JpaRepository<AlertPendingEntity, AlertPendingId> {
    List<AlertPendingEntity> findByAssetId(UUID assetId);

    @Query("select distinct p.assetId from AlertPendingEntity p")
    List<UUID> findDistinctAssetIds();

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from AlertPendingEntity p where p.assetId = :assetId")
    int deleteByAssetId(@Param("assetId") UUID assetId);

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from AlertPendingEntity p where p.ruleId = :ruleId")
    int deleteByRuleId(@Param("ruleId") UUID ruleId);

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("""
            delete from AlertPendingEntity p
            where p.assetId = :assetId
              and p.ruleId = :ruleId
              and p.itemId = :itemId
              and p.instance = :instance
            """)
    int deleteOne(
            @Param("assetId") UUID assetId,
            @Param("ruleId") UUID ruleId,
            @Param("itemId") String itemId,
            @Param("instance") String instance);
}
