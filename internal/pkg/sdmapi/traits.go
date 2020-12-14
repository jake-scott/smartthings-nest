package sdmapi

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
)

/*
 *   Supported Google Smart Device Management trait identifiers and names
 */

type traitID int

const (
	sdmStructuresTraitsInfo traitID = iota
	sdmStructuresTraitsRoomInfo
	sdmDevicesTraitsConnectivity
	sdmDevicesTraitsFan
	sdmDevicesTraitsHumidity
	sdmDevicesTraitsInfo
	sdmDevicesTraitsSettings
	sdmDevicesTraitsTemperature
	sdmDevicesTraitsThermostatEco
	sdmDevicesTraitsThermostatMode
	sdmDevicesTraitsThermostatHvac
	sdmDevicesTraitsThermostatTemperatureSetpoint
)

var traitNames = []string{
	"sdm.structures.traits.Info",
	"sdm.structures.traits.RoomInfo",
	"sdm.devices.traits.Connectivity",
	"sdm.devices.traits.Fan",
	"sdm.devices.traits.Humidity",
	"sdm.devices.traits.Info",
	"sdm.devices.traits.Settings",
	"sdm.devices.traits.Temperature",
	"sdm.devices.traits.ThermostatEco",
	"sdm.devices.traits.ThermostatMode",
	"sdm.devices.traits.ThermostatHvac",
	"sdm.devices.traits.ThermostatTemperatureSetpoint",
}

// convert a trait name to its ID
func parseTraitName(name string) (bool, traitID) {
	for i, val := range traitNames {
		if val == name {
			return true, traitID(i)
		}
	}

	return false, 0
}

// return the name of a trait
func (id traitID) Name() string {
	if int(id) > len(traitNames) {
		return fmt.Sprintf("unknown (id: %d)", id)
	}

	return traitNames[id]
}

// Convert a trait as read from Google, to internal representation
type traitsReader interface {
	Unmarshal() interface{}
}

// A set of traits for a device
type Traits struct {
	traits map[traitID]interface{}
}

func NewTraits() Traits {
	return Traits{
		traits: make(map[traitID]interface{}),
	}
}

// Return a list of traid IDs for the trait set
func (t *Traits) TraitIDs() []traitID {
	keys := make([]traitID, 0, len(t.traits))
	for k := range t.traits {
		keys = append(keys, k)
	}

	return keys
}

// Return the trait data from the trait set given its ID
func (t *Traits) Trait(id traitID) interface{} {
	val, ok := t.traits[id]
	if ok {
		return val
	}
	return nil
}

// Parse a set of traits from JSON into the trait set
func (t *Traits) Parse(data []byte) error {
	logging.Logger(nil).Debugf("Trait data: [%s]", data)
	var alltraits map[string]json.RawMessage
	if err := json.Unmarshal(data, &alltraits); err != nil {
		return err
	}

	for traitName, v := range alltraits {
		ok, traitID := parseTraitName(traitName)
		if !ok {
			logging.Logger(nil).Debugf("Ignoring unimplemented trait [%s]", traitName)
			continue
		}

		var decoded traitsReader
		switch traitID {
		case sdmStructuresTraitsInfo:
			decoded = &StructuresInfoTraits{}
		case sdmStructuresTraitsRoomInfo:
			decoded = &RoomInfoTraits{}
		case sdmDevicesTraitsConnectivity:
			decoded = &deviceConnectivityTraits{}
		case sdmDevicesTraitsFan:
			decoded = &deviceFanTraits{}
		case sdmDevicesTraitsHumidity:
			decoded = &DeviceHumidityTraits{}
		case sdmDevicesTraitsInfo:
			decoded = &DeviceInfoTraits{}
		case sdmDevicesTraitsSettings:
			decoded = &deviceSettingsTraits{}
		case sdmDevicesTraitsTemperature:
			decoded = &DeviceTemperatureTraits{}
		case sdmDevicesTraitsThermostatEco:
			decoded = &deviceThermostatEco{}
		case sdmDevicesTraitsThermostatMode:
			decoded = &deviceThermostatMode{}
		case sdmDevicesTraitsThermostatHvac:
			decoded = &deviceThermostatHvac{}
		case sdmDevicesTraitsThermostatTemperatureSetpoint:
			decoded = &DeviceThermostatTemperatureSetpoint{}
		}

		if decoded == nil {
			logging.Logger(nil).Debugf("Ignoring unimplemented trait [%s]", traitName)
			continue
		}

		if err := json.Unmarshal(v, &decoded); err != nil {
			return err
		}

		value := decoded.Unmarshal()
		t.traits[traitID] = value
	}

	return nil
}

type StructuresInfoTraits struct {
	CustomName string `json:"customName"`
}

func (t *StructuresInfoTraits) Unmarshal() interface{} {
	return t
}

type RoomInfoTraits struct {
	CustomName string `json:"customName"`
}

func (t *RoomInfoTraits) Unmarshal() interface{} {
	return t
}

type deviceConnectivityTraits struct {
	Status string `json:"status"`
}
type DeviceConnectivityTraits struct {
	Online bool
}

func (t *deviceConnectivityTraits) Unmarshal() interface{} {
	v := &DeviceConnectivityTraits{}
	if t.Status == "ONLINE" {
		v.Online = true
	}

	return v
}

type deviceFanTraits struct {
	TimerMode    string `json:"timerMode"`
	TimerTimeout string `json:"timerTimeout"`
}
type DeviceFanTraits struct {
	TimerModeEnabled bool
	TimerTimeout     time.Time
}

func (t *deviceFanTraits) Unmarshal() interface{} {
	v := &DeviceFanTraits{}
	if t.TimerMode == "ON" {
		v.TimerModeEnabled = true
	}
	timeout, err := time.Parse(time.RFC3339, t.TimerTimeout)
	if err == nil {
		v.TimerTimeout = timeout
	}
	return v
}

type DeviceHumidityTraits struct {
	AmbientHumidityPercent float32 `json:"ambientHumidityPercent"`
}

func (t *DeviceHumidityTraits) Unmarshal() interface{} {
	return t
}

type DeviceInfoTraits struct {
	CustomName string `json:"customName"`
}

func (t *DeviceInfoTraits) Unmarshal() interface{} {
	return t
}

type deviceSettingsTraits struct {
	TemperatureScale string `json:"temperatureScale"`
}
type TemperatureScale int

const (
	TemperatureScaleCelsius TemperatureScale = iota
	TemperatureScaleFarenheit
)

type DeviceSettingsTraits struct {
	TemperatureScale TemperatureScale
}

func (t *deviceSettingsTraits) Unmarshal() interface{} {
	v := &DeviceSettingsTraits{}
	switch t.TemperatureScale {
	case "CELSIUS":
		v.TemperatureScale = TemperatureScaleCelsius
	case "FAHRENHEIT":
		v.TemperatureScale = TemperatureScaleFarenheit
	}

	return v
}

type DeviceTemperatureTraits struct {
	AmbientTemperatureCelsius float32 `json:"ambientTemperatureCelsius"`
}

func (t *DeviceTemperatureTraits) Unmarshal() interface{} {
	return t
}

type deviceThermostatEco struct {
	AvailableModes []string `json:"availableModes"`
	Mode           string   `json:"mode"`
	HeatCelsius    float32  `json:"heatCelsius"`
	CoolCelsius    float32  `json:"coolCelsius"`
}
type DeviceThermostatEco struct {
	Enabled     bool
	HeatCelsius float32 `json:"heatCelsius"`
	CoolCelsius float32 `json:"coolCelsius"`
}

func (t *deviceThermostatEco) Unmarshal() interface{} {
	v := &DeviceThermostatEco{}
	if t.Mode == "OFF" {
		v.Enabled = false
	} else {
		v.Enabled = true
	}

	v.HeatCelsius = float32(math.Round(float64(t.HeatCelsius)*10) / 10)
	v.CoolCelsius = float32(math.Round(float64(t.CoolCelsius)*10) / 10)
	return v
}

type thermostatMode int

const (
	thermostatModeOff thermostatMode = iota
	thermostatModeHeat
	thermostatModeCool
	thermostatModeHeatCool
	thermostatModeEco
)

type deviceThermostatMode struct {
	Mode           string   `json:"mode"`
	AvailableModes []string `json:"availableModes"`
}

type DeviceThermostatMode struct {
	mode thermostatMode
}

func (t *deviceThermostatMode) Unmarshal() interface{} {
	v := &DeviceThermostatMode{}
	switch t.Mode {
	case "OFF":
		v.mode = thermostatModeOff
	case "HEAT":
		v.mode = thermostatModeHeat
	case "COOL":
		v.mode = thermostatModeCool
	case "HEATCOOL":
		v.mode = thermostatModeHeatCool
	}

	return v
}

type thermostatStatus int

const (
	thermostatStatusOff thermostatStatus = iota
	thermostatStatusHeating
	thermostatStatusCooling
)

type deviceThermostatHvac struct {
	Status string `json:"status"`
}

type DeviceThermostatHvac struct {
	status thermostatStatus
}

func (t *deviceThermostatHvac) Unmarshal() interface{} {
	v := &DeviceThermostatHvac{}
	switch t.Status {
	case "OFF":
		v.status = thermostatStatusOff
	case "HEATING":
		v.status = thermostatStatusHeating
	case "COOLING":
		v.status = thermostatStatusCooling
	}

	return v
}

type DeviceThermostatTemperatureSetpoint struct {
	HeatCelsius float32 `json:"heatCelsius"`
	CoolCelsius float32 `json:"coolCelsius"`
}

func (t *DeviceThermostatTemperatureSetpoint) Unmarshal() interface{} {
	t.HeatCelsius = float32(math.Round(float64(t.HeatCelsius)*10) / 10)
	t.CoolCelsius = float32(math.Round(float64(t.CoolCelsius)*10) / 10)
	return t
}
