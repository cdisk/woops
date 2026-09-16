package com.ops.control.apitoken;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

public interface ApiTokenRepository extends JpaRepository<ApiTokenEntity, UUID> {
    List<ApiTokenEntity> findByUserIdOrderByCreatedAtDesc(UUID userId);

    @Modifying
    @Query("update ApiTokenEntity t set t.lastUsedAt = :at where t.id = :id and (t.lastUsedAt is null or t.lastUsedAt < :throttleBefore)")
    int touchLastUsedIfStale(@Param("id") UUID id, @Param("at") Instant at, @Param("throttleBefore") Instant throttleBefore);
}
