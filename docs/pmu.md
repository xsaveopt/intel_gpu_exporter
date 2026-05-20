# PMU collector

The PMU collector is fully data-driven. At startup it scans `/sys/bus/event_source/devices/` for entries named `i915`, `i915_<pci>`, or `xe_*`. For each PMU it reads:

- `type` — the `PERF_TYPE` for `perf_event_attr.type`.
- `events/<name>` — the kernel-published event description, e.g. `event=0x01` or `event=0x05,gt=0`.
- `format/<key>` — bit ranges into `attr.config`, used to encode the key=value pairs from events into a single 64-bit config word.
- `cpumask` — DRM PMUs only accept reads from a specific CPU; the fd is pinned to the first CPU listed.

One fd is opened per discovered event. On every scrape we issue a `read(fd)`, parse the 8-byte counter, and emit `intel_gpu_pmu_counter{pmu, event, family, engine, kind}`. Future kernel events light up with zero code changes.

## Decoded labels

The event name from the kernel is preserved verbatim in `event`. We also derive friendlier labels:

| Family       | Event names                                           | engine     | kind                                  |
|--------------|-------------------------------------------------------|------------|---------------------------------------|
| `engine`     | i915 per-engine: `rcs0-busy`, `vcs0-sema`, `bcs0-wait`, … | `rcs0` etc. | `busy` / `sema` / `wait`              |
| `engine`     | xe global: `engine-active-ticks`, `engine-total-ticks` |  | `active` / `total`                    |
| `frequency`  | i915 `actual-frequency`, `requested-frequency`        |  | `actual-frequency` / `requested-frequency` |
| `frequency`  | xe `gt-actual-frequency` (the `gt-` prefix is stripped) |  | `actual-frequency`                    |
| `rc6`        | `rc6-residency`, `gt-c6-residency`                    |  |  |
| `interrupts` | `interrupts`                                          |  |  |
| `awake`      | `software-gt-awake-time`                              |  |  |
| `other`      | anything not matching the above                       |  |  |

## PromQL recipes

Engine busy ratio over a 1-minute window:

```promql
rate(intel_gpu_pmu_counter{family="engine", kind="busy"}[1m]) / 1e9
```

RC6/C6 residency:

```promql
rate(intel_gpu_pmu_counter{family="rc6"}[1m]) / 1e9
```

Average frequency (the PMU counter accumulates MHz samples, so `rate()` returns mean MHz over the window):

```promql
rate(intel_gpu_pmu_counter{family="frequency", kind="actual-frequency"}[1m])
```

The sysfs collectors expose direct frequency gauges as well (`intel_gpu_i915_frequency_actual_mhz`, `intel_gpu_xe_frequency_actual_mhz`) which are usually more convenient for dashboards.

## Permissions

Set either `/proc/sys/kernel/perf_event_paranoid <= 1` or grant the binary `CAP_PERFMON`. The reference unit in [systemd.md](systemd.md) grants `CAP_PERFMON` + `CAP_SYS_ADMIN`. If the PMU collector logs `pmu collector unavailable`, the hint about `perf_event_paranoid` is the first thing to check.
