package collector

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
	"github.com/sratabix/intel_gpu_exporter/internal/levelzero"
)

// LevelZero exposes telemetry that only Level Zero sysman can provide:
//
//   - RAS / ECC counters (correctable, uncorrectable; per category)
//   - per-engine-group active time at sub-engine granularity
//   - memory bandwidth counters (read/write/max)
//   - memory size / free
//   - per-domain frequency state + throttle time accumulators
//   - per-domain energy counters (more authoritative than hwmon for dGPUs)
//   - per-sensor temperatures with named sensor types (gpu/memory/board/...)
//
// On systems without libze_loader installed (most consumer iGPU/dGPU boxes),
// Available() returns false and the collector is silently dropped.
type LevelZero struct {
	log    *slog.Logger
	client *levelzero.Client

	energy        *prometheus.Desc
	energyTS      *prometheus.Desc
	temperature   *prometheus.Desc
	freqActual    *prometheus.Desc
	freqRequest   *prometheus.Desc
	freqTDP       *prometheus.Desc
	freqThrottleNS *prometheus.Desc
	engineActive  *prometheus.Desc
	engineTS      *prometheus.Desc
	rasError      *prometheus.Desc
	memSize       *prometheus.Desc
	memFree       *prometheus.Desc
	memRead       *prometheus.Desc
	memWrite      *prometheus.Desc
	memMaxBW      *prometheus.Desc
	memTS         *prometheus.Desc
}

func NewLevelZero(log *slog.Logger) *LevelZero {
	subdevLabels := []string{"pci", "subdevice"}
	d := func(name, help string, lbls []string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(Namespace, "zes", name), help, lbls, nil)
	}
	return &LevelZero{
		log:            log,
		energy:         d("energy_microjoules_total", "Cumulative energy counter from zesPowerGetEnergyCounter.", append(subdevLabels, "domain")),
		energyTS:       d("energy_timestamp_microseconds", "Timestamp accompanying energy counter (use to derive instantaneous power).", append(subdevLabels, "domain")),
		temperature:    d("temperature_celsius", "Temperature from zesTemperatureGetState.", append(subdevLabels, "sensor")),
		freqActual:     d("frequency_actual_mhz", "Resolved frequency reported by zesFrequencyGetState.", append(subdevLabels, "domain")),
		freqRequest:    d("frequency_request_mhz", "Requested frequency from zesFrequencyGetState.", append(subdevLabels, "domain")),
		freqTDP:        d("frequency_tdp_mhz", "TDP-limited frequency from zesFrequencyGetState.", append(subdevLabels, "domain")),
		freqThrottleNS: d("frequency_throttle_nanoseconds_total", "Cumulative throttle time per frequency domain.", append(subdevLabels, "domain")),
		engineActive:   d("engine_active_nanoseconds_total", "Cumulative active time per engine group (zesEngineGetActivity).", append(subdevLabels, "engine")),
		engineTS:       d("engine_timestamp_microseconds", "Timestamp accompanying engine activity counter.", append(subdevLabels, "engine")),
		rasError:       d("ras_errors_total", "Cumulative RAS error count from zesRasGetState. Labels: type (correctable/uncorrectable), category.", append(subdevLabels, "type", "category")),
		memSize:        d("memory_size_bytes", "Total memory module size.", append(subdevLabels, "module")),
		memFree:        d("memory_free_bytes", "Free memory on the module.", append(subdevLabels, "module")),
		memRead:        d("memory_read_bytes_total", "Cumulative memory read counter.", append(subdevLabels, "module")),
		memWrite:       d("memory_write_bytes_total", "Cumulative memory write counter.", append(subdevLabels, "module")),
		memMaxBW:       d("memory_max_bandwidth_bytes_per_second", "Peak memory bandwidth as reported by the driver.", append(subdevLabels, "module")),
		memTS:          d("memory_timestamp_microseconds", "Timestamp accompanying memory bandwidth counters.", append(subdevLabels, "module")),
	}
}

func (c *LevelZero) Name() string { return "levelzero" }

func (c *LevelZero) Available(gpus []discovery.GPU) bool {
	if !levelzero.Available() {
		c.log.Info("level zero loader not available, levelzero collector disabled",
			"hint", "install intel-level-zero-gpu + libze_loader (apt: intel-level-zero-gpu; dnf: intel-level-zero)")
		return false
	}
	cli, err := levelzero.Open()
	if err != nil {
		c.log.Info("level zero open failed", "err", err)
		return false
	}
	if len(cli.Devices()) == 0 {
		c.log.Info("level zero loaded but enumerated zero devices, disabling")
		return false
	}
	c.client = cli
	for _, dev := range cli.Devices() {
		c.log.Info("level zero device enumerated",
			"pci", dev.PCIBus,
			"power_domains", len(dev.Power),
			"temp_sensors", len(dev.Temp),
			"freq_domains", len(dev.Freq),
			"engine_groups", len(dev.Engine),
			"ras_sets", len(dev.RAS),
			"memory_modules", len(dev.Mem),
		)
	}
	return true
}

func (c *LevelZero) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	if c.client == nil {
		return fmt.Errorf("levelzero client not initialised")
	}
	for _, dev := range c.client.Devices() {
		pci := dev.PCIBus

		for i, p := range dev.Power {
			if e, ts, ok := c.client.Energy(p); ok {
				lbls := []string{pci, subdevLabel(p.OnSubdevice, p.SubdeviceID), strconv.Itoa(i)}
				ch <- prometheus.MustNewConstMetric(c.energy, prometheus.CounterValue, float64(e), lbls...)
				ch <- prometheus.MustNewConstMetric(c.energyTS, prometheus.CounterValue, float64(ts), lbls...)
			}
		}

		for _, t := range dev.Temp {
			if v, ok := c.client.Temperature(t); ok {
				ch <- prometheus.MustNewConstMetric(c.temperature, prometheus.GaugeValue, v,
					pci, subdevLabel(t.OnSubdevice, t.SubdeviceID), t.Sensor)
			}
		}

		for i, f := range dev.Freq {
			st, throttle, ok := c.client.FrequencyState(f)
			if !ok {
				continue
			}
			lbls := []string{pci, subdevLabel(f.OnSubdevice, f.SubdeviceID), strconv.Itoa(i)}
			ch <- prometheus.MustNewConstMetric(c.freqActual, prometheus.GaugeValue, st.ActualMHz(), lbls...)
			ch <- prometheus.MustNewConstMetric(c.freqRequest, prometheus.GaugeValue, st.RequestMHz(), lbls...)
			ch <- prometheus.MustNewConstMetric(c.freqTDP, prometheus.GaugeValue, st.TDPMHz(), lbls...)
			ch <- prometheus.MustNewConstMetric(c.freqThrottleNS, prometheus.CounterValue, float64(throttle), lbls...)
		}

		for _, e := range dev.Engine {
			active, ts, ok := c.client.EngineActivity(e)
			if !ok {
				continue
			}
			lbls := []string{pci, subdevLabel(e.OnSubdevice, e.SubdeviceID), e.Type}
			ch <- prometheus.MustNewConstMetric(c.engineActive, prometheus.CounterValue, float64(active), lbls...)
			ch <- prometheus.MustNewConstMetric(c.engineTS, prometheus.CounterValue, float64(ts), lbls...)
		}

		for _, r := range dev.RAS {
			cats, ok := c.client.RASCategories(r)
			if !ok {
				continue
			}
			for i, n := range cats {
				ch <- prometheus.MustNewConstMetric(c.rasError, prometheus.CounterValue, float64(n),
					pci, subdevLabel(r.OnSubdevice, r.SubdeviceID), r.ErrorType, levelzero.RasCategoryName(i))
			}
		}

		for i, m := range dev.Mem {
			module := fmt.Sprintf("%s%d", m.Type, i)
			lbls := []string{pci, subdevLabel(m.OnSubdevice, m.SubdeviceID), module}
			if size, free, ok := c.client.MemoryState(m); ok {
				ch <- prometheus.MustNewConstMetric(c.memSize, prometheus.GaugeValue, float64(size), lbls...)
				ch <- prometheus.MustNewConstMetric(c.memFree, prometheus.GaugeValue, float64(free), lbls...)
			}
			if read, write, max, ts, ok := c.client.MemoryBandwidth(m); ok {
				ch <- prometheus.MustNewConstMetric(c.memRead, prometheus.CounterValue, float64(read), lbls...)
				ch <- prometheus.MustNewConstMetric(c.memWrite, prometheus.CounterValue, float64(write), lbls...)
				ch <- prometheus.MustNewConstMetric(c.memMaxBW, prometheus.GaugeValue, float64(max), lbls...)
				ch <- prometheus.MustNewConstMetric(c.memTS, prometheus.CounterValue, float64(ts), lbls...)
			}
		}
	}
	return nil
}

func subdevLabel(on bool, id uint32) string {
	if !on {
		return "root"
	}
	return strconv.FormatUint(uint64(id), 10)
}
