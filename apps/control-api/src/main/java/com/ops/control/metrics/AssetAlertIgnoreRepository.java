package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.UUID;

public interface AssetAlertIgnoreRepository extends JpaRepository<AssetAlertIgnoreEntity, AssetAlertIgnoreId> {
    List<AssetAlertIgnoreEntity> findAllByOrderByIgnoredAtDesc();

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from AssetAlertIgnoreEntity i where i.assetId = :assetId")
    int deleteByAssetId(@Param("assetId") UUID assetId);
}
