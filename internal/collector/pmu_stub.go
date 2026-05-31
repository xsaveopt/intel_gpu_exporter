//go:build !linux

package collector

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sratabix/intel_gpu_exporter/internal/discovery"
)

type PMU struct{}

func NewPMU(*slog.Logger) *PMU { return &PMU{} }

func (*PMU) Name() string                        { return "pmu" }
func (*PMU) Available(gpus []discovery.GPU) bool { return false }
func (*PMU) Update(context.Context, chan<- prometheus.Metric) error {
	return errors.New("pmu: linux only")
}
func (*PMU) Close() error { return nil }
