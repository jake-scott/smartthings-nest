package pubsubapi

import (
	"context"
	"time"

	"github.com/jake-scott/smartthings-nest/internal/pkg/sdmapi"
)

type SdmEvent struct {
	AckID     string
	DeviceID  string
	Timestamp time.Time
	Traits    sdmapi.Traits
}

type PubSub interface {
	WithServiceAccountCreds(creds string) PubSub
	WithTimeout(d time.Duration) PubSub
	WithContext(ctx context.Context) PubSub
	Pull() ([]SdmEvent, error)
	AckMessages(ackIDs []string) error
}
