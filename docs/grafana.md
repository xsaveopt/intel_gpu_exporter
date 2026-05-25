# Grafana dashboard

A starter dashboard JSON is at [`docs/grafana/intel-gpu.json`](grafana/intel-gpu.json). Import it via **Grafana → Dashboards → New → Import → Upload JSON**.

It expects a Prometheus datasource and uses a `card` label values query for its `$card` variable.

## Collection cadence

The exporter is fully scrape-driven. On every `/metrics` request the registry runs each collector's `Update()` in parallel:

- **sysfs / hwmon / pcie / memory / engines / fdinfo** — files are read fresh on each scrape. Resolution is your Prometheus scrape interval.
- **PMU** — persistent perf fds opened once at startup; each scrape just `read(2)`s the counter. Effectively zero overhead per scrape.
- **intel_gpu_top** — a background process samples at 1 Hz (`-s 1000`). Scrapes return the latest cached sample, so values are at most ~1s stale.
- **Level Zero** — sysman calls happen synchronously during the scrape.

`intel_gpu_scrape_duration_seconds{source}` and `intel_gpu_scrape_success{source}` expose per-source timing/health and are shown in the Overview row.

## Panels

- **Overview** — `intel_gpu_info` inventory table, per-source scrape success + duration, fdinfo top-N drop count.
- **Frequency** — actual vs requested (i915 + xe), hardware limits (RP0/RPn/min/max).
- **Power & energy** — PL1 / TDP / crit; power draw derived as `rate(energy)`; voltage and current on separate panels.
- **Thermal** — temperature and fan tachometer.
- **Engine utilisation** — PMU engine busy / sema / wait, RC6 (PMU + sysfs fallback), interrupts, awake time, top-N processes by engine time.
- **Memory** — VRAM/local totals, top-N processes by resident bytes.
- **PCIe** — current vs max generation and link width.
- **Throttle reasons (xe)** — active throttle flags.
- **Utilization & correlations** — frequency headroom (actual/RP0), power utilization (draw/PL1), engine-busy vs power, temperature vs xe throttle overlay.
- **Engine config (i915)** *(collapsed)* — heartbeat / preempt / stop / timeslice intervals.
- **Level Zero / sysman** *(collapsed)* — Data Center GPU metrics: per-sensor temp, RAS errors, memory bandwidth, frequency throttle time.
- **intel_gpu_top fallback** *(collapsed)* — same engine/power/RC6 numbers as PMU, useful when PMU isn't reachable on the host.
