package com.ops.control.metrics.notify;

import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.UUID;

public interface AlertNotifyWebhookRepository extends JpaRepository<AlertNotifyWebhookEntity, UUID> {
    List<AlertNotifyWebhookEntity> findAllByOrderByNameAsc();

    List<AlertNotifyWebhookEntity> findByEnabledTrue();
}
