package collector

import (
	"context"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
	"github.com/xsaveopt/intel_gpu_exporter/internal/sysutil"
)

type Memory struct {
	gpus []discovery.GPU

	lmemTotal *prometheus.Desc
	vramTotal *prometheus.Desc
}

func NewMemory(gpus []discovery.GPU) *Memory {
	return &Memory{
		gpus: gpus,
		lmemTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "memory", "lmem_total_bytes"),
			"Total local memory exposed by /sys/class/drm/cardN/lmem_total_bytes (discrete cards).",
			CommonLabels(), nil,
		),
		vramTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "memory", "vram_total_bytes"),
			"Physical VRAM size from xe sysfs (tile-level).",
			append(CommonLabels(), "tile"), nil,
		),
	}
}

func (c *Memory) Name() string { return "memory" }

func (c *Memory) Available(gpus []discovery.GPU) bool { return len(gpus) > 0 }

func (c *Memory) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	for _, g := range c.gpus {
		base := LabelValues(g)
		if v, err := sysutil.ReadFloat64(filepath.Join(g.DRMPath, "lmem_total_bytes")); err == nil {
			ch <- prometheus.MustNewConstMetric(c.lmemTotal, prometheus.GaugeValue, v, base...)
		}
		for _, tile := range g.Tiles {
			if tile.Path == "" || tile.Path == g.DevicePath {
				continue
			}
			if v, err := sysutil.ReadFloat64(filepath.Join(tile.Path, "physical_vram_size_bytes")); err == nil {
				ch <- prometheus.MustNewConstMetric(c.vramTotal, prometheus.GaugeValue, v,
					append(base, itoa(tile.Index))...)
			}
		}
	}
	return nil
}
