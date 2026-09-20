package collector

import (
	"testing"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
)

func TestParseLinkSpeed(t *testing.T) {
	cases := []struct {
		in   string
		gtps float64
		gen  float64
		ok   bool
	}{
		{"2.5 GT/s PCIe", 2.5, 1, true},
		{"5.0 GT/s PCIe", 5, 2, true},
		{"8.0 GT/s PCIe", 8, 3, true},
		{"16.0 GT/s PCIe", 16, 4, true},
		{"32.0 GT/s PCIe", 32, 5, true},
		{"64.0 GT/s PCIe", 64, 6, true},
		{"2.5 GT/s", 2.5, 1, true},
		{"128.0 GT/s PCIe", 128, 0, true},
		{"Unknown", 0, 0, false},
		{"", 0, 0, false},
		{"   ", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gtps, gen, ok := parseLinkSpeed(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if gtps != tc.gtps || gen != tc.gen {
				t.Errorf("got (%v, %v), want (%v, %v)", gtps, gen, tc.gtps, tc.gen)
			}
		})
	}
}

func TestPCIeUpdate(t *testing.T) {
	g := i915GPU(t, nil, map[string]string{
		"current_link_speed": "8.0 GT/s PCIe\n",
		"current_link_width": "4\n",
		"max_link_speed":     "16.0 GT/s PCIe\n",
		"max_link_width":     "16\n",
	})
	assertSamples(t, NewPCIe([]discovery.GPU{g}), []string{
		`intel_gpu_pcie_current_generation{` + i915Labels + `} 3`,
		`intel_gpu_pcie_current_link_speed_gtps{` + i915Labels + `} 8`,
		`intel_gpu_pcie_current_link_width{` + i915Labels + `} 4`,
		`intel_gpu_pcie_max_generation{` + i915Labels + `} 4`,
		`intel_gpu_pcie_max_link_speed_gtps{` + i915Labels + `} 16`,
		`intel_gpu_pcie_max_link_width{` + i915Labels + `} 16`,
	})
}

func TestPCIeUpdateUnknownSpeed(t *testing.T) {
	g := i915GPU(t, nil, map[string]string{
		"current_link_speed": "Unknown\n",
		"current_link_width": "0\n",
	})
	assertSamples(t, NewPCIe([]discovery.GPU{g}), []string{
		`intel_gpu_pcie_current_link_width{` + i915Labels + `} 0`,
	})
}

func TestPCIeUpdateNoFiles(t *testing.T) {
	g := i915GPU(t, nil, nil)
	if got := samples(t, NewPCIe([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples", got)
	}
}

func TestPCIeAvailable(t *testing.T) {
	c := NewPCIe(nil)
	if c.Name() != "pcie" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.Available(nil) {
		t.Error("Available should be false without GPUs")
	}
	if !c.Available([]discovery.GPU{{Card: "card0"}}) {
		t.Error("Available should be true with a GPU present")
	}
}
