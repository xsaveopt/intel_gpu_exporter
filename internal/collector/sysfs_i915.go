package collector

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
	"github.com/sratabix/intel_gpu_exporter/internal/sysutil"
)

// I915Sysfs reads /sys/class/drm/cardN/{gt_*_freq_mhz,gt/gtN/rps_*_freq_mhz}.
//
// Both layouts coexist on modern kernels (>=6.0):
//   - Legacy: cardN/gt_{cur,act,min,max,RP0,RPn}_freq_mhz       (tile 0 only)
//   - Modern: cardN/gt/gtN/rps_{cur,act,min,max,RP0,RPn}_freq_mhz (per-GT)
//
// PVC (now xe-only) and any future multi-GT i915 platform need the per-GT
// layout. We emit a synthetic `gt="0"` label for the legacy files so dashboards
// see a consistent shape.
//
// Reference: drivers/gpu/drm/i915/gt/intel_gt_sysfs_pm.c
type I915Sysfs struct {
	gpus []discovery.GPU

	freqCur *prometheus.Desc
	freqAct *prometheus.Desc
	freqMin *prometheus.Desc
	freqMax *prometheus.Desc
	freqRP0 *prometheus.Desc
	freqRPn *prometheus.Desc
	freqBst *prometheus.Desc
	rc6     *prometheus.Desc
}

func NewI915Sysfs(gpus []discovery.GPU) *I915Sysfs {
	lbls := append(CommonLabels(), "gt")
	d := func(name, help, unit string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(Namespace, "i915", name+"_"+unit), help, lbls, nil)
	}
	return &I915Sysfs{
		gpus:    gpus,
		freqCur: d("frequency_requested", "GuC requested GT frequency.", "mhz"),
		freqAct: d("frequency_actual", "Actual GT frequency reported by hardware.", "mhz"),
		freqMin: d("frequency_min", "Minimum software-allowed GT frequency.", "mhz"),
		freqMax: d("frequency_max", "Maximum software-allowed GT frequency.", "mhz"),
		freqRP0: d("frequency_rp0", "Hardware maximum (RP0) frequency.", "mhz"),
		freqRPn: d("frequency_rpn", "Hardware minimum (RPn) frequency.", "mhz"),
		freqBst: d("frequency_boost", "Boost frequency hint (per-GT only).", "mhz"),
		rc6: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "i915", "rc6_residency_ms"),
			"RC6 residency counter.", CommonLabels(), nil,
		),
	}
}

func (c *I915Sysfs) Name() string { return "i915_sysfs" }

func (c *I915Sysfs) Available(gpus []discovery.GPU) bool {
	for _, g := range gpus {
		if g.Driver == discovery.DriverI915 {
			return true
		}
	}
	return false
}

func (c *I915Sysfs) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	for _, g := range c.gpus {
		if g.Driver != discovery.DriverI915 {
			continue
		}
		base := LabelValues(g)

		// Legacy card-root files (tile 0). Always emit if present.
		legacyMap := map[*prometheus.Desc]string{
			c.freqCur: "gt_cur_freq_mhz",
			c.freqAct: "gt_act_freq_mhz",
			c.freqMin: "gt_min_freq_mhz",
			c.freqMax: "gt_max_freq_mhz",
			c.freqRP0: "gt_RP0_freq_mhz",
			c.freqRPn: "gt_RPn_freq_mhz",
		}
		gts := discovery.I915GTs(g)
		// If the modern per-GT layout exists we let it drive the gt label; the
		// legacy files alias tile 0 and would double-count.
		if len(gts) == 0 {
			for desc, file := range legacyMap {
				if v, err := sysutil.ReadFloat64(filepath.Join(g.DRMPath, file)); err == nil {
					ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v,
						append(base, "0")...)
				}
			}
		}

		// Modern per-GT layout.
		for _, gt := range gts {
			lv := append(append([]string{}, base...), strconv.Itoa(gt.Index))
			emit := func(desc *prometheus.Desc, file string) {
				if v, err := sysutil.ReadFloat64(filepath.Join(gt.Path, file)); err == nil {
					ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
				}
			}
			emit(c.freqCur, "rps_cur_freq_mhz")
			emit(c.freqAct, "rps_act_freq_mhz")
			emit(c.freqMin, "rps_min_freq_mhz")
			emit(c.freqMax, "rps_max_freq_mhz")
			emit(c.freqRP0, "rps_RP0_freq_mhz")
			emit(c.freqRPn, "rps_RPn_freq_mhz")
			emit(c.freqBst, "rps_boost_freq_mhz")
		}

		// RC6 residency lives at <drm>/power/rc6_residency_ms regardless of GT.
		if v, err := sysutil.ReadFloat64(filepath.Join(g.DRMPath, "power", "rc6_residency_ms")); err == nil {
			ch <- prometheus.MustNewConstMetric(c.rc6, prometheus.CounterValue, v, base...)
		}
	}
	return nil
}
