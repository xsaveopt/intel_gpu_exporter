package collector

import (
	"testing"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
)

func TestMemoryUpdateDiscreteLmem(t *testing.T) {
	g := i915GPU(t, map[string]string{"lmem_total_bytes": "17179869184\n"}, nil)
	assertSamples(t, NewMemory([]discovery.GPU{g}), []string{
		`intel_gpu_memory_lmem_total_bytes{` + i915Labels + `} 1.7179869184e+10`,
	})
}

func TestMemoryUpdateXeTiles(t *testing.T) {
	g := xeGPU(t, 2, 1, nil, map[string]string{
		"tile0/physical_vram_size_bytes": "8589934592\n",
		"tile1/physical_vram_size_bytes": "8589934592\n",
	})
	assertSamples(t, NewMemory([]discovery.GPU{g}), []string{
		`intel_gpu_memory_vram_total_bytes{` + xeLabels + `,tile="0"} 8.589934592e+09`,
		`intel_gpu_memory_vram_total_bytes{` + xeLabels + `,tile="1"} 8.589934592e+09`,
	})
}

func TestMemoryUpdateSkipsSyntheticTile(t *testing.T) {
	g := i915GPU(t, nil, map[string]string{"physical_vram_size_bytes": "1234\n"})
	if got := samples(t, NewMemory([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples for a tile aliased to the device path", got)
	}
}

func TestMemoryUpdateIntegratedGPU(t *testing.T) {
	g := i915GPU(t, nil, nil)
	if got := samples(t, NewMemory([]discovery.GPU{g})); len(got) != 0 {
		t.Errorf("got %v, want no samples on an integrated GPU", got)
	}
}

func TestMemoryAvailable(t *testing.T) {
	c := NewMemory(nil)
	if c.Name() != "memory" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.Available(nil) {
		t.Error("Available should be false without GPUs")
	}
	if !c.Available([]discovery.GPU{{Card: "card0"}}) {
		t.Error("Available should be true with a GPU present")
	}
}
