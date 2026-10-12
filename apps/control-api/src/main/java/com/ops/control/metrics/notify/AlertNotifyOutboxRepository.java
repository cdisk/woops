package com.ops.control.metrics.notify;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

public interface AlertNotifyOutboxRepository extends JpaRepository<AlertNotifyOutboxEntity, UUID> {
    boolean existsByDedupeKey(String dedupeKey);

    @Query("""
            select o from AlertNotifyOutboxEntity o
            where o.status in ('PENDING', 'FAILED')
              and o.nextAttemptAt <= :now
            order by o.nextAttemptAt asc
            """)
    List<AlertNotifyOutboxEntity> findDue(@Param("now") Instant now);
}
