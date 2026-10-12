package com.ops.control.metrics;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.annotation.Order;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

/**
 * Partial unique index: one open incident per asset+item+instance+rule.
 * {@code rule_id} may be null for offline; COALESCE keeps uniqueness stable.
 */
@Component
@Order(55)
public class AlertIncidentIndexBootstrap implements ApplicationRunner {
    private static final Logger log = LoggerFactory.getLogger(AlertIncidentIndexBootstrap.class);

    private final JdbcTemplate jdbc;

    public AlertIncidentIndexBootstrap(JdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    @Override
    public void run(ApplicationArguments args) {
        try {
            jdbc.execute(
                    """
                    create unique index if not exists uq_alert_incidents_open
                    on alert_incidents (
                      asset_id,
                      item_id,
                      instance,
                      coalesce(rule_id, '00000000-0000-0000-0000-000000000000')
                    )
                    where resolved_at is null
                    """);
        } catch (Exception e) {
            log.warn("alert_incidents open unique index bootstrap failed: {}", e.toString());
        }
    }
}
