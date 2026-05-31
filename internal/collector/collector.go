package collector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
)

const Namespace = "intel_gpu"

type Source interface {
	Name() string

	Available(gpus []discovery.GPU) bool

	Update(ctx context.Context, ch chan<- prometheus.Metric) error
}

type Registry struct {
	gpus    []discovery.GPU
	sources []Source
	log     *slog.Logger
	timeout time.Duration

	scrapeDuration *prometheus.Desc
	scrapeSuccess  *prometheus.Desc
}

func NewRegistry(log *slog.Logger, gpus []discovery.GPU, timeout time.Duration, sources ...Source) *Registry {
	active := make([]Source, 0, len(sources))
	for _, s := range sources {
		if s.Available(gpus) {
			log.Info("collector enabled", "source", s.Name())
			active = append(active, s)
		} else {
			log.Info("collector unavailable on this system, skipping", "source", s.Name())
		}
	}
	return &Registry{
		gpus:    gpus,
		sources: active,
		log:     log,
		timeout: timeout,
		scrapeDuration: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "scrape", "duration_seconds"),
			"intel_gpu_exporter: time taken to collect from a source.",
			[]string{"source"}, nil,
		),
		scrapeSuccess: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, "scrape", "success"),
			"intel_gpu_exporter: 1 if the source's last scrape succeeded, 0 otherwise.",
			[]string{"source"}, nil,
		),
	}
}

func (r *Registry) Describe(ch chan<- *prometheus.Desc) {
	ch <- r.scrapeDuration
	ch <- r.scrapeSuccess
}

func (r *Registry) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range r.sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			start := time.Now()
			err := s.Update(ctx, ch)
			elapsed := time.Since(start).Seconds()
			ch <- prometheus.MustNewConstMetric(r.scrapeDuration, prometheus.GaugeValue, elapsed, s.Name())
			success := 1.0
			if err != nil {
				success = 0.0
				r.log.Warn("source update failed", "source", s.Name(), "err", err)
			}
			ch <- prometheus.MustNewConstMetric(r.scrapeSuccess, prometheus.GaugeValue, success, s.Name())
		}(s)
	}
	wg.Wait()
}

func CommonLabels() []string {
	return []string{"card", "pci", "device", "driver"}
}

func LabelValues(g discovery.GPU) []string {
	return []string{g.Card, g.PCIAddr, g.DeviceID, string(g.Driver)}
}
