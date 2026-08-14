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
        List<Def> defs = List.of(
                new Def("cpu.usage_percent", "CPU 使用率", "percent", true),
                new Def("cpu.user_percent", "CPU User%", "percent", false),
                new Def("cpu.system_percent", "CPU System%", "percent", false),
                new Def("cpu.idle_percent", "CPU Idle%", "percent", false),
                new Def("cpu.iowait_percent", "CPU Iowait%", "percent", false),
                new Def("cpu.count_logical", "逻辑 CPU 数", "count", false),
                new Def("cpu.count_physical", "物理 CPU 数", "count", false),
                new Def("cpu.mhz", "CPU 频率", "mhz", false),
                new Def("load.load1", "Load 1m", "load", true),
                new Def("load.load5", "Load 5m", "load", false),
                new Def("load.load15", "Load 15m", "load", false),
                new Def("mem.used_percent", "内存使用率", "percent", true),
                new Def("mem.total_bytes", "内存总量", "bytes", false),
                new Def("mem.used_bytes", "内存已用", "bytes", false),
                new Def("mem.available_bytes", "内存可用", "bytes", false),
                new Def("swap.used_percent", "Swap 使用率", "percent", true),
                new Def("swap.total_bytes", "Swap 总量", "bytes", false),
                new Def("swap.used_bytes", "Swap 已用", "bytes", false),
                new Def("disk.used_percent", "磁盘占用率", "percent", true),
                new Def("disk.total_bytes", "磁盘总量", "bytes", false),
                new Def("disk.used_bytes", "磁盘已用", "bytes", false),
                new Def("disk.free_bytes", "磁盘空闲", "bytes", false),
                new Def("disk.inodes_used_percent", "Inode 占用率", "percent", false),
                new Def("diskio.read_bytes_per_sec", "磁盘读 B/s", "bytes_per_sec", false),
                new Def("diskio.write_bytes_per_sec", "磁盘写 B/s", "bytes_per_sec", false),
                new Def("net.rx_bytes_per_sec", "网卡接收 B/s", "bytes_per_sec", true),
                new Def("net.tx_bytes_per_sec", "网卡发送 B/s", "bytes_per_sec", true),
                new Def("net.rx_bytes", "网卡接收字节", "bytes", false),
                new Def("net.tx_bytes", "网卡发送字节", "bytes", false),
                new Def("net.rx_packets", "网卡接收包", "count", false),
                new Def("net.tx_packets", "网卡发送包", "count", false),
                new Def("host.uptime_sec", "运行时长", "seconds", false),
                new Def("process.count", "进程数", "count", false)
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

    private void seedRules() {
        if (rules.count() > 0) return;
        saveRule("CPU 使用率过高", "cpu.usage_percent", "", "gt", 90);
        saveRule("内存使用率过高", "mem.used_percent", "", "gt", 90);
        saveRule("Swap 使用率过高", "swap.used_percent", "", "gt", 80);
        saveRule("磁盘占用率过高", "disk.used_percent", "", "gt", 90);
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
