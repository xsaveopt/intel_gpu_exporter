package collector

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
)

// Fdinfo collects per-process DRM client usage via /proc/<pid>/fdinfo.
//
// Reference: Documentation/gpu/drm-usage-stats.rst
//
// Keys of interest:
//   drm-driver:    i915 | xe
//   drm-pdev:      0000:00:02.0
//   drm-client-id: <number>
//   drm-engine-<name>:    <ns> ns       (cumulative)
//   drm-total-<region>:   <bytes>
//   drm-resident-<region>: <bytes>
//   drm-shared-<region>:   <bytes>
//
// Cardinality control: a system with hundreds of GPU-using processes (game
// engines, containers) would explode the active series count. We aggregate
// per (driver, pci, pid, comm) and keep only the top-N processes by total
// engine time. The dropped processes are reported via a single counter so the
// fact that we're capping is observable.
type Fdinfo struct {
	procRoot string
	topN     int

	engineTime *prometheus.Desc
	memTotal   *prometheus.Desc
	memRes     *prometheus.Desc
	memShared  *prometheus.Desc
	dropped    *prometheus.Desc
}

// NewFdinfo constructs the collector. topN <= 0 disables capping.
func NewFdinfo(procRoot string, topN int) *Fdinfo {
	engineLabels := []string{"pci", "driver", "pid", "comm", "engine"}
	memLabels := []string{"pci", "driver", "pid", "comm", "region"}
	return &Fdinfo{
		procRoot: procRoot,
		topN:     topN,
		engineTime: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "client", "engine_time_seconds_total"),
			"Cumulative per-client GPU engine time, parsed from drm-engine-* fdinfo keys.",
			engineLabels, nil,
		),
		memTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "client", "memory_total_bytes"),
			"Total memory allocated by the client, per region.",
			memLabels, nil,
		),
		memRes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "client", "memory_resident_bytes"),
			"Resident memory by the client, per region.",
			memLabels, nil,
		),
		memShared: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "client", "memory_shared_bytes"),
			"Shared memory by the client, per region.",
			memLabels, nil,
		),
		dropped: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "client", "dropped_processes"),
			"Number of GPU-using processes whose metrics were dropped due to the --collector.fdinfo.top-n cap.",
			nil, nil,
		),
	}
}

func (c *Fdinfo) Name() string { return "fdinfo" }

func (c *Fdinfo) Available(gpus []discovery.GPU) bool { return len(gpus) > 0 }

type procKey struct{ pid, comm, driver, pci string }

type procData struct {
	engine map[string]uint64 // engine -> ns
	total  map[string]uint64
	res    map[string]uint64
	shared map[string]uint64
}

func newProcData() *procData {
	return &procData{
		engine: map[string]uint64{},
		total:  map[string]uint64{},
		res:    map[string]uint64{},
		shared: map[string]uint64{},
	}
}

func (c *Fdinfo) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	entries, err := os.ReadDir(c.procRoot)
	if err != nil {
		return err
	}

	procs := map[procKey]*procData{}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		comm := readComm(filepath.Join(c.procRoot, pid, "comm"))
		fdinfoDir := filepath.Join(c.procRoot, pid, "fdinfo")
		fds, err := os.ReadDir(fdinfoDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			data := parseFdinfo(filepath.Join(fdinfoDir, fd.Name()))
			driver, ok := data["drm-driver"]
			if !ok {
				continue
			}
			if driver != "i915" && driver != "xe" {
				continue
			}
			key := procKey{pid: pid, comm: comm, driver: driver, pci: data["drm-pdev"]}
			pd, ok := procs[key]
			if !ok {
				pd = newProcData()
				procs[key] = pd
			}
			for k, v := range data {
				switch {
				case strings.HasPrefix(k, "drm-engine-"):
					pd.engine[strings.TrimPrefix(k, "drm-engine-")] += parseNs(v)
				case strings.HasPrefix(k, "drm-total-"):
					pd.total[strings.TrimPrefix(k, "drm-total-")] += parseBytes(v)
				case strings.HasPrefix(k, "drm-resident-"):
					pd.res[strings.TrimPrefix(k, "drm-resident-")] += parseBytes(v)
				case strings.HasPrefix(k, "drm-shared-"):
					pd.shared[strings.TrimPrefix(k, "drm-shared-")] += parseBytes(v)
				}
			}
		}
	}

	keys := topNByActivity(procs, c.topN)
	dropped := 0
	if c.topN > 0 && len(procs) > c.topN {
		dropped = len(procs) - c.topN
	}

	for _, k := range keys {
		pd := procs[k]
		for engine, ns := range pd.engine {
			ch <- prometheus.MustNewConstMetric(c.engineTime, prometheus.CounterValue,
				float64(ns)/1e9, k.pci, k.driver, k.pid, k.comm, engine)
		}
		for region, v := range pd.total {
			ch <- prometheus.MustNewConstMetric(c.memTotal, prometheus.GaugeValue,
				float64(v), k.pci, k.driver, k.pid, k.comm, region)
		}
		for region, v := range pd.res {
			ch <- prometheus.MustNewConstMetric(c.memRes, prometheus.GaugeValue,
				float64(v), k.pci, k.driver, k.pid, k.comm, region)
		}
		for region, v := range pd.shared {
			ch <- prometheus.MustNewConstMetric(c.memShared, prometheus.GaugeValue,
				float64(v), k.pci, k.driver, k.pid, k.comm, region)
		}
	}
	ch <- prometheus.MustNewConstMetric(c.dropped, prometheus.GaugeValue, float64(dropped))
	return nil
}

func topNByActivity(procs map[procKey]*procData, n int) []procKey {
	keys := make([]procKey, 0, len(procs))
	for k := range procs {
		keys = append(keys, k)
	}
	if n <= 0 || len(keys) <= n {
		return keys
	}
	totals := make(map[procKey]uint64, len(procs))
	for k, pd := range procs {
		var t uint64
		for _, v := range pd.engine {
			t += v
		}
		totals[k] = t
	}
	sort.Slice(keys, func(i, j int) bool { return totals[keys[i]] > totals[keys[j]] })
	return keys[:n]
}

func readComm(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func parseFdinfo(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "drm-") {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}
		out[line[:idx]] = strings.TrimSpace(line[idx+1:])
	}
	return out
}

func parseNs(v string) uint64 {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, " ns")
	n, _ := strconv.ParseUint(v, 10, 64)
	return n
}

func parseBytes(v string) uint64 {
	v = strings.TrimSpace(v)
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.ParseUint(parts[0], 10, 64)
	if len(parts) >= 2 {
		switch strings.ToLower(parts[1]) {
		case "kib":
			n *= 1024
		case "mib":
			n *= 1024 * 1024
		case "gib":
			n *= 1024 * 1024 * 1024
		}
	}
	return n
}
