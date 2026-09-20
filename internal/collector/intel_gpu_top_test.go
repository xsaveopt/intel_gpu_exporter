package collector

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
)

const gputopStream = `[
{
	"period": {"duration": 1000.4},
	"frequency": {"requested": 350.0, "actual": 348.5},
	"interrupts": {"count": 17.0},
	"rc6": {"value": 97.25},
	"power": {"GPU": 0.05, "Package": 2.5},
	"imc-bandwidth": {"reads": 197.5, "writes": 57.25},
	"engines": {
		"Render/3D/0": {"busy": 0.0, "sema": 0.0, "wait": 0.0}
	}
}
,
{
	"period": {"duration": 1000.1},
	"frequency": {"requested": 1300.0, "actual": 1250.0},
	"interrupts": {"count": 4096.0},
	"rc6": {"value": 12.5},
	"power": {"GPU": 8.25, "Package": 15.5},
	"imc-bandwidth": {"reads": 1024.0, "writes": 512.5},
	"engines": {
		"Render/3D/0": {"busy": 82.5, "sema": 1.25, "wait": 0.5},
		"Video/0": {"busy": 3.0, "sema": 0.0, "wait": 0.0},
		"VideoEnhance/0": {"busy": 0.0, "sema": 0.0, "wait": 0.0}
	}
}
`

func newTestGPUTop(t *testing.T, binPath string) *IntelGPUTop {
	t.Helper()
	return NewIntelGPUTop(binPath, slog.New(slog.DiscardHandler))
}

func TestIntelGPUTopConsumeKeepsLatestSample(t *testing.T) {
	c := newTestGPUTop(t, "intel_gpu_top")
	c.consume(strings.NewReader(gputopStream))

	if c.latest == nil {
		t.Fatal("consume did not store a sample")
	}
	if got := c.latest.Period.Duration; got != 1000.1 {
		t.Errorf("stored period duration = %v, want the last sample in the stream", got)
	}
	if c.last.Load() == 0 {
		t.Error("consume did not record a sample timestamp")
	}
}

func TestIntelGPUTopUpdate(t *testing.T) {
	c := newTestGPUTop(t, "intel_gpu_top")
	c.consume(strings.NewReader(gputopStream))

	const src = `source="intel_gpu_top"`
	assertSamples(t, c, []string{
		`intel_gpu_gputop_frequency_requested_mhz{` + src + `} 1300`,
		`intel_gpu_gputop_frequency_actual_mhz{` + src + `} 1250`,
		`intel_gpu_gputop_rc6_ratio{` + src + `} 12.5`,
		`intel_gpu_gputop_imc_bandwidth_read_mibps{` + src + `} 1024`,
		`intel_gpu_gputop_imc_bandwidth_write_mibps{` + src + `} 512.5`,
		`intel_gpu_gputop_interrupts_per_second{` + src + `} 4096`,
		`intel_gpu_gputop_power_watts{` + src + `,rail="GPU"} 8.25`,
		`intel_gpu_gputop_power_watts{` + src + `,rail="Package"} 15.5`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="Render/3D/0",metric="busy"} 82.5`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="Render/3D/0",metric="sema"} 1.25`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="Render/3D/0",metric="wait"} 0.5`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="Video/0",metric="busy"} 3`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="Video/0",metric="sema"} 0`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="Video/0",metric="wait"} 0`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="VideoEnhance/0",metric="busy"} 0`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="VideoEnhance/0",metric="sema"} 0`,
		`intel_gpu_gputop_engine_busy_ratio{` + src + `,engine="VideoEnhance/0",metric="wait"} 0`,
	})
}

func TestIntelGPUTopUpdateEverythingIsAGauge(t *testing.T) {
	c := newTestGPUTop(t, "intel_gpu_top")
	c.consume(strings.NewReader(gputopStream))
	for name, typ := range metricTypes(t, c) {
		if typ != "gauge" {
			t.Errorf("%s type = %q, want gauge", name, typ)
		}
	}
}

func TestIntelGPUTopConsumeTerminatedArray(t *testing.T) {
	c := newTestGPUTop(t, "intel_gpu_top")
	c.consume(strings.NewReader(`[{"frequency":{"requested":800,"actual":790}}]`))
	if c.latest == nil {
		t.Fatal("consume did not store a sample")
	}
	if got := c.latest.Frequency.Actual; got != 790 {
		t.Errorf("actual frequency = %v, want 790", got)
	}
}

func TestIntelGPUTopConsumeRejectsNonArray(t *testing.T) {
	for name, input := range map[string]string{
		"object": `{"frequency":{"actual":1}}`,
		"empty":  ``,
		"junk":   `not json at all`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestGPUTop(t, "intel_gpu_top")
			c.consume(strings.NewReader(input))
			if c.latest != nil {
				t.Errorf("consume stored a sample from %q", input)
			}
		})
	}
}

func TestIntelGPUTopConsumeRejectsUnitKeysInPower(t *testing.T) {
	c := newTestGPUTop(t, "intel_gpu_top")
	c.consume(strings.NewReader(testdataFile(t, "gputop_stream.json")))
	if c.latest != nil {
		t.Fatal("gpuTopSample.Power now tolerates the unit key; update this test and the fixture expectations")
	}
}

func TestIntelGPUTopUpdateWithoutSample(t *testing.T) {
	c := newTestGPUTop(t, "intel_gpu_top")
	ch := make(chan prometheus.Metric, 8)
	err := c.Update(context.Background(), ch)
	if err == nil {
		t.Fatal("Update should fail before the first sample arrives")
	}
	if len(ch) != 0 {
		t.Errorf("Update emitted %d metrics before the first sample", len(ch))
	}
}

func TestIntelGPUTopAvailable(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "intel_gpu_top")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	i915 := []discovery.GPU{{Driver: discovery.DriverI915}}
	xe := []discovery.GPU{{Driver: discovery.DriverXe}}

	c := newTestGPUTop(t, bin)
	if c.Name() != "intel_gpu_top" {
		t.Errorf("Name = %q", c.Name())
	}
	if !c.Available(i915) {
		t.Error("Available should be true with the binary present and an i915 device")
	}
	if c.Available(xe) {
		t.Error("Available should be false without an i915 device")
	}

	missing := newTestGPUTop(t, filepath.Join(t.TempDir(), "absent"))
	if missing.Available(i915) {
		t.Error("Available should be false when the binary is missing")
	}
}

func TestIntelGPUTopStopWithoutStart(t *testing.T) {
	newTestGPUTop(t, "intel_gpu_top").Stop()
}
