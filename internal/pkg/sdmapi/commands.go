package sdmapi

import (
	"fmt"
	"time"

	"github.com/jake-scott/smartthings-nest/generated/models"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/pkg/errors"
)

type command struct {
	command string `json:"-"`
}

func newCommand(name string) command {
	return command{
		command: name,
	}
}

func (c command) commandName() string {
	return c.command
}

type devicesFanCommandParams struct {
	command
	TimerMode string `json:"timerMode"`
	Duration  string `json:"duration,omitempty"`
}

func NewFanCommand(timerEnabled bool, duration time.Duration) Command {
	mode := "OFF"
	var durString string
	if timerEnabled {
		mode = "ON"
		durString = fmt.Sprintf("%.0fs", duration.Seconds())
	}

	return devicesFanCommandParams{
		command:   newCommand("sdm.devices.commands.Fan.SetTimer"),
		TimerMode: mode,
		Duration:  durString,
	}
}

type devicesThermostatEcoCommandParams struct {
	command
	Mode string `json:"mode"`
}

func NewThermostatEcoCommand(enabled bool) Command {
	mode := "OFF"
	if enabled {
		mode = "MANUAL_ECO"
	}

	return devicesThermostatEcoCommandParams{
		command: newCommand("sdm.devices.commands.ThermostatEco.SetMode"),
		Mode:    mode,
	}
}

type devicesThermostatModeCommandParams struct {
	command
	Mode string `json:"mode"`
}

func NewThermostatModeCommand(mode thermostatMode) Command {
	var modeStr string
	switch mode {
	case thermostatModeOff:
		modeStr = "OFF"
	case thermostatModeCool:
		modeStr = "COOL"
	case thermostatModeHeat:
		modeStr = "HEAT"
	case thermostatModeHeatCool:
		modeStr = "HEATCOOL"
	}

	return devicesThermostatModeCommandParams{
		command: newCommand("sdm.devices.commands.ThermostatMode.SetMode"),
		Mode:    modeStr,
	}
}

type devicesThermostatTemperatureSetpointHeatCommandParams struct {
	command
	HeatCelsius float32 `json:"heatCelsius"`
}
type devicesThermostatTemperatureSetpointCoolCommandParams struct {
	command
	CoolCelsius float32 `json:"coolCelsius"`
}
type devicesThermostatTemperatureSetpointRangeCommandParams struct {
	command
	HeatCelsius float32 `json:"heatCelsius"`
	CoolCelsius float32 `json:"coolCelsius"`
}

func NewThermostatTemperatureSetpointHeatCommand(temp float32) Command {
	return devicesThermostatTemperatureSetpointHeatCommandParams{
		command:     newCommand("sdm.devices.commands.ThermostatTemperatureSetpoint.SetHeat"),
		HeatCelsius: temp,
	}
}
func NewThermostatTemperatureSetpointCoolCommand(temp float32) Command {
	return devicesThermostatTemperatureSetpointCoolCommandParams{
		command:     newCommand("sdm.devices.commands.ThermostatTemperatureSetpoint.SetCool"),
		CoolCelsius: temp,
	}
}
func NewThermostatTemperatureSetpointRangeCommand(heatTemp, coolTemp float32) Command {
	return devicesThermostatTemperatureSetpointRangeCommandParams{
		command:     newCommand("sdm.devices.commands.ThermostatTemperatureSetpoint.SetRange"),
		HeatCelsius: heatTemp,
		CoolCelsius: coolTemp,
	}
}

func StCommandToSdmCommands(stCommand *models.Command) ([]Command, error) {
	var stArgs stCommandParamsReader

	switch *stCommand.Capability {
	case "st.thermostatFanMode":
		stArgs = &stCommandThermostatFanModeSetThermostatFanMode{}
	case "st.thermostatMode":
		stArgs = &stCommandThermostatModeSetMode{}
	case "st.thermostatHeatingSetpoint":
		stArgs = &stCommandThermostatHeatingSetpoint{}
	case "st.thermostatCoolingSetpoint":
		stArgs = &stCommandThermostatCoolingSetpoint{}
	}

	if stArgs == nil {
		logging.Logger(nil).Debugf("Ignoring unimplemented Smartthings capability [%s]", stCommand.Capability)
		return nil, nil
	}

	if err := stArgs.Unmarshal(stCommand.Arguments); err != nil {
		return nil, errors.Wrap(err, "unmarshaling smartthings command arguments")
	}

	return stArgs.ToSdmCommands(), nil
}

type stCommandThermostatModeSetMode struct {
	mode thermostatMode
}

func (t *stCommandThermostatModeSetMode) Unmarshal(args []interface{}) error {
	if len(args) > 1 {
		return fmt.Errorf("expected 1 argument for CommandThermostatModeSetMode, got %d", len(args))
	}

	stMode, ok := args[0].(string)
	if !ok {
		return fmt.Errorf("expected string argument 'mode'")
	}

	switch stMode {
	case "off":
		t.mode = thermostatModeOff
	case "heat":
		t.mode = thermostatModeHeat
	case "cool":
		t.mode = thermostatModeCool
	case "auto":
		t.mode = thermostatModeHeatCool
	case "eco":
		t.mode = thermostatModeEco
	}

	return nil
}

func (t *stCommandThermostatModeSetMode) ToSdmCommands() []Command {
	commands := make([]Command, 0, 2)

	if t.mode == thermostatModeEco {
		commands = append(commands, NewThermostatEcoCommand(true))
	} else {
		commands = append(commands, NewThermostatEcoCommand(false))
		commands = append(commands, NewThermostatModeCommand(t.mode))
	}

	return commands
}

type stCommandThermostatFanModeSetThermostatFanMode struct {
	timerModeEnabled bool
}

func (t *stCommandThermostatFanModeSetThermostatFanMode) Unmarshal(args []interface{}) error {
	if len(args) != 1 {
		return fmt.Errorf("expected 1 argument for CommandThermostatFanModeSetThermostatFanMode, got %d", len(args))
	}

	stMode, ok := args[0].(string)
	if !ok {
		return fmt.Errorf("expected string argument, have : %+v", args[0])
	}

	switch stMode {
	case "followschedule":
		t.timerModeEnabled = true
	case "auto":
		t.timerModeEnabled = false
	default:
		return fmt.Errorf("unsupported fan mode: %s", stMode)
	}

	return nil
}

func (t *stCommandThermostatFanModeSetThermostatFanMode) ToSdmCommands() []Command {
	// TODO: find a way to let the user choose the duration
	return []Command{NewFanCommand(t.timerModeEnabled, time.Second*3600)}
}

type stCommandThermostatHeatingSetpoint struct {
	temperature float32
}

func (t *stCommandThermostatHeatingSetpoint) Unmarshal(args []interface{}) error {
	if len(args) != 1 {
		return fmt.Errorf("expected 1 argument for CommandThermostatHeatingSetpoint, got %d", len(args))
	}

	switch v := args[0].(type) {
	case float64:
		t.temperature = float32(v)
	case int:
		t.temperature = float32(v)
	default:
		return fmt.Errorf("expected float32 or int argument, have : %T : %+v", args[0], args[0])
	}

	return nil
}

func (t *stCommandThermostatHeatingSetpoint) ToSdmCommands() []Command {
	return []Command{NewThermostatTemperatureSetpointHeatCommand(t.temperature)}
}

type stCommandThermostatCoolingSetpoint struct {
	temperature float32
}

func (t *stCommandThermostatCoolingSetpoint) Unmarshal(args []interface{}) error {
	if len(args) != 1 {
		return fmt.Errorf("expected 1 argument for CommandThermostatCoolingSetpoint, got %d", len(args))
	}

	switch v := args[0].(type) {
	case float64:
		t.temperature = float32(v)
	case int:
		t.temperature = float32(v)
	default:
		return fmt.Errorf("expected float32 or int argument, have : %T : %+v", args[0], args[0])
	}

	return nil
}

func (t *stCommandThermostatCoolingSetpoint) ToSdmCommands() []Command {
	return []Command{NewThermostatTemperatureSetpointCoolCommand(t.temperature)}
}
