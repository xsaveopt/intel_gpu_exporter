# Hardware coverage

All Intel GPUs that bind to `i915` or `xe` are auto-discovered — the exporter filters PCI devices by vendor `0x8086` under `/sys/class/drm` and reads whatever kernel interfaces each one exposes. Anything bound to `i915` going back to Gen 3 (i830 / i845 / i865) still produces sysfs frequency, RC6, and PMU metrics where the kernel publishes them. Pre-Sandy-Bridge silicon won't expose PMU events but everything else degrades gracefully.

## Integrated GPUs (iGPU)

| Family / generation         | CPU codenames                                | Default driver | Notes |
|-----------------------------|----------------------------------------------|---------------|-------|
| Gen 6 (HD 2000/3000)        | Sandy Bridge                                 | i915          | basic sysfs only |
| Gen 7/7.5 (HD 2500–5200)    | Ivy Bridge, Haswell                          | i915          | i915 PMU from 4.16 |
| Gen 8 (HD 5300–6300)        | Broadwell                                    | i915          |  |
| Gen 9/9.5 (HD/UHD 5xx–6xx)  | Skylake, Kaby/Coffee/Comet/Whiskey Lake      | i915          |  |
| Gen 11 (Iris Plus G1–G7)    | Ice Lake                                     | i915          |  |
| Xe-LP (Iris Xe / UHD)       | Tiger/Rocket/Alder/Raptor/Jasper/Elkhart     | i915          | xe optional ≥ 6.8 |
| Xe-LPG (Arc Graphics iGPU)  | Meteor Lake                                  | i915          | xe ≥ 6.8 (recommended ≥ 6.12) |
| Xe2-LPG (Arc 130V/140V)     | Lunar Lake                                   | xe            | needs ≥ 6.10 |
| Xe-LPG+ (Arc Graphics)      | Arrow Lake                                   | xe / i915     | dual support ≥ 6.10 |
| Xe3 (upcoming)              | Panther Lake                                 | xe            | merging through 2026 |

## Discrete GPUs (dGPU)

| Product family                          | Architecture | Default driver | Min kernel |
|-----------------------------------------|--------------|---------------|------------|
| Iris Xe MAX (DG1)                       | Xe-LP        | i915          | 5.14 |
| Arc A-series (A310/A380/A580/A750/A770) | Xe-HPG (DG2) | i915 (xe optional) | 6.2 i915 / 6.8 xe |
| Arc B-series (B570/B580)                | Xe2-HPG (BMG)| xe **only**   | 6.11 (recommended 6.13) |
| Data Center GPU Flex (140/170)          | Xe-HPG (ATS-M)| i915 (xe ≥ 6.10) | 6.2 |
| Data Center GPU Max (1100/1550, PVC)    | Xe-HPC       | xe (i915 dropped) | 6.8 xe |
