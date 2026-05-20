package collector

// PMU events are auto-discovered from /sys/bus/event_source/devices/<pmu>/.
// The collector is implemented per-OS:
//
//   pmu_linux.go  — real perf_event_open implementation
//   pmu_stub.go   — no-op on non-Linux dev hosts
//
// Both expose the same NewPMU(...) constructor.
