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

// PMU is a perf_event_open(2)-based collector for the i915 and xe DRM PMUs.
//
// It is fully data-driven: the kernel publishes one entry per device under
// /sys/bus/event_source/devices/. For each such PMU we read:
//
//   - type           - the PERF_TYPE value to put in perf_event_attr.type
//   - events/<name>  - a string like "event=0x01" or "config=0x...,gt=0"
//   - format/<key>   - bit range like "config:0-7"  (tells us how to encode
//     each key=value pair from the events file into the
//     64-bit config word)
//
// At startup we open one perf event fd per discovered event. On each scrape
// we read the cumulative counter and emit a Prometheus metric. Counter
// semantics depend on the event (residencies are nanoseconds, frequencies are
// rolling sums of MHz samples, engine busy is nanoseconds, etc.); we expose
// raw counter values plus a derived rate where it makes sense.
type PMU struct {
	log    *slog.Logger
	events []*pmuEvent
	mu     sync.Mutex

	counter *prometheus.Desc
}

type pmuEvent struct {
	pmu    string // "i915", "xe_0000_00_02_0"
	name   string // "actual-frequency", "rcs0-busy", "gt-c6-residency"
	config uint64
	fd     int
	cpu    int

	// Decoded labels for dashboards (zero values when the event name doesn't
	// match a known shape). See decodeEventName.
	family string // "engine", "frequency", "rc6", "interrupts", "other"
	engine string // e.g. "rcs0", "vcs0"; empty for non-engine events
	kind   string // for engine events: "busy", "sema", "wait"; otherwise empty
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

// Available probes /sys/bus/event_source/devices for known DRM PMUs and tries
// to open at least one event. Permission failure (perf_event_paranoid) is the
// usual reason this returns false.
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
		for evName, encodedConfig := range pm.events {
			fd, err := perfOpen(pm.typeID, encodedConfig, pm.cpu)
			if err != nil {
				skipped++
				p.log.Debug("pmu event open failed",
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

// ---- PMU discovery ----

type pmuDevice struct {
	name   string            // "i915", "xe_0000_00_02_0"
	typeID uint32            // attr.type
	cpu    int               // CPU to bind perf fds to (first entry in cpumask)
	events map[string]uint64 // event name -> encoded config
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
	events, err := loadEvents(filepath.Join(dir, "events"), format)
	if err != nil {
		return pmuDevice{}, err
	}
	cpu := readCpumaskFirst(filepath.Join(dir, "cpumask"))
	return pmuDevice{name: name, typeID: uint32(typeID64), cpu: cpu, events: events}, nil
}

// readCpumaskFirst returns the first CPU listed in the PMU's cpumask file.
// Many uncore-style PMUs (DRM PMUs included) only allow being read on one
// specific CPU; the kernel publishes which one via this file. Falls back to 0.
//
// File format examples: "0", "0-3", "0,4,8".
func readCpumaskFirst(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0
	}
	// Take the first comma-separated chunk, then the first dash-separated part.
	first := strings.SplitN(s, ",", 2)[0]
	first = strings.SplitN(first, "-", 2)[0]
	cpu, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return 0
	}
	return cpu
}

// decodeEventName extracts engine and kind labels from kernel-published event
// names. Pattern reference:
//
//	i915 engine events: <class><instance>-<kind>  e.g. rcs0-busy, vcs0-sema
//	i915 globals:       actual-frequency, requested-frequency, interrupts,
//	                    rc6-residency, software-gt-awake-time
//	xe globals:         engine-active-ticks, engine-total-ticks,
//	                    gt-actual-frequency, gt-c6-residency
//
// We return (family, engine, kind). Unknown events get family="other".
func decodeEventName(name string) (family, engine, kind string) {
	// i915 engine pattern: <class><instance>-<busy|sema|wait>
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

// formatSpec maps a format key (e.g. "event", "gt") to the bit range within
// perf_event_attr.config it occupies. Bit ranges are 0-indexed and inclusive,
// matching the syntax in /sys/bus/event_source/devices/<pmu>/format/<key>.
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
		// content looks like "config:0-7" or "config1:32-39"
		spec := strings.TrimSpace(string(b))
		colon := strings.IndexByte(spec, ':')
		if colon < 0 {
			continue
		}
		field := spec[:colon] // "config" or "config1"
		if field != "config" {
			// We only handle attr.config (64-bit). DRM PMUs don't use config1/2.
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

func loadEvents(dir string, format map[string]formatSpec) (map[string]uint64, error) {
	out := map[string]uint64{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		// skip ".unit" / ".scale" companion files
		if strings.ContainsRune(name, '.') {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		cfg, ok := encodeEventLine(strings.TrimSpace(string(b)), format)
		if !ok {
			continue
		}
		out[name] = cfg
	}
	return out, nil
}

// encodeEventLine parses a perf event description like "event=0x01,gt=0" and
// folds it into a single attr.config value using the PMU's format spec.
// If a referenced format key is missing we drop the event — we only support
// events fully describable through attr.config.
func encodeEventLine(line string, format map[string]formatSpec) (uint64, bool) {
	var cfg uint64
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			return 0, false
		}
		key := strings.TrimSpace(part[:eq])
		valStr := strings.TrimPrefix(strings.TrimSpace(part[eq+1:]), "0x")
		val, err := strconv.ParseUint(valStr, 16, 64)
		if err != nil {
			return 0, false
		}
		spec, ok := format[key]
		if !ok {
			return 0, false
		}
		cfg |= (val & spec.mask) << spec.shift
	}
	return cfg, true
}

// ---- perf_event_open wrappers ----

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
	// pid=-1, cpu pinned to the PMU's cpumask first entry — DRM PMUs are
	// per-device, not per-task, and many only accept one CPU.
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
