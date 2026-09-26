package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLoggerLevels(t *testing.T) {
	cases := []struct {
		in      string
		enabled slog.Level
		below   slog.Level
		hasLow  bool
	}{
		{in: "debug", enabled: slog.LevelDebug},
		{in: "info", enabled: slog.LevelInfo, below: slog.LevelDebug, hasLow: true},
		{in: "warn", enabled: slog.LevelWarn, below: slog.LevelInfo, hasLow: true},
		{in: "error", enabled: slog.LevelError, below: slog.LevelWarn, hasLow: true},
		{in: "", enabled: slog.LevelInfo, below: slog.LevelDebug, hasLow: true},
		{in: "verbose", enabled: slog.LevelInfo, below: slog.LevelDebug, hasLow: true},
	}
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			l := newLogger(tc.in)
			if !l.Enabled(ctx, tc.enabled) {
				t.Errorf("level %v should be enabled", tc.enabled)
			}
			if tc.hasLow && l.Enabled(ctx, tc.below) {
				t.Errorf("level %v should be disabled", tc.below)
			}
		})
	}
}

func TestSlogErrorLog(t *testing.T) {
	var buf bytes.Buffer
	l := slogErrorLog{slog.New(slog.NewTextHandler(&buf, nil))}
	l.Println("encoding failed", 42)
	out := buf.String()
	for _, want := range []string{"level=ERROR", "msg=promhttp", "encoding failed", "42"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}
