package collector

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
	"github.com/xsaveopt/intel_gpu_exporter/internal/sysutil"
)

type Engines struct {
	gpus []discovery.GPU

	info      *prometheus.Desc
	heartbeat *prometheus.Desc
	preempt   *prometheus.Desc
	stop      *prometheus.Desc
	timeslice *prometheus.Desc
	maxBusy   *prometheus.Desc
}

func NewEngines(gpus []discovery.GPU) *Engines {
	lbls := append(CommonLabels(), "engine", "class", "instance")
	infoLbls := append(slices.Clone(lbls), "capabilities", "known_capabilities")
	mk := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(Namespace, "engine", name), help, lbls, nil)
	}
	return &Engines{
		gpus:      gpus,
		info:      prometheus.NewDesc(prometheus.BuildFQName(Namespace, "engine", "info"), "Per-engine metadata: constant 1 gauge.", infoLbls, nil),
		heartbeat: mk("heartbeat_interval_ms", "i915 engine heartbeat interval (0 = disabled)."),
		preempt:   mk("preempt_timeout_ms", "i915 engine preempt timeout."),
		stop:      mk("stop_timeout_ms", "i915 engine stop timeout."),
		timeslice: mk("timeslice_duration_ms", "i915 engine timeslice duration."),
		maxBusy:   mk("max_busywait_duration_ns", "i915 engine max busywait duration."),
	}
}

func (c *Engines) Name() string { return "engines" }

func (c *Engines) Available(gpus []discovery.GPU) bool {
	for _, g := range gpus {
		if g.Driver == discovery.DriverI915 {
			return true
		}
	}
	return false
}

func (c *Engines) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	for _, g := range c.gpus {
		if g.Driver != discovery.DriverI915 {
			continue
		}
		engineRoot := filepath.Join(g.DRMPath, "engine")
		entries, err := os.ReadDir(engineRoot)
		if err != nil {
			continue
		}
		base := LabelValues(g)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			engDir := filepath.Join(engineRoot, e.Name())
			name, _ := sysutil.ReadString(filepath.Join(engDir, "name"))
			class, _ := sysutil.ReadString(filepath.Join(engDir, "class"))
			instance, _ := sysutil.ReadString(filepath.Join(engDir, "instance"))
			caps, _ := sysutil.ReadString(filepath.Join(engDir, "capabilities"))
			known, _ := sysutil.ReadString(filepath.Join(engDir, "known_capabilities"))
			if name == "" {
				name = e.Name()
			}

			lbls := append(append([]string{}, base...), name, class, instance)
			ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
				append(lbls, caps, known)...)

			emit := func(desc *prometheus.Desc, file string) {
				v, err := sysutil.ReadFloat64(filepath.Join(engDir, file))
				if err == nil {
					ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lbls...)
				}
			}
			emit(c.heartbeat, "heartbeat_interval_ms")
			emit(c.preempt, "preempt_timeout_ms")
			emit(c.stop, "stop_timeout_ms")
			emit(c.timeslice, "timeslice_duration_ms")
			emit(c.maxBusy, "max_busywait_duration_ns")
		}
	}
	return nil
}
