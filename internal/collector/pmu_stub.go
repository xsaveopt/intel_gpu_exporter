//go:build !linux

package collector

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
)

// PMU collector is Linux-only. On other platforms this is a no-op that always
// reports unavailable so developers can build/run the exporter on macOS.
type PMU struct{}

func NewPMU(*slog.Logger) *PMU { return &PMU{} }

func (*PMU) Name() string                        { return "pmu" }
func (*PMU) Available(gpus []discovery.GPU) bool { return false }
func (*PMU) Update(context.Context, chan<- prometheus.Metric) error {
	return errors.New("pmu: linux only")
}
func (*PMU) Close() error { return nil }
