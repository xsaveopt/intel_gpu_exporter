// Package kernelinfo detects the running Linux kernel version and reports which
// Intel-GPU-related kernel features are expected to be present.
//
// Each feature carries the minimum kernel version where it was merged. References
// are tracked next to each entry so the matrix can be audited.
package kernelinfo

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a parsed kernel version (major.minor[.patch]).
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

func (v Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast reports whether v >= other (major.minor only, patch ignored).
func (v Version) AtLeast(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor >= other.Minor
}

// V is a shorthand to build a Version literal.
func V(major, minor int) Version { return Version{Major: major, Minor: minor} }

var verRe = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?`)

// PerfParanoid reads /proc/sys/kernel/perf_event_paranoid. Returns the int
// value and a human-readable description of what it means for our PMU use.
//
//	-1  unrestricted
//	 0  disallow raw tracepoints without CAP_SYS_ADMIN
//	 1  also disallow CPU events without CAP_PERFMON (default on most distros)
//	 2  also disallow kernel profiling
//	 3  hardened — no perf events at all (Debian/Ubuntu hardened)
//
// DRM PMUs (i915, xe) require value <= 1 OR CAP_PERFMON.
func PerfParanoid() (int, string, error) {
	b, err := os.ReadFile("/proc/sys/kernel/perf_event_paranoid")
	if err != nil {
		return 0, "", err
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, "", err
	}
	desc := map[int]string{
		-1: "unrestricted",
		0:  "raw tracepoints restricted",
		1:  "default — CPU events need CAP_PERFMON",
		2:  "kernel profiling also restricted",
		3:  "hardened — perf events disabled for unprivileged users",
	}[v]
	if desc == "" {
		desc = "unknown level"
	}
	return v, desc, nil
}

// Detect reads the running kernel version. On non-Linux it returns a zero
// Version with the OS name stashed in Raw so callers can render it.
func Detect() Version {
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return parse(strings.TrimSpace(string(b)))
	}
	if b, err := os.ReadFile("/proc/version"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) >= 3 {
			return parse(fields[2])
		}
	}
	return Version{Raw: "unknown"}
}

func parse(s string) Version {
	m := verRe.FindStringSubmatch(s)
	if m == nil {
		return Version{Raw: s}
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return Version{Major: maj, Minor: min, Patch: patch, Raw: s}
}

// Driver scope a feature applies to.
type Driver string

const (
	DriverAny  Driver = "any"
	DriverI915 Driver = "i915"
	DriverXe   Driver = "xe"
)

// Feature describes a kernel-exposed metric source and when it became available.
type Feature struct {
	ID       string  // short stable identifier used in logs
	Driver   Driver  // which driver(s) this concerns
	Since    Version // minimum kernel where the feature is upstream
	Summary  string  // one-line description
	Notes    string  // optional caveats ("DG1/DG2 only", "Battlemage and newer")
	Endpoint string  // example path / PMU name
	Ref      string  // commit hash, patch URL or LWN article
}

// Matrix is the curated list of features the exporter can read. Keep this list
// in sync with reality; add a Ref for every entry.
//
// Sources of truth (audit when bumping):
//   - drm/i915: Documentation/gpu/i915.rst + drivers/gpu/drm/i915/i915_pmu.c
//   - drm/xe:   Documentation/gpu/xe/ + drivers/gpu/drm/xe/
//   - hwmon ABI: Documentation/ABI/testing/sysfs-driver-intel-i915-hwmon
//                Documentation/ABI/testing/sysfs-driver-intel-xe-hwmon
//   - fdinfo:   Documentation/gpu/drm-usage-stats.rst
var Matrix = []Feature{
	// --- i915 ---
	{
		ID: "i915.pmu", Driver: DriverI915, Since: V(4, 16),
		Summary:  "i915 PMU: engine busy/sema/wait, requested/actual frequency, RC6 residency, interrupts",
		Endpoint: "/sys/bus/event_source/devices/i915",
		Ref:      "drivers/gpu/drm/i915/i915_pmu.c",
	},
	{
		ID: "i915.sysfs.freq", Driver: DriverI915, Since: V(4, 0),
		Summary:  "i915 sysfs GT frequency files (gt_cur/act/min/max/RP0/RPn_freq_mhz)",
		Endpoint: "/sys/class/drm/cardN/gt_*_freq_mhz",
		Ref:      "drivers/gpu/drm/i915/i915_sysfs.c",
	},
	{
		ID: "i915.sysfs.rc6", Driver: DriverI915, Since: V(4, 0),
		Summary:  "i915 RC6 residency counter",
		Endpoint: "/sys/class/drm/cardN/power/rc6_residency_ms",
	},
	{
		ID: "i915.hwmon.power", Driver: DriverI915, Since: V(6, 2),
		Summary:  "i915 hwmon power/energy/voltage (RAPL-style)",
		Notes:    "discrete DG1/DG2 (Arc Alchemist) only — integrated iGPUs do not expose this",
		Endpoint: "/sys/class/hwmon/hwmon*/{power1_max,energy1_input,in0_input}",
		Ref:      "Documentation/ABI/testing/sysfs-driver-intel-i915-hwmon",
	},
	{
		ID: "i915.hwmon.fan", Driver: DriverI915, Since: V(6, 12),
		Summary:  "i915 hwmon fan tachometer",
		Notes:    "discrete cards only",
		Endpoint: "/sys/class/hwmon/hwmon*/fan1_input",
		Ref:      "patchwork.kernel.org/project/intel-gfx/patch/20240730060520...",
	},
	{
		ID: "i915.hwmon.temp", Driver: DriverI915, Since: V(6, 13),
		Summary:  "i915 hwmon package temperature",
		Notes:    "discrete cards only",
		Endpoint: "/sys/class/hwmon/hwmon*/temp1_input",
		Ref:      "lists.freedesktop.org/.../2024-September/355910.html",
	},
	{
		ID: "i915.fdinfo.engine", Driver: DriverI915, Since: V(5, 19),
		Summary:  "i915 per-client engine usage via DRM fdinfo (drm-engine-*)",
		Endpoint: "/proc/<pid>/fdinfo/<fd>",
		Ref:      "Documentation/gpu/drm-usage-stats.rst",
	},
	{
		ID: "i915.fdinfo.memory", Driver: DriverI915, Since: V(6, 8),
		Summary:  "i915 per-client memory stats via fdinfo (drm-total-localN, drm-resident-*)",
		Endpoint: "/proc/<pid>/fdinfo/<fd>",
		Ref:      "commit 9688530, merged v6.8-rc1",
	},

	// --- xe ---
	{
		ID: "xe.driver", Driver: DriverXe, Since: V(6, 8),
		Summary: "Intel Xe driver initial upstream merge (Tiger Lake+, DG2, Lunar Lake)",
		Ref:     "drivers/gpu/drm/xe/",
	},
	{
		ID: "xe.sysfs.freq", Driver: DriverXe, Since: V(6, 8),
		Summary:  "Xe sysfs GT frequency (tile*/gt*/freq0/{cur,act,rp0,rpa,rpn}_freq)",
		Endpoint: "/sys/.../device/tile*/gt*/freq0/",
		Ref:      "Documentation/gpu/xe/xe_gt_freq.rst",
	},
	{
		ID: "xe.sysfs.throttle", Driver: DriverXe, Since: V(6, 10),
		Summary:  "Xe frequency throttle reasons (pl1, pl2, pl4, thermal, prochot, ratl, vr_*)",
		Endpoint: "/sys/.../freq0/throttle/",
	},
	{
		ID: "xe.hwmon.power", Driver: DriverXe, Since: V(6, 8),
		Summary:  "Xe hwmon power (power1_max PL1, power2_max package PL1) and energy",
		Endpoint: "/sys/.../hwmon/hwmon*/power[12]_max",
		Ref:      "Documentation/ABI/testing/sysfs-driver-intel-xe-hwmon",
	},
	{
		ID: "xe.hwmon.fan", Driver: DriverXe, Since: V(6, 12),
		Summary:  "Xe hwmon fan tachometer",
		Notes:    "discrete cards only",
	},
	{
		ID: "xe.hwmon.temp", Driver: DriverXe, Since: V(6, 13),
		Summary:  "Xe hwmon GPU package temperature",
		Notes:    "discrete cards only",
		Endpoint: "/sys/.../hwmon/hwmon*/temp1_input",
	},
	{
		ID: "xe.hwmon.temp_extras", Driver: DriverXe, Since: V(6, 20),
		Summary:  "Xe additional temperatures: vRAM channels, memory controller, PCIe, shutdown limit",
		Notes:    "Linux 6.20 / 7.0 merge window (early 2026) — not yet released",
		Ref:      "phoronix.com/news/Linux-7.0-Intel-GPU-Temperature",
	},
	{
		ID: "xe.fdinfo.engine", Driver: DriverXe, Since: V(6, 8),
		Summary: "Xe per-client engine usage via DRM fdinfo (drm-engine-*)",
	},
	{
		ID: "xe.fdinfo.memory", Driver: DriverXe, Since: V(6, 10),
		Summary: "Xe per-client memory stats via fdinfo",
	},
	{
		ID: "xe.pmu", Driver: DriverXe, Since: V(6, 14),
		Summary:  "Xe PMU perf events (engine busy, freq, C6 residency)",
		Endpoint: "/sys/bus/event_source/devices/xe_*",
		Ref:      "lists.freedesktop.org/archives/intel-xe/2024-December/061444.html (v8)",
	},
}

// Status describes one feature's availability given a kernel version & drivers.
type Status struct {
	Feature   Feature
	Available bool
	Reason    string // why not available (kernel too old / driver not present)
}

// Evaluate returns the status of each known feature for the running kernel and
// the set of detected drivers ("i915", "xe").
func Evaluate(k Version, drivers map[string]bool) []Status {
	out := make([]Status, 0, len(Matrix))
	for _, f := range Matrix {
		s := Status{Feature: f, Available: true}
		if f.Driver != DriverAny && !drivers[string(f.Driver)] {
			s.Available = false
			s.Reason = fmt.Sprintf("no %s device detected", f.Driver)
		} else if !k.AtLeast(f.Since) {
			s.Available = false
			s.Reason = fmt.Sprintf("requires kernel >= %s (running %s)", f.Since, k)
		}
		out = append(out, s)
	}
	return out
}
