//go:build linux

package levelzero

import "unsafe"

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

const zeResultSuccess uint32 = 0

const (
	stypePciProperties    uint32 = 0x2
	stypeEngineProperties uint32 = 0x5
	stypeFreqProperties   uint32 = 0x9
	stypeMemProperties    uint32 = 0xb
	stypePowerProperties  uint32 = 0xd
	stypeRasProperties    uint32 = 0xf
	stypeTempProperties   uint32 = 0x14
	stypeFreqState        uint32 = 0x1b
	stypeMemState         uint32 = 0x1e
	stypeRasState         uint32 = 0x22
)

const maxRasCategoryCount = 7

var rasCategoryNames = [maxRasCategoryCount]string{
	"reset", "programming", "driver", "compute", "non_compute", "cache", "display",
}

const (
	rasTypeCorrectable   uint32 = 0
	rasTypeUncorrectable uint32 = 1
)

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

var tempSensorNames = map[uint32]string{
	0: "global", 1: "gpu", 2: "memory",
	3: "global_min", 4: "gpu_min", 5: "memory_min",
	6: "gpu_board", 7: "gpu_board_min", 8: "voltage_regulator",
}

var memTypeNames = map[uint32]string{
	0: "hbm", 1: "ddr", 2: "ddr3", 3: "ddr4", 4: "ddr5",
	5: "lpddr", 6: "lpddr3", 7: "lpddr4", 8: "lpddr5",
	9: "sram", 10: "l1", 11: "l3", 12: "grf", 13: "slm",
	14: "gddr4", 15: "gddr5", 16: "gddr5x", 17: "gddr6",
	18: "gddr6x", 19: "gddr7",
}

var engineGroupNames = map[uint32]string{
	0:  "all",
	1:  "compute_all",
	2:  "media_all",
	3:  "copy_all",
	4:  "compute_single",
	5:  "render_single",
	6:  "media_decode_single",
	7:  "media_encode_single",
	8:  "copy_single",
	9:  "media_enhancement_single",
	10: "3d_single",
	11: "3d_render_compute_all",
	12: "render_all",
	13: "3d_all",
	14: "media_codec_single",
}

func engineTypeName(group uint32) string {
	if name, ok := engineGroupNames[group]; ok {
		return name
	}
	return "unknown"
}

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
	Energy    uint64
	Timestamp uint64
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
}

type tempProperties struct {
	Stype                   uint32
	_                       uint32
	PNext                   uintptr
	SensorType              uint32
	OnSubdevice             uint8
	_                       [3]byte
	SubdeviceID             uint32
	_                       uint32
	MaxTemperature          float64
	IsCriticalTempSupported uint8
	IsThreshold1Supported   uint8
	IsThreshold2Supported   uint8
	_                       [5]byte
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
	Location     uint32
	PhysicalSize uint64
	BusWidth     int32
	NumChannels  int32
}
