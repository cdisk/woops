package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;

import java.util.List;
import java.util.UUID;

public interface AssetAlertStatusRepository extends JpaRepository<AssetAlertStatusEntity, UUID> {
    long countByFiringTrue();

    @Query("select s from AssetAlertStatusEntity s where s.firing = true order by s.updatedAt desc")
    List<AssetAlertStatusEntity> findFiring();
}
