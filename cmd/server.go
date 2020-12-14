package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jake-scott/smartthings-nest/internal/pkg/handlers"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/jake-scott/smartthings-nest/internal/pkg/sdmapi"
	"github.com/jake-scott/smartthings-nest/pkg/middlewares"
)

var _serverCmdOpts struct {
	httpsPort               uint16
	tlsCertPath             string
	tlsKeyPath              string
	oauthCallbackStateFile  string
	smartthingsClientid     string
	smartthingsClientsecret string
	gracefulTimeout         time.Duration
	readTimeout             time.Duration
	writeTimeout            time.Duration
	googleapiTImeout        time.Duration
	logRequests             bool
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the integration web server",

	RunE: func(cmd *cobra.Command, args []string) error {
		if err := doServer(); err != nil {
			return err
		}

		return nil
	},

	PreRunE: func(cmd *cobra.Command, args []string) error {
		return checkRequiredFlags("https.key", "https.cert", "smartthings.oauth-param-file")
	},
}

func init() {
	serverCmd.Flags().Uint16Var(&_serverCmdOpts.httpsPort, "https-port", 4343, "HTTP port numbers")
	serverCmd.Flags().StringVar(&_serverCmdOpts.tlsCertPath, "tls-cert", "", "TLS certificate file")
	serverCmd.Flags().StringVar(&_serverCmdOpts.tlsKeyPath, "tls-key", "", "TLS key file")
	serverCmd.Flags().DurationVar(&_serverCmdOpts.gracefulTimeout, "graceful-timeout", time.Second*15, "duration to wait for server to finish, eg. 1m or 10s")
	serverCmd.Flags().DurationVar(&_serverCmdOpts.readTimeout, "read-timeout", time.Second*15, "duration to wait for request read, eg. 1m or 10s")
	serverCmd.Flags().DurationVar(&_serverCmdOpts.writeTimeout, "write-timeout", time.Second*60, "duration to wait for request write, eg. 1m or 10s")
	serverCmd.Flags().DurationVar(&_serverCmdOpts.googleapiTImeout, "googleapi-timeout", time.Second*15, "maximum durarion of a Google API call, eg. 1m or 10s")
	serverCmd.Flags().BoolVar(&_serverCmdOpts.logRequests, "log-requests", false, "log requests and responses (only in debug mode)")
	serverCmd.Flags().StringVar(&_serverCmdOpts.oauthCallbackStateFile, "oauth-state-file", "", "File to stash callback parameters")
	serverCmd.Flags().StringVar(&_serverCmdOpts.smartthingsClientid, "smartthings-clientid", "", "oauth Client ID from Smartthings cloud connector 'App Credentials'")
	serverCmd.Flags().StringVar(&_serverCmdOpts.smartthingsClientsecret, "smartthings-clientsecret", "", "oauth Client Secret from Smartthings cloud connector 'App Credentials'")

	errPanic(viper.GetViper().BindPFlag("https.port", serverCmd.Flags().Lookup("https-port")))
	errPanic(viper.GetViper().BindPFlag("https.cert", serverCmd.Flags().Lookup("tls-cert")))
	errPanic(viper.GetViper().BindPFlag("https.key", serverCmd.Flags().Lookup("tls-key")))
	errPanic(viper.GetViper().BindPFlag("https.graceful-timeout", serverCmd.Flags().Lookup("graceful-timeout")))
	errPanic(viper.GetViper().BindPFlag("https.read-timeout", serverCmd.Flags().Lookup("read-timeout")))
	errPanic(viper.GetViper().BindPFlag("https.write-timeout", serverCmd.Flags().Lookup("write-timeout")))
	errPanic(viper.GetViper().BindPFlag("google.device-access.api-timeout", serverCmd.Flags().Lookup("googleapi-timeout")))
	errPanic(viper.GetViper().BindPFlag("logging.log-requests", serverCmd.Flags().Lookup("log-requests")))
	errPanic(viper.GetViper().BindPFlag("smartthings.oauth-param-file", serverCmd.Flags().Lookup("oauth-state-file")))
	errPanic(viper.GetViper().BindPFlag("smartthings.client-id", serverCmd.Flags().Lookup("smartthings-clientid")))
	errPanic(viper.GetViper().BindPFlag("smartthings.client-secret", serverCmd.Flags().Lookup("smartthings-clientsecret")))

	rootCmd.AddCommand(serverCmd)
}

func checkRequiredFlags(needFlags ...string) error {
	missingFlags := []string{}

	for _, f := range needFlags {
		if !viper.IsSet(f) {
			missingFlags = append(missingFlags, f)
		}
	}

	if len(missingFlags) > 0 {
		itemPlural := "item"
		if len(missingFlags) > 1 {
			itemPlural = "items"
		}
		return fmt.Errorf("required config %s `%s` not set", itemPlural, strings.Join(missingFlags, "`, `"))
	}

	return nil
}

func doServer() error {
	wait := viper.GetDuration("https.graceful-timeout")
	port := viper.GetUint("https.port")
	certFile := viper.GetString("https.cert")
	keyFile := viper.GetString("https.key")
	proj := viper.GetString("google.device-access.project")
	apiTimeout := viper.GetDuration("google.device-access.api-timeout")
	oauthFile := viper.GetString("smartthings.oauth-param-file")
	stClientID := viper.GetString("smartthings.client-id")
	stClientSecret := viper.GetString("smartthings.client-secret")

	var logRequests bool
	if viper.GetBool("logging.log-requests") {
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			logRequests = true
		} else {
			logging.Logger(nil).Warn("log-requests ignored when not in debug mode")
		}
	}

	nh := handlers.NewNestHandler(sdmapi.NewLiveClient(proj).WithTimeout(apiTimeout), oauthFile, stClientID, stClientSecret)
	oh := handlers.NewOauthHandler(proj)

	r := mux.NewRouter()
	r.Use(middlewares.NewLoggingMw(logRequests))
	r.Use(middlewares.NewRecoveryMw())
	r.Use(middlewares.NewCorrelationMw("X-Correlation-ID"))
	r.Handle("/nest", &nh).Methods(http.MethodPost)
	r.Handle("/oauth", &oh).Methods(http.MethodGet)
	r.PathPrefix("/").Handler(http.DefaultServeMux)

	s := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		ReadTimeout:  viper.GetDuration("https.read-timeout"),
		WriteTimeout: viper.GetDuration("https.write-timeout"),
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	logging.Logger(nil).Infof("Serving on port %d", port)
	go func() {
		if err := s.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			logging.Logger(nil).WithError(err).Error("running server")
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Block until we receive a signal
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	logging.Logger(nil).Info("shutting down")
	if err := s.Shutdown(ctx); err != nil {
		logging.Logger(nil).WithError(err).Errorf("shutting down")
	}
	logging.Logger(nil).Info("exiting")
	return nil
}
