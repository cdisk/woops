package com.ops.control.asset;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.Collection;
import java.util.List;
import java.util.UUID;

public interface AssetRepository extends JpaRepository<AssetEntity, UUID> {
    long countByOnlineTrue();

    List<AssetEntity> findByGroupIdOrderByDisplayNameAscHostnameAsc(UUID groupId);

    List<AssetEntity> findByGroupIdInOrderByDisplayNameAscHostnameAsc(Collection<UUID> groupIds);

    List<AssetEntity> findByGroupIdIsNullOrderByDisplayNameAscHostnameAsc();

    List<AssetEntity> findAllByOrderByDisplayNameAscHostnameAsc();

    @Modifying
    @Query("update AssetEntity a set a.groupId = null where a.groupId = :groupId")
    int clearGroupId(@Param("groupId") UUID groupId);
}
