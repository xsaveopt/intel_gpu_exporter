package sysutil

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestReadString(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"plain", "i915", "i915"},
		{"trailing_newline", "0x8086\n", "0x8086"},
		{"padded", "  1300  \n", "1300"},
		{"empty", "", ""},
		{"multiline", "PCI_ID=8086:56A0\nPCI_SUBSYS_ID=1849:6005\n", "PCI_ID=8086:56A0\nPCI_SUBSYS_ID=1849:6005"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := write(t, dir, tc.name, tc.content)
			got, err := ReadString(path)
			if err != nil {
				t.Fatalf("ReadString: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadStringMissing(t *testing.T) {
	if _, err := ReadString(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestReadUint64(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    uint64
		wantErr bool
	}{
		{"zero", "0\n", 0, false},
		{"counter", "18446744073709551615\n", 18446744073709551615, false},
		{"spaced", " 4096 \n", 4096, false},
		{"hex_rejected", "0x8086\n", 0, true},
		{"float_rejected", "2.5 GT/s PCIe\n", 0, true},
		{"empty_rejected", "\n", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := write(t, dir, tc.name, tc.content)
			got, err := ReadUint64(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadUint64: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReadFloat64(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    float64
		wantErr bool
	}{
		{"integer", "1300\n", 1300, false},
		{"decimal", "2.5\n", 2.5, false},
		{"negative", "-1\n", -1, false},
		{"scientific", "1e3\n", 1000, false},
		{"unit_suffix_rejected", "2.5 GT/s PCIe\n", 0, true},
		{"text_rejected", "enabled\n", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := write(t, dir, tc.name, tc.content)
			got, err := ReadFloat64(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadFloat64: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadNumericMissing(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")
	if _, err := ReadUint64(absent); err == nil {
		t.Error("ReadUint64: expected an error for a missing file")
	}
	if _, err := ReadFloat64(absent); err == nil {
		t.Error("ReadFloat64: expected an error for a missing file")
	}
}
