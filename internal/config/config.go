package config

import (
	"flag"
	"time"
)

type Config struct {
	ListenAddr      string
	MetricsPath     string
	SysfsRoot       string
	ProcRoot        string
	HwmonRoot       string
	ScrapeTimeout   time.Duration
	EnableFdinfo    bool
	FdinfoTopN      int
	EnableGpuTop    bool
	EnablePMU       bool
	IntelGpuTopPath string
	LogLevel        string
}

func Parse() *Config {
	c := &Config{}
	flag.StringVar(&c.ListenAddr, "web.listen-address", ":9404", "address to listen on for HTTP requests")
	flag.StringVar(&c.MetricsPath, "web.telemetry-path", "/metrics", "path under which to expose metrics")
	flag.StringVar(&c.SysfsRoot, "path.sysfs", "/sys", "sysfs mountpoint")
	flag.StringVar(&c.ProcRoot, "path.procfs", "/proc", "procfs mountpoint")
	flag.StringVar(&c.HwmonRoot, "path.hwmon", "/sys/class/hwmon", "hwmon root")
	flag.DurationVar(&c.ScrapeTimeout, "scrape.timeout", 5*time.Second, "maximum time a single scrape may take")
	flag.BoolVar(&c.EnableFdinfo, "collector.fdinfo", true, "enable per-process DRM fdinfo collector")
	flag.IntVar(&c.FdinfoTopN, "collector.fdinfo.top-n", 32, "fdinfo: cap per-process series at the top N processes by aggregate engine time (0 = unlimited)")
	flag.BoolVar(&c.EnableGpuTop, "collector.intel-gpu-top", true, "enable intel_gpu_top fallback collector when PMU is unavailable")
	flag.BoolVar(&c.EnablePMU, "collector.pmu", true, "enable i915/xe PMU collector via perf_event_open")
	flag.StringVar(&c.IntelGpuTopPath, "collector.intel-gpu-top.path", "intel_gpu_top", "path to intel_gpu_top binary")
	flag.StringVar(&c.LogLevel, "log.level", "info", "log level (debug|info|warn|error)")
	flag.Parse()
	return c
}
