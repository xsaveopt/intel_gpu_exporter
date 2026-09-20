package collector

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/xsaveopt/intel_gpu_exporter/internal/discovery"
)

type sourceCollector struct {
	t *testing.T
	s Source
}

func (c *sourceCollector) Describe(chan<- *prometheus.Desc) {}

func (c *sourceCollector) Collect(ch chan<- prometheus.Metric) {
	if err := c.s.Update(context.Background(), ch); err != nil {
		c.t.Errorf("%s: Update: %v", c.s.Name(), err)
	}
}

func gather(t *testing.T, s Source) []*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(&sourceCollector{t: t, s: s})
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("%s: gather: %v", s.Name(), err)
	}
	return mfs
}

func samples(t *testing.T, s Source) []string {
	t.Helper()
	var out []string
	for _, mf := range gather(t, s) {
		for _, m := range mf.GetMetric() {
			out = append(out, mf.GetName()+labelString(m)+" "+valueString(m))
		}
	}
	slices.Sort(out)
	return out
}

func metricNames(t *testing.T, s Source) []string {
	t.Helper()
	var out []string
	for _, mf := range gather(t, s) {
		out = append(out, mf.GetName())
	}
	slices.Sort(out)
	return out
}

func metricTypes(t *testing.T, s Source) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, mf := range gather(t, s) {
		out[mf.GetName()] = strings.ToLower(mf.GetType().String())
	}
	return out
}

func labelString(m *dto.Metric) string {
	if len(m.GetLabel()) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		parts = append(parts, lp.GetName()+"="+strconv.Quote(lp.GetValue()))
	}
	slices.Sort(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

func valueString(m *dto.Metric) string {
	var v float64
	switch {
	case m.GetGauge() != nil:
		v = m.GetGauge().GetValue()
	case m.GetCounter() != nil:
		v = m.GetCounter().GetValue()
	case m.GetUntyped() != nil:
		v = m.GetUntyped().GetValue()
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func assertSamples(t *testing.T, s Source, want []string) {
	t.Helper()
	got := samples(t, s)
	sorted := make([]string, 0, len(want))
	for _, w := range want {
		sorted = append(sorted, canonicalSample(w))
	}
	slices.Sort(sorted)
	if len(got) != len(sorted) {
		t.Fatalf("%s: got %d samples, want %d\ngot:  %s\nwant: %s",
			s.Name(), len(got), len(sorted), strings.Join(got, "\n      "), strings.Join(sorted, "\n      "))
	}
	for i := range got {
		if got[i] != sorted[i] {
			t.Errorf("%s: sample %d\ngot  %s\nwant %s", s.Name(), i, got[i], sorted[i])
		}
	}
}

func canonicalSample(s string) string {
	open := strings.IndexByte(s, '{')
	closing := strings.LastIndexByte(s, '}')
	if open < 0 || closing < open {
		return s
	}
	labels := strings.Split(s[open+1:closing], ",")
	slices.Sort(labels)
	return s[:open+1] + strings.Join(labels, ",") + s[closing:]
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		mkdirAll(t, filepath.Dir(path))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func testdataFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return string(b)
}

func i915GPU(t *testing.T, drmFiles, devFiles map[string]string) discovery.GPU {
	t.Helper()
	root := t.TempDir()
	drmPath := filepath.Join(root, "class", "drm", "card0")
	devPath := filepath.Join(root, "devices", "pci0000:00", "0000:00:02.0")
	mkdirAll(t, drmPath)
	mkdirAll(t, devPath)
	writeFiles(t, drmPath, drmFiles)
	writeFiles(t, devPath, devFiles)
	return discovery.GPU{
		Card:       "card0",
		DRMPath:    drmPath,
		DevicePath: devPath,
		PCIAddr:    "0000:00:02.0",
		DeviceID:   "0x9a49",
		Driver:     discovery.DriverI915,
		Tiles:      []discovery.Tile{{Index: 0, GTs: []discovery.GT{{Index: 0, Path: devPath}}}},
	}
}

func xeGPU(t *testing.T, tiles, gtsPerTile int, drmFiles, devFiles map[string]string) discovery.GPU {
	t.Helper()
	root := t.TempDir()
	drmPath := filepath.Join(root, "class", "drm", "card1")
	devPath := filepath.Join(root, "devices", "pci0000:00", "0000:03:00.0")
	mkdirAll(t, drmPath)
	mkdirAll(t, devPath)
	writeFiles(t, drmPath, drmFiles)
	writeFiles(t, devPath, devFiles)

	g := discovery.GPU{
		Card:       "card1",
		DRMPath:    drmPath,
		DevicePath: devPath,
		PCIAddr:    "0000:03:00.0",
		DeviceID:   "0x56a0",
		Driver:     discovery.DriverXe,
	}
	for ti := range tiles {
		tilePath := filepath.Join(devPath, "tile"+strconv.Itoa(ti))
		tile := discovery.Tile{Index: ti, Path: tilePath}
		for gi := range gtsPerTile {
			tile.GTs = append(tile.GTs, discovery.GT{Index: gi, Path: filepath.Join(tilePath, "gt"+strconv.Itoa(gi))})
		}
		g.Tiles = append(g.Tiles, tile)
	}
	return g
}

const (
	i915Labels = `card="card0",device="0x9a49",driver="i915",pci="0000:00:02.0"`
	xeLabels   = `card="card1",device="0x56a0",driver="xe",pci="0000:03:00.0"`
)
