package collector

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
	"github.com/xsaveopt/intel_gpu_exporter/internal/sysutil"
)

type XeSysfs struct {
	gpus []discovery.GPU

	freqCur  *prometheus.Desc
	freqAct  *prometheus.Desc
	freqRP0  *prometheus.Desc
	freqRPa  *prometheus.Desc
	freqRPn  *prometheus.Desc
	throttle *prometheus.Desc
}

func NewXeSysfs(gpus []discovery.GPU) *XeSysfs {
	lbls := append(CommonLabels(), "tile", "gt")
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(Namespace, "xe", name), help, lbls, nil)
	}
	return &XeSysfs{
		gpus:     gpus,
		freqCur:  d("frequency_requested_mhz", "GuC requested GT frequency (cur_freq)."),
		freqAct:  d("frequency_actual_mhz", "Actual GT frequency (act_freq)."),
		freqRP0:  d("frequency_rp0_mhz", "RP0 (maximum) frequency."),
		freqRPa:  d("frequency_rpa_mhz", "RPa (achievable) frequency."),
		freqRPn:  d("frequency_rpn_mhz", "RPn (minimum) frequency."),
		throttle: prometheus.NewDesc(prometheus.BuildFQName(Namespace, "xe", "throttle_reason"), "GT throttle reason flags (1=active).", append(lbls, "reason"), nil),
	}
}

func (c *XeSysfs) Name() string { return "xe_sysfs" }

func (c *XeSysfs) Available(gpus []discovery.GPU) bool {
	for _, g := range gpus {
		if g.Driver == discovery.DriverXe {
			return true
		}
	}
	return false
}

func (c *XeSysfs) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	for _, g := range c.gpus {
		if g.Driver != discovery.DriverXe {
			continue
		}
		base := LabelValues(g)
		for _, tile := range g.Tiles {
			for _, gt := range tile.GTs {
				freqDir := filepath.Join(gt.Path, "freq0")
				lv := append(append([]string{}, base...), strconv.Itoa(tile.Index), strconv.Itoa(gt.Index))

				emit := func(desc *prometheus.Desc, file string) {
					v, err := sysutil.ReadFloat64(filepath.Join(freqDir, file))
					if err == nil {
						ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv...)
					}
				}
				emit(c.freqCur, "cur_freq")
				emit(c.freqAct, "act_freq")
				emit(c.freqRP0, "rp0_freq")
				emit(c.freqRPa, "rpa_freq")
				emit(c.freqRPn, "rpn_freq")

				reasons := []string{
					"status", "reason_pl1", "reason_pl2", "reason_pl4",
					"reason_thermal", "reason_prochot", "reason_ratl",
					"reason_vr_thermalert", "reason_vr_tdc",
				}
				for _, r := range reasons {
					v, err := sysutil.ReadFloat64(filepath.Join(freqDir, "throttle", r))
					if err == nil {
						ch <- prometheus.MustNewConstMetric(c.throttle, prometheus.GaugeValue, v, append(lv, r)...)
					}
				}
			}
		}
	}
	return nil
}
