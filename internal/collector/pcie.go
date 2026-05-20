package collector

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
	"github.com/sratabix/intel_gpu_exporter/internal/sysutil"
)

// PCIe reads PCIe link state for each Intel GPU device.
//
// Files read (from /sys/bus/pci/devices/<addr>/):
//
//	current_link_speed  - e.g. "16.0 GT/s PCIe" (Gen4)
//	current_link_width  - e.g. "16"
//	max_link_speed      - e.g. "32.0 GT/s PCIe" (Gen5)
//	max_link_width      - e.g. "16"
//
// Useful for spotting GPUs that got renegotiated down to a lower PCIe gen/width
// because of a thermal event, cable issue, or power-state weirdness.
type PCIe struct {
	gpus []discovery.GPU

	curSpeed  *prometheus.Desc
	curWidth  *prometheus.Desc
	maxSpeed  *prometheus.Desc
	maxWidth  *prometheus.Desc
	curGen    *prometheus.Desc
	maxGen    *prometheus.Desc
}

func NewPCIe(gpus []discovery.GPU) *PCIe {
	lbls := CommonLabels()
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(Namespace, "pcie", name), help, lbls, nil)
	}
	return &PCIe{
		gpus:     gpus,
		curSpeed: d("current_link_speed_gtps", "Current PCIe link speed in GT/s."),
		curWidth: d("current_link_width", "Current PCIe link width (lanes)."),
		maxSpeed: d("max_link_speed_gtps", "Maximum supported PCIe link speed in GT/s."),
		maxWidth: d("max_link_width", "Maximum supported PCIe link width (lanes)."),
		curGen:   d("current_generation", "Current PCIe generation (1..6) derived from link speed."),
		maxGen:   d("max_generation", "Maximum supported PCIe generation."),
	}
}

func (c *PCIe) Name() string { return "pcie" }

func (c *PCIe) Available(gpus []discovery.GPU) bool { return len(gpus) > 0 }

func (c *PCIe) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	for _, g := range c.gpus {
		lv := LabelValues(g)
		readSpeed := func(file string) (gtps float64, gen float64, ok bool) {
			s, err := sysutil.ReadString(filepath.Join(g.DevicePath, file))
			if err != nil {
				return 0, 0, false
			}
			return parseLinkSpeed(s)
		}
		if speed, gen, ok := readSpeed("current_link_speed"); ok {
			ch <- prometheus.MustNewConstMetric(c.curSpeed, prometheus.GaugeValue, speed, lv...)
			ch <- prometheus.MustNewConstMetric(c.curGen, prometheus.GaugeValue, gen, lv...)
		}
		if speed, gen, ok := readSpeed("max_link_speed"); ok {
			ch <- prometheus.MustNewConstMetric(c.maxSpeed, prometheus.GaugeValue, speed, lv...)
			ch <- prometheus.MustNewConstMetric(c.maxGen, prometheus.GaugeValue, gen, lv...)
		}
		if v, err := sysutil.ReadFloat64(filepath.Join(g.DevicePath, "current_link_width")); err == nil {
			ch <- prometheus.MustNewConstMetric(c.curWidth, prometheus.GaugeValue, v, lv...)
		}
		if v, err := sysutil.ReadFloat64(filepath.Join(g.DevicePath, "max_link_width")); err == nil {
			ch <- prometheus.MustNewConstMetric(c.maxWidth, prometheus.GaugeValue, v, lv...)
		}
	}
	return nil
}

// parseLinkSpeed extracts the GT/s number from strings like "16.0 GT/s PCIe"
// and returns both the speed and the corresponding PCIe generation.
//
//	2.5  -> Gen1
//	5.0  -> Gen2
//	8.0  -> Gen3
//	16.0 -> Gen4
//	32.0 -> Gen5
//	64.0 -> Gen6
func parseLinkSpeed(s string) (gtps float64, gen float64, ok bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, false
	}
	switch v {
	case 2.5:
		gen = 1
	case 5.0:
		gen = 2
	case 8.0:
		gen = 3
	case 16.0:
		gen = 4
	case 32.0:
		gen = 5
	case 64.0:
		gen = 6
	}
	return v, gen, true
}
