# intel_gpu_exporter

**Prometheus exporter for Intel GPU telemetry on Linux. Covers every kernel surface — i915/xe sysfs, hwmon, DRM fdinfo, perf PMU, Level Zero sysman — and degrades gracefully on anything it can't reach.**

## Contents

- [How it works](#how-it-works)
- [Installation](#installation)
- [Configuration](#configuration)
- [Hardware coverage](#hardware-coverage)
- [Metrics](#metrics)
- [Permissions](#permissions)
- [Flags](#flags)

## How it works

On startup the exporter walks `/sys/class/drm`, identifies every Intel PCI device, decides whether `i915` or `xe` is bound, and lights up the collectors that source can actually answer for. Each collector is independent: sysfs for frequencies and RC6, hwmon for power and temperature, DRM fdinfo for per-process engine usage, `perf_event_open` on the kernel-published i915/xe PMUs for the high-resolution counters, `intel_gpu_top -J` as a fallback when PMU access is locked down, and `libze_loader` via `dlopen` for Data Center GPUs (Flex, Max, Ponte Vecchio) — RAS counters, ECC errors, sub-engine activity, memory bandwidth. A kernel feature matrix runs at startup and logs which interfaces this kernel exposes vs which it doesn't, so missing metrics are explained rather than silently absent.

Deep notes on each source live in `docs/`: [kernel-feature-matrix](docs/kernel-feature-matrix.md), [pmu](docs/pmu.md), [levelzero](docs/levelzero.md).

## Installation

Grab the Linux binary for your arch from the [releases page](https://github.com/xsaveopt/intel_gpu_exporter/releases/latest) and drop it in `/usr/local/bin`:

```sh
sudo curl -fL -o /usr/local/bin/intel_gpu_exporter \
  https://github.com/xsaveopt/intel_gpu_exporter/releases/latest/download/intel_gpu_exporter_linux_amd64
sudo chmod +x /usr/local/bin/intel_gpu_exporter
```

For a long-running service: [docs/systemd.md](docs/systemd.md) has a reference unit, the service-user setup, and the lines to comment out if you're not using the PMU collector.

## Configuration

Everything is flag-driven on the systemd unit's `ExecStart=` line. Edit `/etc/systemd/system/intel_gpu_exporter.service` and `systemctl daemon-reload` to change behaviour — typical things to tweak are the listen address, the fdinfo top-N cap, and disabling the `intel_gpu_top` fallback if you've already got native PMU access. See [Flags](#flags) below.

The Level Zero collector self-activates if `libze_loader.so.1` is on the standard loader path. To enable it on a Data Center GPU host, install the Intel oneAPI runtime (`apt install intel-level-zero-gpu libze1` on Debian/Ubuntu, `dnf install intel-level-zero` on Fedora/RHEL) and restart the service.

A starter Grafana dashboard ships in [docs/grafana/intel-gpu.json](docs/grafana/intel-gpu.json) — see [docs/grafana.md](docs/grafana.md) for the panel breakdown.

## Hardware coverage

Every Intel GPU back to Gen 3 (i830) that binds to `i915` or `xe` is auto-discovered. Coverage table — iGPUs Sandy Bridge through Panther Lake, dGPUs DG1 / Arc Alchemist / Battlemage / Flex / Max — is in [docs/hardware-coverage.md](docs/hardware-coverage.md).

## Metrics

All metric names start with `intel_gpu_`. Common labels on per-device series: `card`, `pci`, `device`, `driver`. Per-process series carry `pid`, `comm`, `engine`. PMU counters carry `pmu`, `event`, `family`, `engine`, `kind`. Level Zero metrics carry `pci`, `subdevice` and a metric-specific axis (sensor, domain, engine, module, category, …).

Full breakdown of every emitted series and the kernel source feeding it: [docs/metrics.md](docs/metrics.md).

## Permissions

| Collector       | Required                                                   |
| --------------- | ---------------------------------------------------------- |
| sysfs (i915/xe) | read of `/sys/class/drm` (world-readable by default)       |
| hwmon           | read of `/sys/class/hwmon`                                 |
| fdinfo          | `CAP_SYS_PTRACE` to read other users' fds                  |
| `intel_gpu_top` | `CAP_PERFMON` or root (depends on kernel)                  |
| PMU             | `CAP_PERFMON` and `perf_event_paranoid <= 1`               |
| Level Zero      | access to `/dev/dri/renderD*` (usually the `render` group) |

The reference unit in [docs/systemd.md](docs/systemd.md) runs as a dedicated `intel-gpu-exporter` user with `SupplementaryGroups=render video` (so it can open `/dev/dri/*` without being added to those groups globally), grants `CAP_PERFMON` for PMU access, and otherwise locks the process down (`NoNewPrivileges`, read-only `/sys` and `/proc`, no namespaces, syscall filter, cgroup `DeviceAllow=/dev/dri rw`). Drop `CAP_PERFMON` if you're not running PMU.

## Flags

| Flag                             | Default         | Purpose                                                                |
| -------------------------------- | --------------- | ---------------------------------------------------------------------- |
| `--web.listen-address`           | `:9404`         | HTTP listen address (Prometheus default port for this exporter).       |
| `--web.telemetry-path`           | `/metrics`      | Endpoint path.                                                         |
| `--path.sysfs`                   | `/sys`          | sysfs mountpoint (override for chrooted scrapes).                      |
| `--path.procfs`                  | `/proc`         | procfs mountpoint.                                                     |
| `--scrape.timeout`               | `5s`            | Per-scrape deadline shared across all collectors.                      |
| `--collector.fdinfo`             | `true`          | Per-process DRM fdinfo collector.                                      |
| `--collector.fdinfo.top-n`       | `32`            | Cap process-level series at the top-N busiest PIDs (`0` disables cap). |
| `--collector.pmu`                | `true`          | Native `perf_event_open` PMU collector.                                |
| `--collector.intel-gpu-top`      | `true`          | `intel_gpu_top -J` fallback when PMU is locked down.                   |
| `--collector.intel-gpu-top.path` | `intel_gpu_top` | Override the binary path.                                              |
| `--log.level`                    | `info`          | `debug` / `info` / `warn` / `error`.                                   |
