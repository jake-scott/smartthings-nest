package pubsubapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/jake-scott/smartthings-nest/internal/pkg/sdmapi"
	"github.com/pkg/errors"
	apioption "google.golang.org/api/option"
	pubsubv1 "google.golang.org/api/pubsub/v1"
)

type Live struct {
	sdmProjectID   string
	gcpProjectID   string
	subscriptionID string
	credsFile      string
	timeout        time.Duration
	maxMessageAge  time.Duration
	logMessages    bool
	ctx            context.Context
}

func NewLiveClient(sdmProjectID string, gcpProjectID string, subscriptionID string) *Live {
	return &Live{
		sdmProjectID:   sdmProjectID,
		gcpProjectID:   gcpProjectID,
		subscriptionID: subscriptionID,
		maxMessageAge:  time.Second * 120,
		ctx:            context.Background(),
	}
}

func (c *Live) WithServiceAccountCreds(credsFile string) PubSub {
	nc := *c
	nc.credsFile = credsFile
	return &nc
}

func (c *Live) WithTimeout(d time.Duration) PubSub {
	nc := *c
	nc.timeout = d
	return &nc
}

func (c *Live) WithContext(ctx context.Context) PubSub {
	nc := *c
	nc.ctx = ctx
	return &nc
}

func (c *Live) WithMaxMessageAge(d time.Duration) *Live {
	nc := *c
	nc.maxMessageAge = d
	return &nc
}

func (c *Live) WithLogMessages() *Live {
	nc := *c
	nc.logMessages = true
	return &nc
}

func (c *Live) api() (*pubsubv1.Service, error) {
	pubsub, err := pubsubv1.NewService(context.TODO(), apioption.WithCredentialsFile(c.credsFile))
	if err != nil {
		return nil, err
	}

	return pubsub, nil
}

func (c *Live) MakeContext() (context.Context, context.CancelFunc) {
	var ctx = c.ctx
	var cancel context.CancelFunc = func() {}
	if c.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
	}

	return ctx, cancel
}

/*
  Message format for resource updates:

{
	"eventId" : "0120ecc7-3b57-4eb4-9941-91609f189fb4",
	"timestamp" : "2019-01-01T00:00:01Z",
	"resourceUpdate" : {
	  "name" : "enterprises/project-id/devices/device-id",
	  "traits" : {
		"sdm.devices.traits.ThermostatMode" : {
		  "mode" : "COOL"
		}
	  }
	},
	"userId": "AVPHwEuBfnPOnTqzVFT4IONX2Qqhu9EJ4ubO-bNnQ-yi"
}
*/

type sdmResourceUpdate struct {
	Name   string          `json:"name"`
	Traits json.RawMessage `json:"traits"`
}

type sdmEvent struct {
	EventID        string             `json:"eventID"`
	Timestamp      time.Time          `json:"timestamp"`
	ResourceUpdate *sdmResourceUpdate `json:"resourceUpdate,omitempty"`
	UserID         string             `json:"userId"`
}

func (c *Live) AckMessages(ackIDs []string) error {
	s, err := c.api()
	if err != nil {
		return errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	ackRequest := pubsubv1.AcknowledgeRequest{
		AckIds: ackIDs,
	}

	subsID := "projects/" + c.gcpProjectID + "/subscriptions/" + c.subscriptionID

	_, err = s.Projects.Subscriptions.Acknowledge(subsID, &ackRequest).Context(ctx).Do()
	if err != nil {
		return errors.Wrap(err, "executing acknowledge call")
	}

	logging.Logger(nil).Debugf("sent ACK %v", ackIDs)

	return nil
}

func (c *Live) parseReceivedMessages(messages []*pubsubv1.ReceivedMessage) (toAck []string, events []SdmEvent, err error) {
	for _, message := range messages {
		logging.Logger(nil).Infof("pubsub message: ID %s, Devliery attempt %d", message.Message.MessageId, message.DeliveryAttempt)

		// event data is base64 encoded
		data, err := base64.StdEncoding.DecodeString(message.Message.Data)
		if err != nil {
			logging.Logger(nil).WithError(err).Error("decoding base64-encoded data field")
			continue
		}
		if c.logMessages {
			logging.Logger(nil).Debugf("message data (ID %s): %s", message.Message.MessageId, data)
		}

		// retreieve the message publish time
		publishTime, err := time.Parse(time.RFC3339Nano, message.Message.PublishTime)
		if err != nil {
			logging.Logger(nil).WithError(err).Warnf("parsing message publish time (`%s`)", message.Message.PublishTime)
		} else {
			if time.Now().After(publishTime.Add(c.maxMessageAge)) {
				logging.Logger(nil).Warnf("ignoring message ID %s, older than %s (%s)", message.Message.MessageId, c.maxMessageAge, publishTime)
				toAck = append(toAck, message.AckId)
				continue
			}
		}

		event := sdmEvent{}
		if err := json.Unmarshal(data, &event); err != nil {
			logging.Logger(nil).WithError(err).Error("parsing SDM event")
			continue
		}

		if event.ResourceUpdate == nil {
			logging.Logger(nil).Warnf("ignoring message ID %s, not a resource update (%s)", message.Message.MessageId, message.Message.Data)
			toAck = append(toAck, message.AckId)
			continue
		}

		t := sdmapi.NewTraits()
		if err := t.Parse(event.ResourceUpdate.Traits); err != nil {
			logging.Logger(nil).WithError(err).Error("parsing device traits")
			continue
		}

		parsedEvent := SdmEvent{
			AckID:     message.AckId,
			Timestamp: event.Timestamp,
			DeviceID:  c.shortDeviceName(event.ResourceUpdate.Name),
			Traits:    t,
		}
		events = append(events, parsedEvent)
	}

	return
}

func (c *Live) shortDeviceName(longName string) string {
	return strings.TrimPrefix(longName, "enterprises/"+c.sdmProjectID+"/devices/")
}

func (c *Live) Pull() ([]SdmEvent, error) {
	s, err := c.api()
	if err != nil {
		return nil, errors.Wrap(err, "initialising the api")
	}

	ctx, cancel := c.MakeContext()
	defer cancel()

	pullRequest := pubsubv1.PullRequest{
		MaxMessages: 10,
	}

	subsID := "projects/" + c.gcpProjectID + "/subscriptions/" + c.subscriptionID

	response, err := s.Projects.Subscriptions.Pull(subsID, &pullRequest).Context(ctx).Do()
	if err != nil {
		return nil, errors.Wrap(err, "pulling messages from topic subscription")
	}

	messagesToAck, events, err := c.parseReceivedMessages(response.ReceivedMessages)

	// Ack messages we declined to process
	if len(messagesToAck) > 0 {
		if err := c.AckMessages(messagesToAck); err != nil {
			logging.Logger(nil).WithError(err).Warnf("acknowledging %d old messages", len(messagesToAck))
		}
	}

	return events, err
}
