# Level Zero / sysman

For Intel **Data Center GPU Flex, Max, and Ponte Vecchio** (and any other host with `libze_loader` installed), the exporter dlopens `libze_loader.so.1` at startup via [purego](https://github.com/ebitengine/purego) — no CGo, no build tag, the static binary stays single-file. If the loader isn't found the collector logs the fact once and stays out of the way.

`zesInit(0)` is called once, then every driver, device, and sub-handle (power, temperature, frequency, engine groups, RAS sets, memory modules) is enumerated and cached. Per-scrape we hit the cached handles only — no re-enumeration.

## Metrics added on top of kernel sysfs

| Metric                                                 | Source call                       |
|--------------------------------------------------------|-----------------------------------|
| `intel_gpu_zes_energy_microjoules_total`               | `zesPowerGetEnergyCounter`        |
| `intel_gpu_zes_temperature_celsius`                    | `zesTemperatureGetState` (per named sensor: gpu, memory, board, voltage_regulator, …) |
| `intel_gpu_zes_frequency_{actual,request,tdp}_mhz`     | `zesFrequencyGetState`            |
| `intel_gpu_zes_frequency_throttle_nanoseconds_total`   | `zesFrequencyGetThrottleTime`     |
| `intel_gpu_zes_engine_active_nanoseconds_total`        | `zesEngineGetActivity` per group (compute / render / media / dma / …) |
| `intel_gpu_zes_ras_errors_total{type, category}`       | `zesRasGetState` — correctable / uncorrectable × {reset, programming, driver, compute, non_compute, cache, display} |
| `intel_gpu_zes_memory_{size,free}_bytes`               | `zesMemoryGetState`               |
| `intel_gpu_zes_memory_{read,write}_bytes_total`, `_max_bandwidth_bytes_per_second` | `zesMemoryGetBandwidth` |

## Installing the runtime

```sh
# Debian/Ubuntu (Intel oneAPI repo)
apt-get install intel-level-zero-gpu libze1
# Fedora/RHEL
dnf install intel-level-zero
```

The service user needs membership in the `render` group to open `/dev/dri/renderD*`. The `useradd` + `usermod` lines in [systemd.md](systemd.md) cover this.

## Risk: struct ABI drift

The Go side carries hand-mirrored equivalents of `zes_*_t` structs from `zes_api.h` (see `internal/levelzero/types.go`). The layout assumes the LP64 ABI; **field-order drift in future `zes_api.h` revisions would silently corrupt readings**. The first time this collector runs against real hardware, spot-check one or two values against `xpumcli` or `zelist` to confirm the layout still matches.
