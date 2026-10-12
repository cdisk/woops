package com.ops.control.metrics.notify;

import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Component
public class AlertNotifyDeliveryJob {
    private final AlertNotifyService notify;

    public AlertNotifyDeliveryJob(AlertNotifyService notify) {
        this.notify = notify;
    }

    @Scheduled(fixedDelayString = "5000", initialDelay = 15_000)
    public void tick() {
        notify.processDue(50);
    }
}
