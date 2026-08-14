package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.UUID;

public interface MonitorLatestRepository extends JpaRepository<MonitorLatestEntity, MonitorLatestId> {
    List<MonitorLatestEntity> findByAssetIdOrderByItemIdAscInstanceAsc(UUID assetId);
}
