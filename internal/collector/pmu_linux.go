//go:build linux

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
)

type PMU struct {
	log    *slog.Logger
	events []*pmuEvent
	mu     sync.Mutex

	counter *prometheus.Desc
}

type pmuEvent struct {
	pmu    string
	name   string
	config uint64
	fd     int
	cpu    int

	family string
	engine string
	kind   string
}

func NewPMU(log *slog.Logger) *PMU {
	return &PMU{
		log: log,
		counter: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "pmu", "counter"),
			"Raw counter value read from a DRM PMU event via perf_event_open. "+
				"Semantics vary per event: residency/busy events are nanoseconds, "+
				"frequency events accumulate MHz samples, interrupts is a count.",
			[]string{"pmu", "event", "family", "engine", "kind"}, nil,
		),
	}
}

func (p *PMU) Name() string { return "pmu" }

func (p *PMU) Available(gpus []discovery.GPU) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) > 0 {
		return true
	}
	if err := p.open(); err != nil {
		p.log.Info("pmu collector unavailable", "err", err,
			"hint", "check /proc/sys/kernel/perf_event_paranoid (<= 1 needed) and CAP_PERFMON")
		return false
	}
	return len(p.events) > 0
}

func (p *PMU) open() error {
	pmus, err := discoverPMUs()
	if err != nil {
		return err
	}
	if len(pmus) == 0 {
		return errors.New("no i915/xe PMU found under /sys/bus/event_source/devices")
	}
	var opened, skipped int
	for _, pm := range pmus {
		p.log.Info("pmu discovered",
			"pmu", pm.name, "type", pm.typeID, "cpu", pm.cpu,
			"events_encoded", len(pm.events), "events_dropped", len(pm.droppedEvents),
			"format_keys", pm.formatKeys)
		for _, d := range pm.droppedEvents {
			p.log.Warn("pmu event dropped during encode",
				"pmu", pm.name, "event", d.name, "raw", d.raw, "reason", d.reason)
		}
		for evName, encodedConfig := range pm.events {
			fd, err := perfOpen(pm.typeID, encodedConfig, pm.cpu)
			if err != nil {
				skipped++
				p.log.Warn("pmu event open failed",
					"pmu", pm.name, "event", evName, "cpu", pm.cpu, "err", err)
				continue
			}
			family, engine, kind := decodeEventName(evName)
			p.events = append(p.events, &pmuEvent{
				pmu:    pm.name,
				name:   evName,
				config: encodedConfig,
				fd:     fd,
				cpu:    pm.cpu,
				family: family,
				engine: engine,
				kind:   kind,
			})
			opened++
		}
	}
	p.log.Info("pmu events opened", "opened", opened, "skipped", skipped)
	if opened == 0 {
		return errors.New("opened no PMU events (perf_event_paranoid too restrictive?)")
	}
	return nil
}

func (p *PMU) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	p.mu.Lock()
	events := p.events
	p.mu.Unlock()
	if len(events) == 0 {
		return errors.New("no PMU events configured")
	}
	var firstErr error
	for _, ev := range events {
		v, err := perfRead(ev.fd)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("read %s/%s: %w", ev.pmu, ev.name, err)
			}
			continue
		}
		ch <- prometheus.MustNewConstMetric(p.counter, prometheus.CounterValue,
			float64(v), ev.pmu, ev.name, ev.family, ev.engine, ev.kind)
	}
	return firstErr
}

func (p *PMU) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ev := range p.events {
		_ = unix.Close(ev.fd)
	}
	p.events = nil
	return nil
}

type pmuDevice struct {
	name          string
	typeID        uint32
	cpu           int
	events        map[string]uint64
	droppedEvents []droppedEvent
	formatKeys    []string
}

type droppedEvent struct {
	name, raw, reason string
}

func discoverPMUs() ([]pmuDevice, error) {
	root := "/sys/bus/event_source/devices"
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var pmus []pmuDevice
	for _, e := range entries {
		name := e.Name()
		if name != "i915" && !strings.HasPrefix(name, "i915_") && !strings.HasPrefix(name, "xe_") {
			continue
		}
		dev, err := loadPMU(filepath.Join(root, name), name)
		if err != nil {
			continue
		}
		pmus = append(pmus, dev)
	}
	return pmus, nil
}

func loadPMU(dir, name string) (pmuDevice, error) {
	typeBytes, err := os.ReadFile(filepath.Join(dir, "type"))
	if err != nil {
		return pmuDevice{}, err
	}
	typeID64, err := strconv.ParseUint(strings.TrimSpace(string(typeBytes)), 10, 32)
	if err != nil {
		return pmuDevice{}, err
	}
	format, err := loadFormats(filepath.Join(dir, "format"))
	if err != nil {
		return pmuDevice{}, err
	}
	formatKeys := make([]string, 0, len(format))
	for k := range format {
		formatKeys = append(formatKeys, k)
	}
	events, dropped, err := loadEvents(filepath.Join(dir, "events"), format)
	if err != nil {
		return pmuDevice{}, err
	}
	cpu := readCpumaskFirst(filepath.Join(dir, "cpumask"))
	return pmuDevice{
		name: name, typeID: uint32(typeID64), cpu: cpu,
		events: events, droppedEvents: dropped, formatKeys: formatKeys,
	}, nil
}

func readCpumaskFirst(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0
	}

	first := strings.SplitN(s, ",", 2)[0]
	first = strings.SplitN(first, "-", 2)[0]
	cpu, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return 0
	}
	return cpu
}

func decodeEventName(name string) (family, engine, kind string) {

	for _, suffix := range []string{"-busy", "-sema", "-wait"} {
		if strings.HasSuffix(name, suffix) {
			return "engine", strings.TrimSuffix(name, suffix), strings.TrimPrefix(suffix, "-")
		}
	}
	switch name {
	case "actual-frequency", "requested-frequency", "gt-actual-frequency":
		return "frequency", "", strings.TrimPrefix(name, "gt-")
	case "rc6-residency", "gt-c6-residency":
		return "rc6", "", ""
	case "interrupts":
		return "interrupts", "", ""
	case "software-gt-awake-time":
		return "awake", "", ""
	case "engine-active-ticks":
		return "engine", "", "active"
	case "engine-total-ticks":
		return "engine", "", "total"
	}
	return "other", "", ""
}

type formatSpec struct {
	shift uint
	mask  uint64
}

func loadFormats(dir string) (map[string]formatSpec, error) {
	out := map[string]formatSpec{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		spec := strings.TrimSpace(string(b))
		colon := strings.IndexByte(spec, ':')
		if colon < 0 {
			continue
		}
		field := spec[:colon]
		if field != "config" {

			continue
		}
		rng := spec[colon+1:]
		lo, hi := parseBitRange(rng)
		out[e.Name()] = formatSpec{
			shift: lo,
			mask:  bitMask(hi - lo + 1),
		}
	}
	return out, nil
}

func parseBitRange(s string) (lo, hi uint) {
	if dash := strings.IndexByte(s, '-'); dash >= 0 {
		l, _ := strconv.ParseUint(s[:dash], 10, 8)
		h, _ := strconv.ParseUint(s[dash+1:], 10, 8)
		return uint(l), uint(h)
	}
	l, _ := strconv.ParseUint(s, 10, 8)
	return uint(l), uint(l)
}

func bitMask(width uint) uint64 {
	if width >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << width) - 1
}

func loadEvents(dir string, format map[string]formatSpec) (map[string]uint64, []droppedEvent, error) {
	out := map[string]uint64{}
	var dropped []droppedEvent
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil, nil
		}
		return nil, nil, err
	}
	for _, e := range entries {
		name := e.Name()

		if strings.ContainsRune(name, '.') {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(b))
		cfg, reason := encodeEventLine(raw, format)
		if reason != "" {
			dropped = append(dropped, droppedEvent{name: name, raw: raw, reason: reason})
			continue
		}
		out[name] = cfg
	}
	return out, dropped, nil
}

func encodeEventLine(line string, format map[string]formatSpec) (uint64, string) {
	var cfg uint64
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			return 0, fmt.Sprintf("malformed token %q (no '=')", part)
		}
		key := strings.TrimSpace(part[:eq])
		valStr := strings.TrimPrefix(strings.TrimSpace(part[eq+1:]), "0x")
		val, err := strconv.ParseUint(valStr, 16, 64)
		if err != nil {
			return 0, fmt.Sprintf("bad hex value for %q: %v", key, err)
		}

		if key == "config" {
			cfg |= val
			continue
		}
		if key == "config1" || key == "config2" {
			return 0, fmt.Sprintf("event uses %s (perf attr.%s) — not supported yet", key, key)
		}
		spec, ok := format[key]
		if !ok {
			return 0, fmt.Sprintf("unknown format key %q (kernel exposed it but we don't have a config bit-range for it)", key)
		}
		cfg |= (val & spec.mask) << spec.shift
	}
	return cfg, ""
}

func perfOpen(typeID uint32, config uint64, cpu int) (int, error) {
	attr := unix.PerfEventAttr{
		Type:        typeID,
		Size:        unix.PERF_ATTR_SIZE_VER0,
		Config:      config,
		Sample:      0,
		Sample_type: 0,
		Read_format: 0,
		Bits:        unix.PerfBitDisabled | unix.PerfBitExcludeHv,
	}

	fd, err := unix.PerfEventOpen(&attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return -1, err
	}
	if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_RESET, 0); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("reset: %w", err)
	}
	if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_ENABLE, 0); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("enable: %w", err)
	}
	return fd, nil
}

func perfRead(fd int) (uint64, error) {
	var buf [8]byte
	n, err := unix.Read(fd, buf[:])
	if err != nil {
		return 0, err
	}
	if n != 8 {
		return 0, fmt.Errorf("short read: %d", n)
	}
	return uint64(buf[0]) | uint64(buf[1])<<8 | uint64(buf[2])<<16 | uint64(buf[3])<<24 |
		uint64(buf[4])<<32 | uint64(buf[5])<<40 | uint64(buf[6])<<48 | uint64(buf[7])<<56, nil
}
