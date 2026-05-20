package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sratabix/intel_gpu_exporter/internal/sysutil"
)

const intelVendor = "0x8086"

type Driver string

const (
	DriverI915    Driver = "i915"
	DriverXe      Driver = "xe"
	DriverUnknown Driver = "unknown"
)

// GPU describes a discovered Intel GPU device.
type GPU struct {
	Card       string // e.g. "card0"
	DRMPath    string // /sys/class/drm/card0
	DevicePath string // /sys/class/drm/card0/device (resolved real path)
	PCIAddr    string // 0000:00:02.0
	DeviceID   string // 0x...
	Driver     Driver
	HwmonPaths []string // /sys/class/hwmon/hwmon* belonging to this device
	Tiles      []Tile   // xe only; for i915 typically a single synthetic tile
}

type Tile struct {
	Index int
	GTs   []GT
	Path  string // tile root in sysfs (xe), or empty (i915)
}

type GT struct {
	Index int
	Path  string
}

// Discover scans sysfs for Intel GPUs.
func Discover(sysfsRoot string) ([]GPU, error) {
	drmRoot := filepath.Join(sysfsRoot, "class", "drm")
	entries, err := os.ReadDir(drmRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", drmRoot, err)
	}

	var gpus []GPU
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "card") || strings.ContainsRune(name, '-') {
			continue
		}
		drmPath := filepath.Join(drmRoot, name)
		devPath, err := filepath.EvalSymlinks(filepath.Join(drmPath, "device"))
		if err != nil {
			continue
		}
		vendor, _ := sysutil.ReadString(filepath.Join(devPath, "vendor"))
		if vendor != intelVendor {
			continue
		}
		deviceID, _ := sysutil.ReadString(filepath.Join(devPath, "device"))
		driver := detectDriver(devPath)
		gpu := GPU{
			Card:       name,
			DRMPath:    drmPath,
			DevicePath: devPath,
			PCIAddr:    filepath.Base(devPath),
			DeviceID:   deviceID,
			Driver:     driver,
		}
		gpu.HwmonPaths = findHwmon(devPath)
		gpu.Tiles = discoverTiles(driver, devPath)
		gpus = append(gpus, gpu)
	}
	sort.Slice(gpus, func(i, j int) bool { return gpus[i].Card < gpus[j].Card })
	if len(gpus) == 0 {
		return nil, errors.New("no Intel GPU found in sysfs")
	}
	return gpus, nil
}

func detectDriver(devPath string) Driver {
	link, err := os.Readlink(filepath.Join(devPath, "driver"))
	if err != nil {
		return DriverUnknown
	}
	switch filepath.Base(link) {
	case "i915":
		return DriverI915
	case "xe":
		return DriverXe
	default:
		return DriverUnknown
	}
}

func findHwmon(devPath string) []string {
	matches, _ := filepath.Glob(filepath.Join(devPath, "hwmon", "hwmon*"))
	return matches
}

func discoverTiles(driver Driver, devPath string) []Tile {
	switch driver {
	case DriverXe:
		var tiles []Tile
		tileDirs, _ := filepath.Glob(filepath.Join(devPath, "tile*"))
		sort.Strings(tileDirs)
		for ti, td := range tileDirs {
			tile := Tile{Index: ti, Path: td}
			gtDirs, _ := filepath.Glob(filepath.Join(td, "gt*"))
			sort.Strings(gtDirs)
			for gi, gd := range gtDirs {
				tile.GTs = append(tile.GTs, GT{Index: gi, Path: gd})
			}
			tiles = append(tiles, tile)
		}
		return tiles
	case DriverI915:
		// Modern i915 (>= 6.0) exposes per-GT files under <drm>/gt/gtN/.
		// drmPath equals filepath.Dir(devPath) sans /device; we receive devPath
		// here, but per-GT lives under the DRM node not the PCI node, so the
		// caller (Discover) re-maps. We do the glob through the DRM root by
		// walking up to /sys/class/drm/cardN/gt/.
		// To keep this function pure we return the synthetic tile with no GT
		// paths; the i915 sysfs collector resolves the DRM path itself.
		return []Tile{{Index: 0, GTs: []GT{{Index: 0, Path: devPath}}}}
	default:
		return []Tile{{Index: 0, GTs: []GT{{Index: 0, Path: devPath}}}}
	}
}

// I915GTs returns the per-GT sysfs paths under /sys/class/drm/cardN/gt/gt*
// for a given GPU. Returns empty slice on single-GT platforms that don't
// expose the gt/ subtree.
func I915GTs(g GPU) []GT {
	if g.Driver != DriverI915 {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(g.DRMPath, "gt", "gt*"))
	sort.Strings(matches)
	gts := make([]GT, 0, len(matches))
	for i, p := range matches {
		gts = append(gts, GT{Index: i, Path: p})
	}
	return gts
}
