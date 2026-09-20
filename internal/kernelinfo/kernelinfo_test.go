package kernelinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in    string
		major int
		minor int
		patch int
	}{
		{"6.12.9-arch1-1", 6, 12, 9},
		{"6.8.0-45-generic", 6, 8, 0},
		{"5.15.0", 5, 15, 0},
		{"6.14", 6, 14, 0},
		{"6.20.0-rc1", 6, 20, 0},
		{"7.0.0+", 7, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			v := parse(tc.in)
			if v.Major != tc.major || v.Minor != tc.minor || v.Patch != tc.patch {
				t.Errorf("got %d.%d.%d, want %d.%d.%d", v.Major, v.Minor, v.Patch, tc.major, tc.minor, tc.patch)
			}
			if v.Raw != tc.in {
				t.Errorf("Raw = %q, want %q", v.Raw, tc.in)
			}
			if v.String() != tc.in {
				t.Errorf("String() = %q, want %q", v.String(), tc.in)
			}
		})
	}
}

func TestParseUnrecognised(t *testing.T) {
	v := parse("unknown")
	if v.Major != 0 || v.Minor != 0 || v.Patch != 0 {
		t.Errorf("expected a zero version, got %d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	if v.Raw != "unknown" {
		t.Errorf("Raw = %q, want %q", v.Raw, "unknown")
	}
}

func TestVersionString(t *testing.T) {
	if got := V(6, 12).String(); got != "6.12.0" {
		t.Errorf("got %q, want %q", got, "6.12.0")
	}
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		name string
		have Version
		want Version
		ok   bool
	}{
		{"same", V(6, 12), V(6, 12), true},
		{"higher minor", V(6, 13), V(6, 12), true},
		{"lower minor", V(6, 11), V(6, 12), false},
		{"higher major", V(7, 0), V(6, 20), true},
		{"lower major", V(5, 19), V(6, 0), false},
		{"higher major lower minor", V(7, 0), V(6, 99), true},
		{"patch ignored", parse("6.12.0"), parse("6.12.9"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.have.AtLeast(tc.want); got != tc.ok {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", tc.have, tc.want, got, tc.ok)
			}
		})
	}
}

func fakeProc(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

func TestDetectFromOsrelease(t *testing.T) {
	root := fakeProc(t, map[string]string{
		"sys/kernel/osrelease": "6.12.9-arch1-1\n",
		"version":              "Linux version 5.4.0-100-generic (buildd@lgw01) #113-Ubuntu SMP\n",
	})
	v := detect(root)
	if v.Major != 6 || v.Minor != 12 || v.Patch != 9 {
		t.Fatalf("got %s, want 6.12.9", v)
	}
}

func TestDetectFallsBackToVersion(t *testing.T) {
	root := fakeProc(t, map[string]string{
		"version": "Linux version 6.8.0-45-generic (buildd@lcy02) #45-Ubuntu SMP\n",
	})
	v := detect(root)
	if v.Major != 6 || v.Minor != 8 {
		t.Fatalf("got %s, want 6.8.x", v)
	}
}

func TestDetectUnknown(t *testing.T) {
	v := detect(t.TempDir())
	if v.Raw != "unknown" {
		t.Fatalf("got %q, want %q", v.Raw, "unknown")
	}
}

func TestPerfParanoid(t *testing.T) {
	cases := []struct {
		content string
		level   int
		desc    string
	}{
		{"-1\n", -1, "unrestricted"},
		{"0\n", 0, "raw tracepoints restricted"},
		{"1\n", 1, "default — CPU events need CAP_PERFMON"},
		{"2\n", 2, "kernel profiling also restricted"},
		{"3\n", 3, "hardened — perf events disabled for unprivileged users"},
		{"4\n", 4, "unknown level"},
	}
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.content), func(t *testing.T) {
			root := fakeProc(t, map[string]string{"sys/kernel/perf_event_paranoid": tc.content})
			level, desc, err := perfParanoid(root)
			if err != nil {
				t.Fatalf("perfParanoid: %v", err)
			}
			if level != tc.level {
				t.Errorf("level = %d, want %d", level, tc.level)
			}
			if desc != tc.desc {
				t.Errorf("desc = %q, want %q", desc, tc.desc)
			}
		})
	}
}

func TestPerfParanoidErrors(t *testing.T) {
	if _, _, err := perfParanoid(t.TempDir()); err == nil {
		t.Error("expected an error when the file is missing")
	}
	root := fakeProc(t, map[string]string{"sys/kernel/perf_event_paranoid": "not a number\n"})
	if _, _, err := perfParanoid(root); err == nil {
		t.Error("expected an error for unparsable content")
	}
}

func TestEvaluateGatesOnDriver(t *testing.T) {
	statuses := Evaluate(V(6, 20), map[string]bool{"i915": true})
	if len(statuses) != len(Matrix) {
		t.Fatalf("got %d statuses, want %d", len(statuses), len(Matrix))
	}
	for _, s := range statuses {
		switch s.Feature.Driver {
		case DriverI915:
			if !s.Available {
				t.Errorf("%s: expected available on 6.20 with i915, reason %q", s.Feature.ID, s.Reason)
			}
		case DriverXe:
			if s.Available {
				t.Errorf("%s: expected unavailable without an xe device", s.Feature.ID)
			}
			if s.Reason != "no xe device detected" {
				t.Errorf("%s: reason = %q", s.Feature.ID, s.Reason)
			}
		}
	}
}

func TestEvaluateGatesOnKernelVersion(t *testing.T) {
	statuses := Evaluate(V(6, 2), map[string]bool{"i915": true, "xe": true})
	byID := map[string]Status{}
	for _, s := range statuses {
		byID[s.Feature.ID] = s
	}

	if s := byID["i915.hwmon.power"]; !s.Available {
		t.Errorf("i915.hwmon.power should be available on 6.2, reason %q", s.Reason)
	}
	s := byID["i915.hwmon.fan"]
	if s.Available {
		t.Error("i915.hwmon.fan should not be available on 6.2")
	}
	if want := "requires kernel >= 6.12.0 (running 6.2.0)"; s.Reason != want {
		t.Errorf("reason = %q, want %q", s.Reason, want)
	}
}

func TestEvaluateNoDrivers(t *testing.T) {
	for _, s := range Evaluate(V(7, 0), map[string]bool{}) {
		if s.Available {
			t.Errorf("%s: expected unavailable with no drivers detected", s.Feature.ID)
		}
	}
}

func TestMatrixEntriesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Matrix {
		if f.ID == "" {
			t.Error("matrix entry with an empty ID")
			continue
		}
		if seen[f.ID] {
			t.Errorf("duplicate matrix ID %q", f.ID)
		}
		seen[f.ID] = true
		if f.Summary == "" {
			t.Errorf("%s: empty summary", f.ID)
		}
		switch f.Driver {
		case DriverAny, DriverI915, DriverXe:
		default:
			t.Errorf("%s: unknown driver %q", f.ID, f.Driver)
		}
		if f.Since.Major == 0 {
			t.Errorf("%s: missing Since version", f.ID)
		}
		if !strings.HasPrefix(f.ID, string(f.Driver)+".") {
			t.Errorf("%s: ID does not match driver %q", f.ID, f.Driver)
		}
	}
}
