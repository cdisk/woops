package com.ops.control.ci;

import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.UUID;

public interface DeployTokenRepository extends JpaRepository<DeployTokenEntity, UUID> {
    List<DeployTokenEntity> findByAssetIdOrderByCreatedAtDesc(UUID assetId);
}
