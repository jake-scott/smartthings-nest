package sdmapi

import "time"

type Structure struct {
	ID         string
	CustomName string
	Traits     Traits
}

type Room struct {
	ID         string
	CustomName string
	Traits     Traits
}

type Device struct {
	ID         string
	DeviceType string
	Traits     Traits
}

type Command interface {
	commandName() string
}

type stCommandParamsReader interface {
	Unmarshal(args []interface{}) error
	ToSdmCommands() []Command
}

type SmartDeviceManagement interface {
	WithAccessToken(token string) SmartDeviceManagement
	WithTimeout(d time.Duration) SmartDeviceManagement
	Structures() ([]Structure, error)
	Rooms(structureID string) ([]Room, error)
	Devices() ([]Device, error)
	GetDevice(deviceID string) (*Device, error)
	SendCommand(deviceID string, command Command) error
}
