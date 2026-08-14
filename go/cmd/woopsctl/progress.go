package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ops-bastion/ops/go/internal/opsctl"
)

// transferProgressPrinter writes human progress to stderr (single-line updates when TTY).
type transferProgressPrinter struct {
	out        io.Writer
	tty        bool
	verb       string // "upload" or "download"
	started    time.Time
	xferStart  time.Time
	baseLoaded int64 // offset at transfer-phase start (resume baseline; excluded from speed)
	lastPrint  time.Time
	lastLine   string
	hadLine    bool
}

func newTransferProgressPrinter(verb string) *transferProgressPrinter {
	tty := false
	if fi, err := os.Stderr.Stat(); err == nil {
		tty = fi.Mode()&os.ModeCharDevice != 0
	}
	return &transferProgressPrinter{
		out:  os.Stderr,
		tty:  tty,
		verb: verb,
	}
}

func (p *transferProgressPrinter) Handle(ev opsctl.TransferProgress) {
	now := time.Now()
	if p.started.IsZero() {
		p.started = now
	}
	switch ev.Phase {
	case opsctl.ProgressTicket:
		p.emitLine(fmt.Sprintf("%s: requesting ticket…", p.verb), false)
	case opsctl.ProgressConnecting:
		p.emitLine(fmt.Sprintf("%s: connecting…", p.verb), false)
	case opsctl.ProgressConnected:
		msg := fmt.Sprintf("%s: connected", p.verb)
		if ev.Resumed {
			msg += fmt.Sprintf(" (resume from %s)", formatBytes(ev.Loaded))
		}
		if ev.Total > 0 {
			msg += fmt.Sprintf(", size %s", formatBytes(ev.Total))
		}
		p.emitLine(msg, false)
		p.xferStart = now
		p.baseLoaded = ev.Loaded
	case opsctl.ProgressRetrying:
		errMsg := ev.Message
		if errMsg == "" {
			errMsg = "transient error"
		}
		p.emitLine(fmt.Sprintf("%s: retrying (%d) — %s", p.verb, ev.Attempt, errMsg), false)
	case opsctl.ProgressTransfer:
		if p.xferStart.IsZero() {
			p.xferStart = now
			p.baseLoaded = ev.Loaded
		}
		// Throttle transfer lines (~4 Hz) unless finished.
		finished := ev.Total > 0 && ev.Loaded >= ev.Total
		if !finished && !p.lastPrint.IsZero() && now.Sub(p.lastPrint) < 250*time.Millisecond {
			return
		}
		p.lastPrint = now
		p.emitLine(p.formatTransfer(ev, now), !finished)
	case opsctl.ProgressDone:
		elapsed := now.Sub(p.started).Truncate(time.Millisecond)
		total := ev.Total
		if total <= 0 {
			total = ev.Loaded
		}
		p.emitLine(fmt.Sprintf("%s: done %s in %s", p.verb, formatBytes(total), formatDuration(elapsed)), false)
	}
}

func (p *transferProgressPrinter) formatTransfer(ev opsctl.TransferProgress, now time.Time) string {
	loaded, total := ev.Loaded, ev.Total
	pct := 0.0
	if total > 0 {
		pct = float64(loaded) * 100 / float64(total)
	}
	elapsed := now.Sub(p.xferStart)
	if elapsed < time.Millisecond {
		elapsed = time.Millisecond
	}
	sent := loaded - p.baseLoaded
	if sent < 0 {
		sent = 0
	}
	bps := float64(sent) / elapsed.Seconds()
	var eta string
	if total > 0 && bps > 0 && loaded < total {
		remain := float64(total-loaded) / bps
		eta = formatDuration(time.Duration(remain * float64(time.Second)))
	} else if total > 0 && loaded >= total {
		eta = "0s"
	} else {
		eta = "--"
	}
	if total > 0 {
		return fmt.Sprintf("%s: %s / %s (%.1f%%)  %s/s  elapsed %s  eta %s",
			p.verb, formatBytes(loaded), formatBytes(total), pct, formatBytes(int64(bps)), formatDuration(elapsed), eta)
	}
	return fmt.Sprintf("%s: %s  %s/s  elapsed %s",
		p.verb, formatBytes(loaded), formatBytes(int64(bps)), formatDuration(elapsed))
}

func (p *transferProgressPrinter) emitLine(line string, overwrite bool) {
	if p.tty && overwrite {
		pad := ""
		if len(p.lastLine) > len(line) {
			pad = strings.Repeat(" ", len(p.lastLine)-len(line))
		}
		fmt.Fprintf(p.out, "\r%s%s", line, pad)
		p.lastLine = line
		p.hadLine = true
		return
	}
	if p.tty && p.hadLine {
		fmt.Fprint(p.out, "\n")
		p.hadLine = false
		p.lastLine = ""
	}
	fmt.Fprintln(p.out, line)
}

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GiB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.2f MiB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.2f KiB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
}
