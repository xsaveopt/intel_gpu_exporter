# Kernel feature matrix

The exporter ships a kernel-version-aware feature matrix and prints it at startup. Each line marked `kernel feature available=...` or `kernel feature NOT available=...` tells you exactly which interface the kernel exposes versus which the exporter would have liked to read.

| Feature                                          | Driver | Since kernel | Notes |
|--------------------------------------------------|--------|--------------|-------|
| i915 PMU (engine busy / freq / RC6 / interrupts) | i915   | 4.16         |  |
| i915 sysfs GT frequencies                        | i915   | ancient      | iGPU + dGPU |
| i915 RC6 residency counter                       | i915   | ancient      |  |
| i915 hwmon power/energy/voltage (RAPL)           | i915   | 6.2          | DG1/DG2 only |
| i915 hwmon fan speed                             | i915   | 6.12         | discrete cards |
| i915 hwmon package temperature                   | i915   | 6.13         | discrete cards |
| i915 fdinfo `drm-engine-*`                       | i915   | 5.19         | per-process |
| i915 fdinfo memory stats                         | i915   | 6.8          | `drm-total-localN` |
| Xe driver merge                                  | xe     | 6.8          | Tiger Lake+ |
| Xe sysfs GT freq (`tile*/gt*/freq0`)             | xe     | 6.8          |  |
| Xe sysfs throttle reasons                        | xe     | 6.10         | PL1/PL2/PL4/thermal/prochot/ratl |
| Xe hwmon power/energy                            | xe     | 6.8          | `power1_max`, `power2_max` |
| Xe hwmon fan                                     | xe     | 6.12         | discrete |
| Xe hwmon package temperature                     | xe     | 6.13         | discrete |
| Xe hwmon extra temps (vRAM, mem ctrl, PCIe)      | xe     | 6.20 / 7.0   | future, not released |
| Xe fdinfo engine usage                           | xe     | 6.8          |  |
| Xe fdinfo memory stats                           | xe     | 6.10         |  |
| Xe PMU perf events                               | xe     | 6.14         | engine busy, C6, freq |

The authoritative source is `internal/kernelinfo/kernelinfo.go` — every entry carries a `Ref` to the upstream commit / patch / LWN article that introduced it.

`perf_event_paranoid` from `/proc/sys/kernel/perf_event_paranoid` is logged at startup too: `<=1` means PMU is usable as the service user; `>=2` means you'll need `CAP_PERFMON` on the unit (the reference unit in [systemd.md](systemd.md) grants this).
