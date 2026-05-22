//go:build linux

// Package levelzero binds the subset of the oneAPI Level Zero sysman (ZES)
// API needed to read telemetry from Intel Data Center GPUs (Flex, Max/PVC) and
// any other Level-Zero-supported device on the host.
//
// The library is loaded at runtime via purego — no CGo, so the exporter remains
// a single static binary. If libze_loader is not installed the package returns
// errUnavailable from Open and the collector self-disables.
//
// API reference: https://oneapi-src.github.io/level-zero-spec/level-zero/latest/sysman/api.html
package levelzero

import "unsafe"

// Opaque handles — all Level Zero handles are just typedef'd pointers in C.
type (
	DriverHandle = unsafe.Pointer
	DeviceHandle = unsafe.Pointer
	PwrHandle    = unsafe.Pointer
	TempHandle   = unsafe.Pointer
	FreqHandle   = unsafe.Pointer
	EngineHandle = unsafe.Pointer
	RasHandle    = unsafe.Pointer
	MemHandle    = unsafe.Pointer
)

// ZE_RESULT_SUCCESS is the only return value we ever want to see.
const zeResultSuccess uint32 = 0

// Structure type constants — only the few we set on input structs.
const (
	stypePowerEnergyCounter uint32 = 0x2
	stypeFreqState          uint32 = 0xb
	stypeFreqThrottleTime   uint32 = 0xc
	stypeFreqProperties     uint32 = 0xa
	stypePowerProperties    uint32 = 0x1
	stypeEngineStats        uint32 = 0x19
	stypeEngineProperties   uint32 = 0x18
	stypeMemState           uint32 = 0x1d
	stypeMemBandwidth       uint32 = 0x1e
	stypeMemProperties      uint32 = 0x1c
	stypeRasState           uint32 = 0x20
	stypeRasProperties      uint32 = 0x1f
	stypeTempProperties     uint32 = 0x21
	stypePciProperties      uint32 = 0x6
)

// Maximum categories filled in zes_ras_state_t.category[].
// ZES_MAX_RAS_ERROR_CATEGORY_COUNT in zes_api.h.
const maxRasCategoryCount = 7

// RAS category names indexed by zes_ras_error_cat_t.
var rasCategoryNames = [maxRasCategoryCount]string{
	"reset", "programming", "driver", "compute", "non_compute", "cache", "display",
}

// RAS error types.
const (
	rasTypeCorrectable   uint32 = 0
	rasTypeUncorrectable uint32 = 1
)

// rasTypeName turns a zes_ras_error_type_t into a Prometheus label value.
func rasTypeName(t uint32) string {
	switch t {
	case rasTypeCorrectable:
		return "correctable"
	case rasTypeUncorrectable:
		return "uncorrectable"
	default:
		return "unknown"
	}
}

// Temperature sensor types.
var tempSensorNames = map[uint32]string{
	0: "global", 1: "gpu", 2: "memory",
	3: "global_min", 4: "gpu_min", 5: "memory_min",
	6: "gpu_board", 7: "gpu_board_min", 8: "voltage_regulator",
}

// Memory module types.
var memTypeNames = map[uint32]string{
	0: "hbm", 1: "ddr", 2: "ddr3", 3: "ddr4", 4: "ddr5",
	5: "lpddr", 6: "lpddr3", 7: "lpddr4", 8: "lpddr5",
	9: "sram", 10: "l1", 11: "l3", 12: "grf", 13: "slm",
	14: "gddr4", 15: "gddr5", 16: "gddr5x", 17: "gddr6",
	18: "gddr6x", 19: "gddr7",
}

// Engine type flag bit -> label.
var engineTypeFlagNames = map[uint32]string{
	1 << 0: "other",
	1 << 1: "compute",
	1 << 2: "3d",
	1 << 3: "media",
	1 << 4: "dma",
	1 << 5: "render",
}

func engineTypeName(flags uint32) string {
	for bit, name := range engineTypeFlagNames {
		if flags&bit != 0 {
			return name
		}
	}
	return "all"
}

// ----- C struct mirrors -----
//
// Layout must match zes_api.h on 64-bit Linux. All structs start with
// (uint32 stype, void* pNext); the 4 bytes of padding between them on x86_64
// is reproduced explicitly with a blank field.

type pciAddress struct {
	Domain   uint32
	Bus      uint32
	Device   uint32
	Function uint32
}

type pciSpeed struct {
	Gen          int32
	Width        int32
	MaxBandwidth int64
}

type pciProperties struct {
	Stype                 uint32
	_                     uint32
	PNext                 uintptr
	Address               pciAddress
	MaxSpeed              pciSpeed
	HaveBandwidthCounters uint8
	HavePacketCounters    uint8
	HaveReplayCounters    uint8
	_                     [5]byte
}

type powerEnergyCounter struct {
	Stype     uint32
	_         uint32
	PNext     uintptr
	Energy    uint64 // microjoules
	Timestamp uint64 // microseconds
}

type powerProperties struct {
	Stype                      uint32
	_                          uint32
	PNext                      uintptr
	OnSubdevice                uint8
	_                          [3]byte
	SubdeviceID                uint32
	CanControl                 uint8
	IsEnergyThresholdSupported uint8
	_                          [2]byte
	DefaultLimit               int32
	MinLimit                   int32
	MaxLimit                   int32
	_                          [4]byte
}

type tempProperties struct {
	Stype                   uint32
	_                       uint32
	PNext                   uintptr
	SensorType              uint32
	OnSubdevice             uint8
	_                       [3]byte
	SubdeviceID             uint32
	MaxTemperature          int32
	IsCriticalTempSupported uint8
	_                       [3]byte
	CriticalTempThreshold   int32
	_                       [4]byte
}

type freqState struct {
	Stype           uint32
	_               uint32
	PNext           uintptr
	CurrentVoltage  float64
	Request         float64
	TDP             float64
	Efficient       float64
	Actual          float64
	ThrottleReasons uint32
	_               uint32
}

type freqThrottleTime struct {
	Stype        uint32
	_            uint32
	PNext        uintptr
	ThrottleTime uint64
	Timestamp    uint64
}

type freqProperties struct {
	Stype           uint32
	_               uint32
	PNext           uintptr
	Type            uint32
	OnSubdevice     uint8
	_               [3]byte
	SubdeviceID     uint32
	CanControl      uint8
	IsThrottleEvent uint8
	_               [2]byte
	Min             float64
	Max             float64
}

type engineStats struct {
	Stype      uint32
	_          uint32
	PNext      uintptr
	ActiveTime uint64
	Timestamp  uint64
}

type engineProperties struct {
	Stype       uint32
	_           uint32
	PNext       uintptr
	Type        uint32
	OnSubdevice uint8
	_           [3]byte
	SubdeviceID uint32
	_           [4]byte
}

type rasState struct {
	Stype    uint32
	_        uint32
	PNext    uintptr
	Category [maxRasCategoryCount]uint64
}

type rasProperties struct {
	Stype       uint32
	_           uint32
	PNext       uintptr
	Type        uint32
	OnSubdevice uint8
	_           [3]byte
	SubdeviceID uint32
	_           [4]byte
}

type memState struct {
	Stype  uint32
	_      uint32
	PNext  uintptr
	Health uint32
	_      uint32
	Free   uint64
	Size   uint64
}

type memBandwidth struct {
	Stype        uint32
	_            uint32
	PNext        uintptr
	ReadCounter  uint64
	WriteCounter uint64
	MaxBandwidth uint64
	Timestamp    uint64
}

type memProperties struct {
	Stype        uint32
	_            uint32
	PNext        uintptr
	Type         uint32
	OnSubdevice  uint8
	_            [3]byte
	SubdeviceID  uint32
	PhysicalSize uint64
	BusWidth     int32
	NumChannels  int32
}
