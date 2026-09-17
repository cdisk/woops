package com.ops.control.metrics;

import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

@Component
public class MetricsBootstrap implements ApplicationRunner {
    private final MonitorItemDefRepository items;
    private final MetricAlertRuleRepository rules;

    public MetricsBootstrap(MonitorItemDefRepository items, MetricAlertRuleRepository rules) {
        this.items = items;
        this.rules = rules;
    }

    @Override
    @Transactional
    public void run(ApplicationArguments args) {
        seedItems();
        seedRules();
    }

    private void seedItems() {
        record Def(String id, String name, String unit, boolean chart) {}
        // Seeded in English: the console shows these through its own i18n catalog
        // (`monitorItems.*`, keyed by itemId), so `name` is what API clients and
        // any locale the console does not translate will see. See seedRules() for
        // why existing rows are left alone.
        List<Def> defs = List.of(
                new Def("cpu.usage_percent", "CPU usage", "percent", true),
                new Def("cpu.user_percent", "CPU user", "percent", false),
                new Def("cpu.system_percent", "CPU system", "percent", false),
                new Def("cpu.idle_percent", "CPU idle", "percent", false),
                new Def("cpu.iowait_percent", "CPU iowait", "percent", false),
                new Def("cpu.count_logical", "Logical CPUs", "count", false),
                new Def("cpu.count_physical", "Physical CPUs", "count", false),
                new Def("cpu.mhz", "CPU frequency", "mhz", false),
                new Def("load.load1", "Load 1m", "load", true),
                new Def("load.load5", "Load 5m", "load", false),
                new Def("load.load15", "Load 15m", "load", false),
                new Def("mem.used_percent", "Memory usage", "percent", true),
                new Def("mem.total_bytes", "Memory total", "bytes", false),
                new Def("mem.used_bytes", "Memory used", "bytes", false),
                new Def("mem.available_bytes", "Memory available", "bytes", false),
                new Def("swap.used_percent", "Swap usage", "percent", true),
                new Def("swap.total_bytes", "Swap total", "bytes", false),
                new Def("swap.used_bytes", "Swap used", "bytes", false),
                new Def("disk.used_percent", "Disk usage", "percent", true),
                new Def("disk.total_bytes", "Disk total", "bytes", false),
                new Def("disk.used_bytes", "Disk used", "bytes", false),
                new Def("disk.free_bytes", "Disk free", "bytes", false),
                new Def("disk.inodes_used_percent", "Inode usage", "percent", false),
                new Def("diskio.read_bytes_per_sec", "Disk read B/s", "bytes_per_sec", false),
                new Def("diskio.write_bytes_per_sec", "Disk write B/s", "bytes_per_sec", false),
                new Def("net.rx_bytes_per_sec", "Network in B/s", "bytes_per_sec", true),
                new Def("net.tx_bytes_per_sec", "Network out B/s", "bytes_per_sec", true),
                new Def("net.rx_bytes", "Network in bytes", "bytes", false),
                new Def("net.tx_bytes", "Network out bytes", "bytes", false),
                new Def("net.rx_packets", "Network in packets", "count", false),
                new Def("net.tx_packets", "Network out packets", "count", false),
                new Def("host.uptime_sec", "Uptime", "seconds", false),
                new Def("process.count", "Processes", "count", false)
        );
        for (Def d : defs) {
            if (items.existsById(d.id())) continue;
            MonitorItemDefEntity e = new MonitorItemDefEntity();
            e.setItemId(d.id());
            e.setName(d.name());
            e.setUnit(d.unit());
            e.setChartDefault(d.chart());
            items.save(e);
        }
    }

    /**
     * Seeds the default rules on an empty install only.
     *
     * <p>Rule names are user data — they are shown verbatim in alert labels and the
     * user may rename them — so they are deliberately never localized at render
     * time and never rewritten here. That does mean an install seeded by an older
     * version keeps its original Chinese defaults; rename them in the console (or
     * with SQL) if the deployment is English-facing.
     */
    private void seedRules() {
        if (rules.count() > 0) return;
        saveRule("CPU usage high", "cpu.usage_percent", "", "gt", 90);
        saveRule("Memory usage high", "mem.used_percent", "", "gt", 90);
        saveRule("Swap usage high", "swap.used_percent", "", "gt", 80);
        saveRule("Disk usage high", "disk.used_percent", "", "gt", 90);
    }

    private void saveRule(String name, String itemId, String instance, String op, double threshold) {
        MetricAlertRuleEntity r = new MetricAlertRuleEntity();
        r.setId(UUID.randomUUID());
        r.setName(name);
        r.setItemId(itemId);
        r.setInstance(instance);
        r.setOp(op);
        r.setThreshold(threshold);
        r.setEnabled(true);
        r.setCreatedAt(Instant.now());
        r.setUpdatedAt(Instant.now());
        rules.save(r);
    }
}
