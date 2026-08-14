package com.ops.control.controlaudit;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.Collection;
import java.util.UUID;

public interface ControlAuditEventRepository extends JpaRepository<ControlAuditEventEntity, UUID> {

    /**
     * Pass concrete Instant bounds (never null). Callers should use
     * Instant.EPOCH / far-future when the client omits from/to — PostgreSQL
     * cannot infer types for null Instant bind parameters in {@code :x is null}.
     */
    @Query("""
            select e from ControlAuditEventEntity e
            where (:category is null or e.category = :category)
              and (:assetId is null or e.assetId = :assetId)
              and e.occurredAt >= :from
              and e.occurredAt <= :to
            order by e.occurredAt desc
            """)
    Page<ControlAuditEventEntity> search(
            @Param("category") String category,
            @Param("assetId") UUID assetId,
            @Param("from") Instant from,
            @Param("to") Instant to,
            Pageable pageable);

    /**
     * Scoped visibility: asset events in {@code assetIds}, group events in
     * {@code groupIds}, or AUTH/USER rows when {@code manageUsers} / own user.
     * Pass a non-empty dummy UUID collection when a set is empty so {@code IN}
     * stays valid.
     */
    @Query("""
            select e from ControlAuditEventEntity e
            where (:category is null or e.category = :category)
              and e.occurredAt >= :from
              and e.occurredAt <= :to
              and (
                    (e.assetId is not null and e.assetId in :assetIds)
                 or (e.assetId is null and e.groupId is not null and e.groupId in :groupIds)
                 or (e.assetId is null and e.groupId is null
                     and (:manageUsers = true or e.userId = :userId))
              )
            order by e.occurredAt desc
            """)
    Page<ControlAuditEventEntity> searchScoped(
            @Param("category") String category,
            @Param("from") Instant from,
            @Param("to") Instant to,
            @Param("assetIds") Collection<UUID> assetIds,
            @Param("groupIds") Collection<UUID> groupIds,
            @Param("manageUsers") boolean manageUsers,
            @Param("userId") UUID userId,
            Pageable pageable);
}
