package monitor

import (
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ops-bastion/ops/go/internal/hostinfo/netiface"
	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

var skipFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "proc": true, "sysfs": true, "cgroup": true,
	"cgroup2": true, "pstore": true, "bpf": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "devpts": true, "hugetlbfs": true, "mqueue": true,
	"overlay": true, "nsfs": true, "squashfs": true, "autofs": true,
	"fusectl": true, "configfs": true, "rpc_pipefs": true, "binfmt_misc": true,
	// Optical / ISO media: read-only discs report ~100% used and trip disk alerts.
	"iso9660": true, "udf": true, "cdfs": true,
}

type rateState struct {
	mu     sync.Mutex
	lastAt time.Time
	diskIO map[string]disk.IOCountersStat
	netIO  map[string]net.IOCountersStat
}

var rates = &rateState{
	diskIO: map[string]disk.IOCountersStat{},
	netIO:  map[string]net.IOCountersStat{},
}

func point(itemID, instance string, value float64) monitorproto.Point {
	if value != value || value > 1e18 || value < -1e18 { // NaN / Inf / absurd
		value = 0
	}
	return monitorproto.Point{ItemID: itemID, Instance: instance, Value: value}
}

// Collect gathers host metrics as flat points (one value per row).
func Collect() []monitorproto.Point {
	now := time.Now()
	var out []monitorproto.Point

	if hi, err := host.Info(); err == nil && hi != nil {
		out = append(out, point("host.uptime_sec", "", float64(hi.Uptime)))
		out = append(out, point("host.boot_time", "", float64(hi.BootTime)))
	}

	logical, _ := cpu.Counts(true)
	physical, _ := cpu.Counts(false)
	out = append(out, point("cpu.count_logical", "", float64(logical)))
	if physical > 0 {
		out = append(out, point("cpu.count_physical", "", float64(physical)))
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		if infos[0].Mhz > 0 {
			out = append(out, point("cpu.mhz", "", infos[0].Mhz))
		}
	}

	// Overall usage only (no per-core series).
	if percents, err := cpu.Percent(250*time.Millisecond, false); err == nil && len(percents) > 0 {
		out = append(out, point("cpu.usage_percent", "", percents[0]))
	}

	if times, err := cpu.Times(false); err == nil && len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
		if total > 0 {
			out = append(out, point("cpu.user_percent", "", 100*t.User/total))
			out = append(out, point("cpu.system_percent", "", 100*t.System/total))
			out = append(out, point("cpu.idle_percent", "", 100*t.Idle/total))
			if runtime.GOOS != "windows" {
				out = append(out, point("cpu.iowait_percent", "", 100*t.Iowait/total))
			}
		}
	}

	if avg, err := load.Avg(); err == nil && avg != nil {
		out = append(out, point("load.load1", "", avg.Load1))
		out = append(out, point("load.load5", "", avg.Load5))
		out = append(out, point("load.load15", "", avg.Load15))
	}

	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		out = append(out, point("mem.total_bytes", "", float64(vm.Total)))
		out = append(out, point("mem.used_bytes", "", float64(vm.Used)))
		out = append(out, point("mem.available_bytes", "", float64(vm.Available)))
		out = append(out, point("mem.used_percent", "", vm.UsedPercent))
	}
	if sm, err := mem.SwapMemory(); err == nil && sm != nil {
		out = append(out, point("swap.total_bytes", "", float64(sm.Total)))
		out = append(out, point("swap.used_bytes", "", float64(sm.Used)))
		out = append(out, point("swap.used_percent", "", sm.UsedPercent))
	}

	if parts, err := disk.Partitions(false); err == nil {
		// Prefer short mount paths so "/" wins over Docker bind mounts of the same device
		// (e.g. /etc/hostname, /etc/hosts - same fs, not real disks).
		sort.Slice(parts, func(i, j int) bool {
			return len(parts[i].Mountpoint) < len(parts[j].Mountpoint)
		})
		seenMount := map[string]bool{}
		seenDevice := map[string]bool{}
		for _, p := range parts {
			fs := strings.ToLower(p.Fstype)
			if skipFS[fs] {
				continue
			}
			mount := p.Mountpoint
			if mount == "" || seenMount[mount] {
				continue
			}
			if isOpticalDrive(mount) {
				continue
			}
			// Bind-mounted files (common in containers) are not disk volumes.
			if fi, err := os.Stat(mount); err != nil || !fi.IsDir() {
				continue
			}
			// One row per block device, like `df` (not `df -a`).
			if p.Device != "" {
				if seenDevice[p.Device] {
					continue
				}
				seenDevice[p.Device] = true
			}
			seenMount[mount] = true
			usage, err := disk.Usage(mount)
			if err != nil || usage == nil || usage.Total == 0 {
				continue
			}
			out = append(out, point("disk.total_bytes", mount, float64(usage.Total)))
			out = append(out, point("disk.used_bytes", mount, float64(usage.Used)))
			out = append(out, point("disk.free_bytes", mount, float64(usage.Free)))
			out = append(out, point("disk.used_percent", mount, usage.UsedPercent))
			if usage.InodesTotal > 0 {
				out = append(out, point("disk.inodes_total", mount, float64(usage.InodesTotal)))
				out = append(out, point("disk.inodes_used", mount, float64(usage.InodesUsed)))
				out = append(out, point("disk.inodes_used_percent", mount, usage.InodesUsedPercent))
			}
		}
	}

	if counters, err := disk.IOCounters(); err == nil {
		rates.mu.Lock()
		elapsed := now.Sub(rates.lastAt).Seconds()
		for name, cur := range counters {
			out = append(out, point("diskio.read_bytes", name, float64(cur.ReadBytes)))
			out = append(out, point("diskio.write_bytes", name, float64(cur.WriteBytes)))
			out = append(out, point("diskio.read_count", name, float64(cur.ReadCount)))
			out = append(out, point("diskio.write_count", name, float64(cur.WriteCount)))
			if prev, ok := rates.diskIO[name]; ok && elapsed > 0 {
				out = append(out, point("diskio.read_bytes_per_sec", name, float64(cur.ReadBytes-prev.ReadBytes)/elapsed))
				out = append(out, point("diskio.write_bytes_per_sec", name, float64(cur.WriteBytes-prev.WriteBytes)/elapsed))
			}
			rates.diskIO[name] = cur
		}
		rates.mu.Unlock()
	}

	if nics, err := net.IOCounters(true); err == nil {
		rates.mu.Lock()
		elapsed := now.Sub(rates.lastAt).Seconds()
		for _, cur := range nics {
			name := cur.Name
			if !netiface.IsReportable(name) {
				continue
			}
			out = append(out, point("net.rx_bytes", name, float64(cur.BytesRecv)))
			out = append(out, point("net.tx_bytes", name, float64(cur.BytesSent)))
			out = append(out, point("net.rx_packets", name, float64(cur.PacketsRecv)))
			out = append(out, point("net.tx_packets", name, float64(cur.PacketsSent)))
			out = append(out, point("net.rx_errors", name, float64(cur.Errin)))
			out = append(out, point("net.tx_errors", name, float64(cur.Errout)))
			out = append(out, point("net.rx_dropped", name, float64(cur.Dropin)))
			out = append(out, point("net.tx_dropped", name, float64(cur.Dropout)))
			if prev, ok := rates.netIO[name]; ok && elapsed > 0 {
				out = append(out, point("net.rx_bytes_per_sec", name, float64(cur.BytesRecv-prev.BytesRecv)/elapsed))
				out = append(out, point("net.tx_bytes_per_sec", name, float64(cur.BytesSent-prev.BytesSent)/elapsed))
				out = append(out, point("net.rx_packets_per_sec", name, float64(cur.PacketsRecv-prev.PacketsRecv)/elapsed))
				out = append(out, point("net.tx_packets_per_sec", name, float64(cur.PacketsSent-prev.PacketsSent)/elapsed))
			}
			rates.netIO[name] = cur
		}
		rates.mu.Unlock()
	}

	rates.mu.Lock()
	rates.lastAt = now
	rates.mu.Unlock()

	if pids, err := process.Pids(); err == nil {
		out = append(out, point("process.count", "", float64(len(pids))))
	}

	return out
}
