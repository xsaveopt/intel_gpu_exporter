//go:build linux

package levelzero

import (
	"errors"
	"fmt"
)

// Client holds initialised Level Zero state. Cheap to keep around for the
// lifetime of the process; the underlying library does its own caching.
type Client struct {
	devices []Device
}

// Device is one enumerated sysman device with all sub-handles pre-collected.
type Device struct {
	Handle DeviceHandle
	PCIBus string // "0000:18:00.0" — matches /sys/bus/pci/devices/<this>
	Power  []Power
	Temp   []Temperature
	Freq   []Frequency
	Engine []Engine
	RAS    []RAS
	Mem    []Memory
}

type Power struct {
	Handle      PwrHandle
	OnSubdevice bool
	SubdeviceID uint32
	MinLimitMW  int32
	MaxLimitMW  int32
}

type Temperature struct {
	Handle      TempHandle
	Sensor      string
	OnSubdevice bool
	SubdeviceID uint32
}

type Frequency struct {
	Handle      FreqHandle
	Type        uint32
	OnSubdevice bool
	SubdeviceID uint32
}

type Engine struct {
	Handle      EngineHandle
	Type        string // "compute", "render", "media", ...
	OnSubdevice bool
	SubdeviceID uint32
}

type RAS struct {
	Handle      RasHandle
	ErrorType   string // "correctable" / "uncorrectable"
	OnSubdevice bool
	SubdeviceID uint32
}

type Memory struct {
	Handle       MemHandle
	Type         string
	OnSubdevice  bool
	SubdeviceID  uint32
	PhysicalSize uint64
}

// Open dlopens libze_loader, runs zesInit, and enumerates everything.
// Returns errUnavailable if the loader cannot be found.
func Open() (*Client, error) {
	if err := loadLibrary(); err != nil {
		return nil, err
	}
	if rc := fns.zesInit(0); rc != zeResultSuccess {
		return nil, fmt.Errorf("zesInit: 0x%x", rc)
	}

	drivers, err := list(fns.zesDriverGet)
	if err != nil {
		return nil, fmt.Errorf("driver enum: %w", err)
	}
	if len(drivers) == 0 {
		return nil, errors.New("zesDriverGet returned no drivers")
	}

	c := &Client{}
	for _, drv := range drivers {
		devHandles, err := list(func(count *uint32, out *DeviceHandle) uint32 {
			return fns.zesDeviceGet(drv, count, out)
		})
		if err != nil {
			continue
		}
		for _, h := range devHandles {
			c.devices = append(c.devices, enumerateDevice(h))
		}
	}
	return c, nil
}

// Devices returns the enumerated sysman device list.
func (c *Client) Devices() []Device { return c.devices }

// Available reports whether the loader is present and at least one device was
// found. Cheap idempotent check.
func Available() bool {
	if err := loadLibrary(); err != nil {
		return false
	}
	return true
}

func enumerateDevice(h DeviceHandle) Device {
	d := Device{Handle: h}

	// PCI properties → matches sysfs paths.
	var pci pciProperties
	pci.Stype = stypePciProperties
	if rc := fns.zesDevicePciGetProperties(h, &pci); rc == zeResultSuccess {
		d.PCIBus = fmt.Sprintf("%04x:%02x:%02x.%x",
			pci.Address.Domain, pci.Address.Bus,
			pci.Address.Device, pci.Address.Function)
	}

	// Power
	if pwrs, err := list(func(c *uint32, o *PwrHandle) uint32 {
		return fns.zesDeviceEnumPowerDomains(h, c, o)
	}); err == nil {
		for _, ph := range pwrs {
			var p powerProperties
			p.Stype = stypePowerProperties
			if rc := fns.zesPowerGetProperties(ph, &p); rc == zeResultSuccess {
				d.Power = append(d.Power, Power{
					Handle:      ph,
					OnSubdevice: p.OnSubdevice != 0,
					SubdeviceID: p.SubdeviceID,
					MinLimitMW:  p.MinLimit,
					MaxLimitMW:  p.MaxLimit,
				})
			} else {
				d.Power = append(d.Power, Power{Handle: ph})
			}
		}
	}

	// Temperature
	if temps, err := list(func(c *uint32, o *TempHandle) uint32 {
		return fns.zesDeviceEnumTemperatureSensors(h, c, o)
	}); err == nil {
		for _, th := range temps {
			var p tempProperties
			p.Stype = stypeTempProperties
			t := Temperature{Handle: th, Sensor: "unknown"}
			if rc := fns.zesTemperatureGetProperties(th, &p); rc == zeResultSuccess {
				if name, ok := tempSensorNames[p.SensorType]; ok {
					t.Sensor = name
				}
				t.OnSubdevice = p.OnSubdevice != 0
				t.SubdeviceID = p.SubdeviceID
			}
			d.Temp = append(d.Temp, t)
		}
	}

	// Frequency
	if freqs, err := list(func(c *uint32, o *FreqHandle) uint32 {
		return fns.zesDeviceEnumFrequencyDomains(h, c, o)
	}); err == nil {
		for _, fh := range freqs {
			var p freqProperties
			p.Stype = stypeFreqProperties
			f := Frequency{Handle: fh}
			if rc := fns.zesFrequencyGetProperties(fh, &p); rc == zeResultSuccess {
				f.Type = p.Type
				f.OnSubdevice = p.OnSubdevice != 0
				f.SubdeviceID = p.SubdeviceID
			}
			d.Freq = append(d.Freq, f)
		}
	}

	// Engine groups
	if engs, err := list(func(c *uint32, o *EngineHandle) uint32 {
		return fns.zesDeviceEnumEngineGroups(h, c, o)
	}); err == nil {
		for _, eh := range engs {
			var p engineProperties
			p.Stype = stypeEngineProperties
			e := Engine{Handle: eh, Type: "unknown"}
			if rc := fns.zesEngineGetProperties(eh, &p); rc == zeResultSuccess {
				e.Type = engineTypeName(p.Type)
				e.OnSubdevice = p.OnSubdevice != 0
				e.SubdeviceID = p.SubdeviceID
			}
			d.Engine = append(d.Engine, e)
		}
	}

	// RAS error sets
	if rasSets, err := list(func(c *uint32, o *RasHandle) uint32 {
		return fns.zesDeviceEnumRasErrorSets(h, c, o)
	}); err == nil {
		for _, rh := range rasSets {
			var p rasProperties
			p.Stype = stypeRasProperties
			r := RAS{Handle: rh, ErrorType: "unknown"}
			if rc := fns.zesRasGetProperties(rh, &p); rc == zeResultSuccess {
				r.ErrorType = rasTypeName(p.Type)
				r.OnSubdevice = p.OnSubdevice != 0
				r.SubdeviceID = p.SubdeviceID
			}
			d.RAS = append(d.RAS, r)
		}
	}

	// Memory modules
	if mems, err := list(func(c *uint32, o *MemHandle) uint32 {
		return fns.zesDeviceEnumMemoryModules(h, c, o)
	}); err == nil {
		for _, mh := range mems {
			var p memProperties
			p.Stype = stypeMemProperties
			m := Memory{Handle: mh, Type: "unknown"}
			if rc := fns.zesMemoryGetProperties(mh, &p); rc == zeResultSuccess {
				if name, ok := memTypeNames[p.Type]; ok {
					m.Type = name
				}
				m.OnSubdevice = p.OnSubdevice != 0
				m.SubdeviceID = p.SubdeviceID
				m.PhysicalSize = p.PhysicalSize
			}
			d.Mem = append(d.Mem, m)
		}
	}

	return d
}

// ----- per-scrape readers -----

// EnergyMicrojoules / TimestampMicros — accumulators for delta computation
// in PromQL via rate(). Returns 0,0,false if the call fails.
func (c *Client) Energy(p Power) (uint64, uint64, bool) {
	var cnt powerEnergyCounter
	cnt.Stype = stypePowerEnergyCounter
	if rc := fns.zesPowerGetEnergyCounter(p.Handle, &cnt); rc != zeResultSuccess {
		return 0, 0, false
	}
	return cnt.Energy, cnt.Timestamp, true
}

func (c *Client) Temperature(t Temperature) (float64, bool) {
	var v float64
	if rc := fns.zesTemperatureGetState(t.Handle, &v); rc != zeResultSuccess {
		return 0, false
	}
	return v, true
}

func (c *Client) FrequencyState(f Frequency) (state freqState, throttleNS uint64, ok bool) {
	state.Stype = stypeFreqState
	if rc := fns.zesFrequencyGetState(f.Handle, &state); rc != zeResultSuccess {
		return state, 0, false
	}
	var tt freqThrottleTime
	tt.Stype = stypeFreqThrottleTime
	if rc := fns.zesFrequencyGetThrottleTime(f.Handle, &tt); rc == zeResultSuccess {
		throttleNS = tt.ThrottleTime
	}
	return state, throttleNS, true
}

// FreqMHz / Actual / Request — convenience extractors.
func (s freqState) ActualMHz() float64  { return s.Actual }
func (s freqState) RequestMHz() float64 { return s.Request }
func (s freqState) TDPMHz() float64     { return s.TDP }

func (c *Client) EngineActivity(e Engine) (active uint64, ts uint64, ok bool) {
	var s engineStats
	s.Stype = stypeEngineStats
	if rc := fns.zesEngineGetActivity(e.Handle, &s); rc != zeResultSuccess {
		return 0, 0, false
	}
	return s.ActiveTime, s.Timestamp, true
}

func (c *Client) RASCategories(r RAS) ([maxRasCategoryCount]uint64, bool) {
	var s rasState
	s.Stype = stypeRasState
	if rc := fns.zesRasGetState(r.Handle, 0, &s); rc != zeResultSuccess {
		return [maxRasCategoryCount]uint64{}, false
	}
	return s.Category, true
}

// RasCategoryName exposes the categories[i] -> Prometheus label mapping.
func RasCategoryName(i int) string {
	if i < 0 || i >= maxRasCategoryCount {
		return "unknown"
	}
	return rasCategoryNames[i]
}

func (c *Client) MemoryState(m Memory) (size uint64, free uint64, ok bool) {
	var s memState
	s.Stype = stypeMemState
	if rc := fns.zesMemoryGetState(m.Handle, &s); rc != zeResultSuccess {
		return 0, 0, false
	}
	return s.Size, s.Free, true
}

func (c *Client) MemoryBandwidth(m Memory) (read, write, max, ts uint64, ok bool) {
	var b memBandwidth
	b.Stype = stypeMemBandwidth
	if rc := fns.zesMemoryGetBandwidth(m.Handle, &b); rc != zeResultSuccess {
		return 0, 0, 0, 0, false
	}
	return b.ReadCounter, b.WriteCounter, b.MaxBandwidth, b.Timestamp, true
}
