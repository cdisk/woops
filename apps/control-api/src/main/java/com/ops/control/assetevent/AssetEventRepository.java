package com.ops.control.assetevent;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.Collection;
import java.util.UUID;

public interface AssetEventRepository extends JpaRepository<AssetEventEntity, UUID> {

    /**
     * Pass concrete Instant bounds (never null). Callers should use
     * Instant.EPOCH / far-future when the client omits from/to — PostgreSQL
     * cannot infer types for null Instant bind parameters.
     */
    @Query("""
            select e from AssetEventEntity e
            where (:assetId is null or e.assetId = :assetId)
              and (:category is null or e.category = :category)
              and (:eventType is null or e.eventType = :eventType)
              and e.occurredAt >= :from
              and e.occurredAt <= :to
            order by e.occurredAt desc
            """)
    Page<AssetEventEntity> search(
            @Param("assetId") UUID assetId,
            @Param("category") String category,
            @Param("eventType") String eventType,
            @Param("from") Instant from,
            @Param("to") Instant to,
            Pageable pageable);

    @Query("""
            select e from AssetEventEntity e
            where e.assetId in :assetIds
              and (:category is null or e.category = :category)
              and (:eventType is null or e.eventType = :eventType)
              and e.occurredAt >= :from
              and e.occurredAt <= :to
            order by e.occurredAt desc
            """)
    Page<AssetEventEntity> searchInAssets(
            @Param("assetIds") Collection<UUID> assetIds,
            @Param("category") String category,
            @Param("eventType") String eventType,
            @Param("from") Instant from,
            @Param("to") Instant to,
            Pageable pageable);
}
