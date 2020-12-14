package sdmapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	apioption "google.golang.org/api/option"
	sdmv1 "google.golang.org/api/smartdevicemanagement/v1"
)

type Live struct {
	sdmProjectID string
	accessToken  string
	timeout      time.Duration
}

func NewLiveClient(sdmProjectID string) *Live {
	return &Live{
		sdmProjectID: "enterprises/" + sdmProjectID,
	}
}

func (c *Live) WithAccessToken(token string) SmartDeviceManagement {
	nc := *c
	nc.accessToken = token
	return &nc
}

func (c *Live) WithTimeout(d time.Duration) SmartDeviceManagement {
	nc := *c
	nc.timeout = d
	return &nc
}

func (c *Live) api() (*sdmv1.Service, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: c.accessToken})
	sdm, err := sdmv1.NewService(context.TODO(), apioption.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}

	return sdm, nil
}

func (c *Live) MakeContext() (context.Context, context.CancelFunc) {
	var ctx = context.Background()
	var cancel context.CancelFunc = func() {}
	if c.timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), c.timeout)
	}

	return ctx, cancel
}

func (c *Live) Structures() ([]Structure, error) {
	s, err := c.api()
	if err != nil {
		return nil, errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	structList, err := s.Enterprises.Structures.List(c.sdmProjectID).Context(ctx).Do()
	if err != nil {
		return nil, errors.Wrap(err, "listing structures")
	}

	var items []Structure
	for _, s := range structList.Structures {
		t := NewTraits()
		if err := t.Parse(s.Traits); err != nil {
			return nil, errors.Wrap(err, "parsing structure traits")
		}

		item := Structure{
			ID:     s.Name,
			Traits: t,
		}

		items = append(items, item)
	}

	return items, nil
}

func (c *Live) Rooms(structureID string) ([]Room, error) {
	s, err := c.api()
	if err != nil {
		return nil, errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	roomList, err := s.Enterprises.Structures.Rooms.List(structureID).Context(ctx).Do()
	if err != nil {
		return nil, errors.Wrap(err, "listing rooms")
	}

	var items []Room
	for _, r := range roomList.Rooms {
		t := NewTraits()
		if err := t.Parse(r.Traits); err != nil {
			return nil, errors.Wrap(err, "parsing room traits")
		}

		item := Room{
			ID:     r.Name,
			Traits: t,
		}

		items = append(items, item)
	}

	return items, nil
}

func (c *Live) Devices() ([]Device, error) {
	s, err := c.api()
	if err != nil {
		return nil, errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	deviceList, err := s.Enterprises.Devices.List(c.sdmProjectID).Context(ctx).Do()
	if err != nil {
		return nil, errors.Wrap(err, "listing devices")
	}

	var items []Device
	for _, d := range deviceList.Devices {
		t := NewTraits()
		if err := t.Parse(d.Traits); err != nil {
			return nil, errors.Wrap(err, "parsing device traits")
		}

		item := Device{
			ID:         c.shortDeviceName(d.Name),
			DeviceType: d.Type,
			Traits:     t,
		}

		items = append(items, item)
	}

	return items, nil
}

func (c *Live) shortDeviceName(longName string) string {
	return strings.TrimPrefix(longName, c.sdmProjectID+"/devices/")
}

func (c *Live) longDeviceName(shortName string) string {
	return c.sdmProjectID + "/devices/" + shortName
}

func (c *Live) GetDevice(deviceID string) (*Device, error) {
	s, err := c.api()
	if err != nil {
		return nil, errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	longDeviceName := c.longDeviceName(deviceID)
	device, err := s.Enterprises.Devices.Get(longDeviceName).Context(ctx).Do()
	if err != nil {
		return nil, errors.Wrap(err, "fetching device details")
	}

	t := NewTraits()
	if err := t.Parse(device.Traits); err != nil {
		return nil, errors.Wrap(err, "parsing device traits")
	}

	item := &Device{
		ID:         c.shortDeviceName(device.Name),
		DeviceType: device.Type,
		Traits:     t,
	}

	return item, nil
}

func (c *Live) SendCommand(deviceID string, command Command) error {
	s, err := c.api()
	if err != nil {
		return errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	longDeviceName := c.longDeviceName(deviceID)
	cmdParams, err := json.Marshal(command)
	if err != nil {
		return errors.Wrap(err, "marshaling command parameters")
	}

	cmdRequest := sdmv1.GoogleHomeEnterpriseSdmV1ExecuteDeviceCommandRequest{
		Command: command.commandName(),
		Params:  cmdParams,
	}

	logging.Logger(nil).Debugf("sending command: %s, params %s", cmdRequest.Command, string(cmdRequest.Params))

	resp, err := s.Enterprises.Devices.ExecuteCommand(longDeviceName, &cmdRequest).Context(ctx).Do()
	if err != nil {
		return errors.Wrapf(err, "executing command: %s, params %s", cmdRequest.Command, string(cmdRequest.Params))
	}

	if resp.HTTPStatusCode != 200 {
		return fmt.Errorf("command response error: HTTP status %d, %s", resp.HTTPStatusCode, string(resp.Results))
	}

	return nil
}
