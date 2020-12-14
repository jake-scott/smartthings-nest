package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/korovkin/limiter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jake-scott/smartthings-nest/generated/models"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/jake-scott/smartthings-nest/internal/pkg/pubsubapi"
	"github.com/jake-scott/smartthings-nest/internal/pkg/sdmapi"
	"github.com/jake-scott/smartthings-nest/internal/pkg/stoauth"
)

var _pubSubCmdOpts struct {
	smartthingsClientid      string
	smartthingsClientsecret  string
	smartthingsTimeout       time.Duration
	oauthCallbackStateFile   string
	sdmProjectID             string
	googlePubSubSubscription string
	googlePubSubProjectID    string
	googleCloudCredsFile     string
	maxMessageAge            time.Duration
	logMessages              bool
}

var pubSubCmd = &cobra.Command{
	Use:   "pubsub",
	Short: "Run the Google pub/sub listener",

	RunE: func(cmd *cobra.Command, args []string) error {
		if err := doPubSub(); err != nil {
			return err
		}

		return nil
	},

	PreRunE: func(cmd *cobra.Command, args []string) error {
		return checkRequiredFlags("smartthings.client-id", "smartthings.client-secret",
			"google.device-access.project", "google.pubsub.subscription-id",
			"google.creds.file")
	},
}

func init() {
	pubSubCmd.Flags().DurationVar(&_pubSubCmdOpts.smartthingsTimeout, "smartthings-timeout", time.Second*15, "maximum durarion of a Smartthings callback, eg. 1m or 10s")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.oauthCallbackStateFile, "oauth-state-file", "", "File to stash callback parameters")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.smartthingsClientid, "smartthings-clientid", "", "oauth Client ID from Smartthings cloud connector 'App Credentials'")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.smartthingsClientsecret, "smartthings-clientsecret", "", "oauth Client Secret from Smartthings cloud connector 'App Credentials'")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.sdmProjectID, "sdm-project", "", "Google Smart Device project ID from Device Access console")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.googlePubSubProjectID, "pubsub-project", "", "ID of Google cloud projcet containing the pub/sub subscription")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.googlePubSubSubscription, "pubsub-subscription", "", "Google pub/sub subscription ID")
	pubSubCmd.Flags().StringVar(&_pubSubCmdOpts.googleCloudCredsFile, "gcp-creds", "", "Google Cloud service account credentials file")
	pubSubCmd.Flags().DurationVar(&_pubSubCmdOpts.maxMessageAge, "pubsub-maxage", time.Second*1200, "maximum age of a Device Access message that we will process, eg. 1m or 10s")
	pubSubCmd.Flags().BoolVar(&_pubSubCmdOpts.logMessages, "log-messages", false, "log pubsub messages (only in debug mode)")

	errPanic(viper.GetViper().BindPFlag("smartthings.callback-timeout", pubSubCmd.Flags().Lookup("smartthings-timeout")))
	errPanic(viper.GetViper().BindPFlag("smartthings.oauth-param-file", pubSubCmd.Flags().Lookup("oauth-state-file")))
	errPanic(viper.GetViper().BindPFlag("smartthings.client-id", pubSubCmd.Flags().Lookup("smartthings-clientid")))
	errPanic(viper.GetViper().BindPFlag("smartthings.client-secret", pubSubCmd.Flags().Lookup("smartthings-clientsecret")))
	errPanic(viper.GetViper().BindPFlag("google.device-access.project", pubSubCmd.Flags().Lookup("sdm-project")))
	errPanic(viper.GetViper().BindPFlag("google.pubsub.project-id", pubSubCmd.Flags().Lookup("pubsub-project")))
	errPanic(viper.GetViper().BindPFlag("google.pubsub.subscription-id", pubSubCmd.Flags().Lookup("pubsub-subscription")))
	errPanic(viper.GetViper().BindPFlag("google.pubsub.max-message-age", pubSubCmd.Flags().Lookup("pubsub-maxage")))
	errPanic(viper.GetViper().BindPFlag("google.creds.file", pubSubCmd.Flags().Lookup("gcp-creds")))
	errPanic(viper.GetViper().BindPFlag("logging.log-messages", pubSubCmd.Flags().Lookup("log-messages")))

	rootCmd.AddCommand(pubSubCmd)
}

func pullLoop(pubsub pubsubapi.PubSub, ctx context.Context, c chan pubsubapi.SdmEvent) {
	defer close(c)

	pubsub = pubsub.WithContext(ctx)

	for {
		logging.Logger(nil).Debug("message-loop: waiting for messages")
		events, err := pubsub.Pull()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logging.Logger(nil).Infof("message-loop: shutting down")
				return
			}

			logging.Logger(nil).WithError(err).Errorf("message-loop: pulling subscription messages, sleeping 5s")
			time.Sleep(time.Second * 5)
			continue
		}

		if len(events) == 0 {
			continue
		}

		for _, event := range events {
			// Catch shutdown messages and don't block waiting for a publisher if they are busy
			select {
			case <-ctx.Done():
				break
			case c <- event:
			}
		}
	}
}

func publishLoop(maxConcurrent int, pubsub pubsubapi.PubSub, tokenState stoauth.State, c chan pubsubapi.SdmEvent) {
	limit := limiter.NewConcurrencyLimiter(maxConcurrent)

	for event := range c {
		limit.ExecuteWithTicket(func(ticket int) {
			publishEvent(ticket, pubsub, tokenState, event)
		})
	}

	logging.Logger(nil).Info("publish-loop: shutting down")
	limit.Wait()
	logging.Logger(nil).Info("publish-loop: done")
}

func makeDeviceStates(event pubsubapi.SdmEvent) []*models.DeviceStateStatesItems0 {
	nestTraits := event.Traits.TraitIDs()
	states := make([]*models.DeviceStateStatesItems0, 0, len(nestTraits))

	for _, nestTraitID := range nestTraits {
		nestTrait := event.Traits.Trait(nestTraitID)

		// Does the trait know how to expose itself to Smartthings?
		i, ok := nestTrait.(sdmapi.StCapability)
		if !ok {
			logging.Logger(nil).Debugf("Ignoring Nest trait %s, no Smartthings adapter", nestTraitID.Name())
			continue
		}

		stStates := i.ToSmartthingsState(event.Traits)
		states = append(states, stStates...)
	}

	// tack on the health check state
	deviceState := models.DeviceStateStatesItems0{
		Component:  "main",
		Capability: "st.healthcheck",
		Attribute:  "healthStatus",
		Value:      "offline",
	}
	states = append(states, &deviceState)

	timestampMillis := event.Timestamp.UnixNano() / 1000000
	for _, s := range states {
		s.Timestamp = &timestampMillis
	}

	return states
}

func executeDeviceStateCallback(tokenState stoauth.State, deviceInfo models.DeviceState) error {
	token, err := tokenState.GetAccessToken()
	if err != nil {
		return errors.Wrap(err, "fetching access token for device callback")
	}

	req := newDeviceStateCallback()
	req.DeviceState = []*models.DeviceState{&deviceInfo}
	req.Authentication.Token = &token

	reqBody, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "encoding smartthing device callback request")
	}

	logging.Logger(nil).Debugf("Sending device callback request to Smartthings URL [%s]: %s", tokenState.StateCallbackURL, reqBody)

	// Send request
	resp, err := http.Post(tokenState.StateCallbackURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return errors.Wrap(err, "executing smartthing device callback")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "reading response body")
	}

	if resp.StatusCode != 200 && resp.StatusCode == 204 {
		return errors.Wrapf(err, "non-200/204 code from Smartthings device callback URL: %d (%s): %s", resp.StatusCode, resp.Status, bodyBytes)
	}

	return nil
}

func publishEvent(ticket int, pubsub pubsubapi.PubSub, tokenState stoauth.State, event pubsubapi.SdmEvent) {
	logging.Logger(nil).Debugf("publish-goroutine %d: got %+v", ticket, event)

	deviceInfo := models.DeviceState{}
	deviceInfo.ExternalDeviceID = event.DeviceID
	deviceInfo.States = makeDeviceStates(event)

	if err := executeDeviceStateCallback(tokenState, deviceInfo); err == nil {
		if err := pubsub.AckMessages([]string{event.AckID}); err != nil {
			logging.Logger(nil).WithError(err).Error("acknowledging event")
		}
	} else {
		logging.Logger(nil).WithError(err).Error("executing Smartthings device callback")
	}

	logging.Logger(nil).Debugf("publish-goroutine %d: done", ticket)
}

func newDeviceStateCallback() models.DeviceStateCallback {
	stSchema := "st-schema"
	stVersion := "1.0"
	requestID := uuid.New().String()
	tokenType := "Bearer"

	return models.DeviceStateCallback{
		Headers: &models.Headers{
			Schema:          &stSchema,
			Version:         &stVersion,
			RequestID:       &requestID,
			InteractionType: models.InteractionTypeStateCallback,
		},
		Authentication: &models.Authentication{TokenType: &tokenType},
	}
}

func doPubSub() error {
	maxAge := viper.GetDuration("google.pubsub.max-message-age")
	sdmProject := viper.GetString("google.device-access.project")
	gcpProject := viper.GetString("google.pubsub.project-id")
	subscription := viper.GetString("google.pubsub.subscription-id")
	credsFile := viper.GetString("google.creds.file")
	oauthFile := viper.GetString("smartthings.oauth-param-file")
	clientSecret := viper.GetString("smartthings.client-secret")

	var logMesssages bool
	if viper.GetBool("logging.log-messages") {
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			logMesssages = true
		} else {
			logging.Logger(nil).Warn("log-messages ignored when not in debug mode")
		}
	}

	// context to allow us to stop the request loops
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// wait group for request loops
	var wg sync.WaitGroup

	// comms between pull and publish loops
	eventChan := make(chan pubsubapi.SdmEvent)

	// pubsub API instance
	pubsub := pubsubapi.NewLiveClient(sdmProject, gcpProject, subscription).WithMaxMessageAge(maxAge).WithServiceAccountCreds(credsFile)
	if logMesssages {
		pubsub = pubsub.(*pubsubapi.Live).WithLogMessages()
	}

	/* Start the publishing loop first */
	// load oauth data that should have been written by the web service
	tokenState := stoauth.NewState().WithClientSecret(clientSecret)
	if err := tokenState.Load(oauthFile); err != nil {
		return err
	}

	// Run the publish loop in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		publishLoop(10, pubsub, tokenState, eventChan)
	}()

	/* Start the pubsub pull loop */

	// Run the pubsub loop in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		pullLoop(pubsub, ctx, eventChan)
	}()

	// ctrl-c handler
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Block until we receive a signal
	<-c
	logging.Logger(nil).Info("main: shutting down")

	// cancel the request loop context
	cancel()

	// Wait for processing to end
	wg.Wait()

	logging.Logger(nil).Info("main: exiting")
	return nil
}
