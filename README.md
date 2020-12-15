# SmartThings Cloud Connector for Google Nest

## Description

This project aimed to provide an integration between Google Nest and the SmartThings ecosystem.
Since we started, the two companies have announced their own integration so this project is
on hold, but has been published in the hope that it might be useful to folks implementing
other integrations with either of those platforms.

## Building

From source :

    $ git clone https://github.com/jake-scott/smartthings-nest.git
    $ cd smartthings-nest
    $ go install

or have Go install it for you :

    $ go get -u github.com/jake-scott/smartthings-nest@v0.0.1

The binary will be installed as `$GOPATH/bin/smartthings-nest` (usually ~/go/bin/smartthings-nest).


## Pre-requsites

The adapter requires a number of Samsung and Google dependencies to be set up prior operation:

   * Google Cloud (GCP) account and project
   * GCP Oauth Client registration that facilitates SmartThings obtaining authentication tokens for the Google Smart Device API on the Nest user's behalf
   * Google Device Access API registration
   * SmartThings developer account and workspace
   * SmartThings _Cloud Connector_ project
   * SmartThings device profile
   * A TLS certificate for this connector


### Google Cloud account and project

   * Browse to https://console.cloud.google.com/project and login using an existing Google account (or create a new one)
   * Click `Create Project`, enter a suitable name and accept


### Google Cloud Oauth Client ID

  * Open a browser at https://console.cloud.google.com/apis/credentials
      * Choose *Create Credential* -> *Oauth client ID*
      *  Application type: Web application
      *  Name - choose something sensible
      *  Authorised redirect URIs:
         *  https://c2c-us.smartthings.com/oauth/callback
         *  https://c2c-eu.smartthings.com/oauth/callback
         *  https://c2c-ap.smartthings.com/oauth/callback
  * Record the Client ID and Client Secret displayed at the top right of the form


### Google Device Access API registration

   * Register for access to the device management API, and pay the $5 fee :
      * https://developers.google.com/nest/device-access/registration
   * Open the Device Access Console
      * https://console.nest.google.com/device-access
   * Create Project
      * Choose a suitable name
      * Use the OAuth Client ID created in the previous step
      * Choose to enable events
   * Make a note of the project ID and pub/sub topic name


### SmartThings developer account and workspace

   * Sign up for a developer account at https://smartthings.developer.samsung.com


### SmartThings _Cloud Connector_ project
   * Create a new project at https://smartthings.developer.samsung.com/workspace/projects
     * Select `Device Integration`
     * Choose `SmartThings Cloud Connector`
     * Choose `SmartThings Schema Connector`
   * Register the app (in the project Overview pane)
     * Select `WebHook Endpoint`
     * The target URL is the location you will deploy this application, ending in /nest, eg https://test.foo.com/nest
     * The Credential options are as follows :

| Config option                     | Description |
| -----                             | ---- |
| Client ID                         | ClientID from *Google Cloud Oauth Client ID* step |
| Client Secret                     | Client secret *Google Cloud Oauth Client ID* step |
| Authorization URI                 | https://*yourservice.domain.com:port*/oauth |
| Token URI                         | https://oauth2.googleapis.com/token |
| OAuth Scopes                      | https://www.googleapis.com/auth/sdm.service |


Note that you need a certificate issued by a public CA for the hostname specified in the webhook target URL.


### SmartThings device profile

The `smartthings-device-config.json` file in the source contains the device profile to use in SmartThings.

It is a basic device profile with a single componeng (`main`) and the following capabilities:

   * Thermostat Mode
   * Thermostat Heating Setpoint
   * Thermostat Cooling Setpoint
   * Thermostat Fan Mode
   * Thermostat Operating State
   * Temperature Measurement
   * Relative Humidity Measurement
   * Health Check

Record the device profile ID.


## Configuration

Most options can be supplied via the command line or read from a Yaml file.  See sample-config.yml
for an idea of what is configurable.

The web service (web hook) needs:

| Config option                     | Description |
| -----                             | ---- |
| https.port                        | The port to listen on |
| https.cert                        | PEM encoded TLS certificate and chain |
| https.key                         | PEM encoded private key |
| google.device-access.project      | The Smart Device Management project ID |
| smartthings.oauth-param-file      | File to cache SmartThings callback information |
| smartthings.client-id             | SmartThings client ID from the Cloud Connector registration |
| smartthings.client-secret         | SmartThings client secret from the Cloud Connector registration |


The pubsub server needs:

| Config option                     | Description |
| -----                             | ---- |
| google.device-access.project      | The Smart Device Management project ID |
| google.creds.file                 | Google cloud service account credentials with pub/sub subscription access |
| google.pubsub.subscription-id     | Name of the GCP pub/sub subscription for the Smart Device topic |
| smartthings.oauth-param-file      | File to cache SmartThings callback information |
| smartthings.client-id             | SmartThings client ID from the Cloud Connector registration |
| smartthings.client-secret         | SmartThings client secret from the Cloud Connector registration |



## Running the web service

    $ smartthings-nest server --config app.yml

The server can be run in debug mode (`-d`) and can be made to log requests and responses (`--log-requests`)


Once enabled in developer mode, you should be able to add a new Nest device using the SmartThings mobile app.  The first time, this will redrect you to the Google oauth consent screen, and you should see a request in the web service log.  Follow the instructions and there should then be a flurry of request in the web service log as SmartThings requests a device discovery and state refresh.  It will also send a
callback request that will cause the web service to fetch an Oauth refresh and access token from SmartThings, and that will be stored in the file referenced by the *smartthings.oauth-param-file* config parameter.


## Running the pub/sub service

    $ smartthings-nest pubsub --config app.yml

The pubsub service requres the Oauth credentials that the web service writes.

Although the pub/sub service can receive messages and publigh them to Smartthings, the mobile app does not presently seem to pick up the state events. *Not sure if this is a bug with ST or my code at this stage*
