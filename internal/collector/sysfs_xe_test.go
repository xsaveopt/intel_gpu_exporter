package collector

import (
	"testing"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
)

func TestXeSysfsUpdate(t *testing.T) {
	g := xeGPU(t, 1, 1, nil, map[string]string{
		"tile0/gt0/freq0/cur_freq": "1550\n",
		"tile0/gt0/freq0/act_freq": "1500\n",
		"tile0/gt0/freq0/rp0_freq": "2400\n",
		"tile0/gt0/freq0/rpa_freq": "2100\n",
		"tile0/gt0/freq0/rpn_freq": "300\n",
	})

	assertSamples(t, NewXeSysfs([]discovery.GPU{g}), []string{
		`intel_gpu_xe_frequency_requested_mhz{` + xeLabels + `,tile="0",gt="0"} 1550`,
		`intel_gpu_xe_frequency_actual_mhz{` + xeLabels + `,tile="0",gt="0"} 1500`,
		`intel_gpu_xe_frequency_rp0_mhz{` + xeLabels + `,tile="0",gt="0"} 2400`,
		`intel_gpu_xe_frequency_rpa_mhz{` + xeLabels + `,tile="0",gt="0"} 2100`,
		`intel_gpu_xe_frequency_rpn_mhz{` + xeLabels + `,tile="0",gt="0"} 300`,
	})
}

func TestXeSysfsUpdateThrottleReasons(t *testing.T) {
	g := xeGPU(t, 1, 1, nil, map[string]string{
		"tile0/gt0/freq0/throttle/status":               "1\n",
		"tile0/gt0/freq0/throttle/reason_pl1":           "0\n",
		"tile0/gt0/freq0/throttle/reason_pl2":           "1\n",
		"tile0/gt0/freq0/throttle/reason_pl4":           "0\n",
		"tile0/gt0/freq0/throttle/reason_thermal":       "0\n",
		"tile0/gt0/freq0/throttle/reason_prochot":       "0\n",
		"tile0/gt0/freq0/throttle/reason_ratl":          "0\n",
		"tile0/gt0/freq0/throttle/reason_vr_thermalert": "0\n",
		"tile0/gt0/freq0/throttle/reason_vr_tdc":        "0\n",
	})

	assertSamples(t, NewXeSysfs([]discovery.GPU{g}), []string{
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="status"} 1`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_pl1"} 0`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_pl2"} 1`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_pl4"} 0`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_thermal"} 0`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_prochot"} 0`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_ratl"} 0`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_vr_thermalert"} 0`,
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="reason_vr_tdc"} 0`,
	})
}

func TestXeSysfsUpdateIgnoresUnknownThrottleFiles(t *testing.T) {
	g := xeGPU(t, 1, 1, nil, map[string]string{
		"tile0/gt0/freq0/throttle/status":         "1\n",
		"tile0/gt0/freq0/throttle/reason_unknown": "1\n",
	})
	assertSamples(t, NewXeSysfs([]discovery.GPU{g}), []string{
		`intel_gpu_xe_throttle_reason{` + xeLabels + `,tile="0",gt="0",reason="status"} 1`,
	})
}

func TestXeSysfsUpdateMultipleTilesAndGTs(t *testing.T) {
	g := xeGPU(t, 2, 2, nil, map[string]string{
		"tile0/gt0/freq0/act_freq": "1500\n",
		"tile0/gt1/freq0/act_freq": "1400\n",
		"tile1/gt0/freq0/act_freq": "1300\n",
		"tile1/gt1/freq0/act_freq": "1200\n",
	})

	assertSamples(t, NewXeSysfs([]discovery.GPU{g}), []string{
		`intel_gpu_xe_frequency_actual_mhz{` + xeLabels + `,tile="0",gt="0"} 1500`,
		`intel_gpu_xe_frequency_actual_mhz{` + xeLabels + `,tile="0",gt="1"} 1400`,
		`intel_gpu_xe_frequency_actual_mhz{` + xeLabels + `,tile="1",gt="0"} 1300`,
		`intel_gpu_xe_frequency_actual_mhz{` + xeLabels + `,tile="1",gt="1"} 1200`,
	})
}

func TestXeSysfsUpdateSkipsUnreadableValues(t *testing.T) {
	g := xeGPU(t, 1, 1, nil, map[string]string{
		"tile0/gt0/freq0/act_freq": "not a number\n",
		"tile0/gt0/freq0/cur_freq": "1550\n",
	})
	assertSamples(t, NewXeSysfs([]discovery.GPU{g}), []string{
		`intel_gpu_xe_frequency_requested_mhz{` + xeLabels + `,tile="0",gt="0"} 1550`,
	})
}

func TestXeSysfsUpdateSkipsOtherDrivers(t *testing.T) {
	g := i915GPU(t, nil, map[string]string{"tile0/gt0/freq0/act_freq": "1500\n"})
	if got := samples(t, NewXeSysfs([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples for an i915 device", got)
	}
}

func TestXeSysfsUpdateEmptyTree(t *testing.T) {
	g := xeGPU(t, 1, 1, nil, nil)
	if got := samples(t, NewXeSysfs([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples", got)
	}
}

func TestXeSysfsAvailable(t *testing.T) {
	c := NewXeSysfs(nil)
	if c.Name() != "xe_sysfs" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.Available([]discovery.GPU{{Driver: discovery.DriverI915}}) {
		t.Error("Available should be false without an xe device")
	}
	if !c.Available([]discovery.GPU{{Driver: discovery.DriverXe}}) {
		t.Error("Available should be true with an xe device")
	}
}
