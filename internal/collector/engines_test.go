package collector

import (
	"testing"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
)

func TestEnginesUpdate(t *testing.T) {
	g := i915GPU(t, map[string]string{
		"engine/rcs0/name":                     "rcs0\n",
		"engine/rcs0/class":                    "0\n",
		"engine/rcs0/instance":                 "0\n",
		"engine/rcs0/capabilities":             "\n",
		"engine/rcs0/known_capabilities":       "\n",
		"engine/rcs0/heartbeat_interval_ms":    "2500\n",
		"engine/rcs0/preempt_timeout_ms":       "640\n",
		"engine/rcs0/stop_timeout_ms":          "100\n",
		"engine/rcs0/timeslice_duration_ms":    "1\n",
		"engine/rcs0/max_busywait_duration_ns": "8000\n",
		"engine/vcs0/name":                     "vcs0\n",
		"engine/vcs0/class":                    "2\n",
		"engine/vcs0/instance":                 "0\n",
		"engine/vcs0/capabilities":             "hevc\n",
		"engine/vcs0/known_capabilities":       "hevc sfc\n",
		"engine/vcs0/heartbeat_interval_ms":    "2500\n",
	}, nil)

	assertSamples(t, NewEngines([]discovery.GPU{g}), []string{
		`intel_gpu_engine_heartbeat_interval_ms{` + i915Labels + `,class="0",engine="rcs0",instance="0"} 2500`,
		`intel_gpu_engine_heartbeat_interval_ms{` + i915Labels + `,class="2",engine="vcs0",instance="0"} 2500`,
		`intel_gpu_engine_info{` + i915Labels + `,capabilities="",class="0",engine="rcs0",instance="0",known_capabilities=""} 1`,
		`intel_gpu_engine_info{` + i915Labels + `,capabilities="hevc",class="2",engine="vcs0",instance="0",known_capabilities="hevc sfc"} 1`,
		`intel_gpu_engine_max_busywait_duration_ns{` + i915Labels + `,class="0",engine="rcs0",instance="0"} 8000`,
		`intel_gpu_engine_preempt_timeout_ms{` + i915Labels + `,class="0",engine="rcs0",instance="0"} 640`,
		`intel_gpu_engine_stop_timeout_ms{` + i915Labels + `,class="0",engine="rcs0",instance="0"} 100`,
		`intel_gpu_engine_timeslice_duration_ms{` + i915Labels + `,class="0",engine="rcs0",instance="0"} 1`,
	})
}

func TestEnginesUpdateFallsBackToDirectoryName(t *testing.T) {
	g := i915GPU(t, map[string]string{
		"engine/bcs0/class":    "3\n",
		"engine/bcs0/instance": "0\n",
	}, nil)
	assertSamples(t, NewEngines([]discovery.GPU{g}), []string{
		`intel_gpu_engine_info{` + i915Labels + `,capabilities="",class="3",engine="bcs0",instance="0",known_capabilities=""} 1`,
	})
}

func TestEnginesUpdateWithoutEngineDirectory(t *testing.T) {
	g := i915GPU(t, nil, nil)
	if got := samples(t, NewEngines([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples", got)
	}
}

func TestEnginesUpdateSkipsNonI915(t *testing.T) {
	g := xeGPU(t, 1, 1, map[string]string{"engine/rcs0/class": "0\n"}, nil)
	if got := samples(t, NewEngines([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples for an xe device", got)
	}
}

func TestEnginesAvailable(t *testing.T) {
	c := NewEngines(nil)
	if c.Name() != "engines" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.Available([]discovery.GPU{{Driver: discovery.DriverXe}}) {
		t.Error("Available should be false without an i915 device")
	}
	if !c.Available([]discovery.GPU{{Driver: discovery.DriverI915}}) {
		t.Error("Available should be true with an i915 device")
	}
}
