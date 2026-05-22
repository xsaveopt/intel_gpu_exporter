//go:build !linux

package levelzero

import "errors"

var errUnavailable = errors.New("level zero supported on linux only")

type Client struct{}

type Device struct {
	PCIBus string
	Power  []Power
	Temp   []Temperature
	Freq   []Frequency
	Engine []Engine
	RAS    []RAS
	Mem    []Memory
}

type Power struct {
	MinLimitMW, MaxLimitMW int32
	OnSubdevice            bool
	SubdeviceID            uint32
}
type Temperature struct {
	Sensor      string
	OnSubdevice bool
	SubdeviceID uint32
}
type Frequency struct {
	Type        uint32
	OnSubdevice bool
	SubdeviceID uint32
}
type Engine struct {
	Type        string
	OnSubdevice bool
	SubdeviceID uint32
}
type RAS struct {
	ErrorType   string
	OnSubdevice bool
	SubdeviceID uint32
}
type Memory struct {
	Type         string
	OnSubdevice  bool
	SubdeviceID  uint32
	PhysicalSize uint64
}

// freqState is exported as opaque on stub.
type freqState struct{ Actual, Request, TDP float64 }

func (s freqState) ActualMHz() float64  { return s.Actual }
func (s freqState) RequestMHz() float64 { return s.Request }
func (s freqState) TDPMHz() float64     { return s.TDP }

const maxRasCategoryCount = 7

func Open() (*Client, error)     { return nil, errUnavailable }
func Available() bool            { return false }
func RasCategoryName(int) string { return "" }

func (*Client) Devices() []Device                       { return nil }
func (*Client) Energy(Power) (uint64, uint64, bool)     { return 0, 0, false }
func (*Client) Temperature(Temperature) (float64, bool) { return 0, false }
func (*Client) FrequencyState(Frequency) (freqState, uint64, bool) {
	return freqState{}, 0, false
}
func (*Client) EngineActivity(Engine) (uint64, uint64, bool) { return 0, 0, false }
func (*Client) RASCategories(RAS) ([maxRasCategoryCount]uint64, bool) {
	return [maxRasCategoryCount]uint64{}, false
}
func (*Client) MemoryState(Memory) (uint64, uint64, bool) { return 0, 0, false }
func (*Client) MemoryBandwidth(Memory) (uint64, uint64, uint64, uint64, bool) {
	return 0, 0, 0, 0, false
}
