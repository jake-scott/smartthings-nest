package sdmapi

import (
	"github.com/jake-scott/smartthings-nest/generated/models"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
)

//  Convert data to a set of SmartThings device states
//  ToSmartThingsState() may use data from other traits to compose the device state
type StCapability interface {
	ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0
}

func (t DeviceConnectivityTraits) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	var status string
	switch t.Online {
	case true:
		status = "online"
	default:
		status = "offline"
	}

	model := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.healthCheck",
		Attribute:  "healthStatus",
		Value:      status,
	}

	return []*models.DeviceStateStatesItems0{&model}
}

func (t DeviceFanTraits) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	var stFanMode string
	switch t.TimerModeEnabled {
	case true:
		stFanMode = "followschedule"
	default:
		stFanMode = "auto"
	}

	supportedFanModes := []string{"auto", "followschedule"}
	// no supported fan modes if thermostat mode is OFF and not in Eco mode
	modeTrait := traits.Trait(sdmDevicesTraitsThermostatMode)
	ecoTrait := traits.Trait(sdmDevicesTraitsThermostatEco)

	isEco := false
	if ecoTrait != nil && ecoTrait.(*DeviceThermostatEco).Enabled {
		isEco = true
	}

	if modeTrait != nil && modeTrait.(*DeviceThermostatMode).mode == thermostatModeOff {
		if !isEco {
			logging.Logger(nil).Debug("Overriding available fan modes to none (thermostat mode is OFF, eco is OFF)")
			supportedFanModes = []string{"auto"}
		}
	}

	model1 := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.thermostatFanMode",
		Attribute:  "thermostatFanMode",
		Value:      stFanMode,
	}
	model2 := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.thermostatFanMode",
		Attribute:  "supportedThermostatFanModes",
		Value:      supportedFanModes,
	}

	return []*models.DeviceStateStatesItems0{&model1, &model2}
}

func (t DeviceHumidityTraits) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	model := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.relativeHumidityMeasurement",
		Attribute:  "humidity",
		Value:      t.AmbientHumidityPercent,
	}

	return []*models.DeviceStateStatesItems0{&model}
}

func (t DeviceTemperatureTraits) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	model := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.temperatureMeasurement",
		Attribute:  "temperature",
		Value:      t.AmbientTemperatureCelsius,
		DeviceStateStatesItems0AdditionalProperties: make(map[string]interface{}),
	}

	model.DeviceStateStatesItems0AdditionalProperties["unit"] = "C"

	return []*models.DeviceStateStatesItems0{&model}
}

func (t DeviceThermostatMode) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	var mode string
	switch t.mode {
	case thermostatModeOff:
		mode = "off"
	case thermostatModeHeat:
		mode = "heat"
	case thermostatModeCool:
		mode = "cool"
	case thermostatModeHeatCool:
		mode = "auto"
	}

	// Express mode as 'eco' if Nest is in Eco mode
	ecoTrait := traits.Trait(sdmDevicesTraitsThermostatEco)
	if ecoTrait != nil && ecoTrait.(*DeviceThermostatEco).Enabled {
		logging.Logger(nil).Debugf("Overriding mode %s to eco", mode)
		mode = "eco"
	}

	model1 := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.thermostatMode",
		Attribute:  "thermostatMode",
		Value:      mode,
	}

	model2 := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.thermostatMode",
		Attribute:  "supportedThermostatModes",
		Value:      []string{"off", "heat", "cool", "auto", "eco"},
	}

	return []*models.DeviceStateStatesItems0{&model1, &model2}
}

func (t DeviceThermostatHvac) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	var status string
	switch t.status {
	case thermostatStatusOff:
		status = "idle"
	case thermostatStatusHeating:
		status = "heating"
	case thermostatStatusCooling:
		status = "cooling"
	}

	// Express status as 'fan only' if Nest is on fan-timer mode
	fanTrait := traits.Trait(sdmDevicesTraitsFan)
	if fanTrait != nil && fanTrait.(*DeviceFanTraits).TimerModeEnabled {
		logging.Logger(nil).Debugf("Overriding HVAC status %s to `fan only``", status)
		status = "fan only"
	}

	model := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.thermostatOperatingState",
		Attribute:  "thermostatOperatingState",
		Value:      status,
	}

	return []*models.DeviceStateStatesItems0{&model}
}

func (t DeviceThermostatTemperatureSetpoint) ToSmartthingsState(traits Traits) []*models.DeviceStateStatesItems0 {
	var modelList []*models.DeviceStateStatesItems0

	var coolPoint, heatPoint float32

	// Refleft the Eco settings if Eco mode is available is active
	ecoTrait := traits.Trait(sdmDevicesTraitsThermostatEco)
	if ecoTrait != nil && ecoTrait.(*DeviceThermostatEco).Enabled {
		coolPoint = ecoTrait.(*DeviceThermostatEco).CoolCelsius
		heatPoint = ecoTrait.(*DeviceThermostatEco).HeatCelsius
		logging.Logger(nil).Debugf("Overriding setpoints %f/%f to eco setpoints %f/%f", t.CoolCelsius, t.HeatCelsius, coolPoint, heatPoint)
	} else {
		coolPoint = t.CoolCelsius
		heatPoint = t.HeatCelsius
	}

	if coolPoint > 0 {
		model := models.DeviceStateStatesItems0{
			Component:  "main",
			Capability: "st.thermostatCoolingSetpoint",
			Attribute:  "coolingSetpoint",
			Value:      coolPoint,
			DeviceStateStatesItems0AdditionalProperties: make(map[string]interface{}),
		}
		model.DeviceStateStatesItems0AdditionalProperties["unit"] = "C"
		modelList = append(modelList, &model)
	}

	if heatPoint > 0 {
		model := models.DeviceStateStatesItems0{
			Component:  "main",
			Capability: "st.thermostatHeatingSetpoint",
			Attribute:  "heatingSetpoint",
			Value:      heatPoint,
			DeviceStateStatesItems0AdditionalProperties: make(map[string]interface{}),
		}
		model.DeviceStateStatesItems0AdditionalProperties["unit"] = "C"
		modelList = append(modelList, &model)
	}

	return modelList
}
