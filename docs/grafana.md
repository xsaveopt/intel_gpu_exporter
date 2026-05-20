# Grafana dashboard

A starter dashboard JSON is at [`docs/grafana/intel-gpu.json`](grafana/intel-gpu.json). Import it via **Grafana → Dashboards → New → Import → Upload JSON**.

It expects a Prometheus datasource and uses a `card` label values query for its `$card` variable.

Panels:

- GPU inventory (table built from `intel_gpu_info`)
- GT frequency, actual vs requested (i915 + xe)
- Power (PL1 + derived from energy counter)
- Temperature
- Engine busy %, derived from `rate(intel_gpu_pmu_counter[1m]) / 1e9`
- RC6 / C6 residency %
- Top-N processes by GPU engine time
- PCIe link state
- xe throttle reasons
