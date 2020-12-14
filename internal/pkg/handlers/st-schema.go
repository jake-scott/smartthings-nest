package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jake-scott/smartthings-nest/generated/models"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/jake-scott/smartthings-nest/internal/pkg/sdmapi"
	"github.com/jake-scott/smartthings-nest/internal/pkg/stoauth"
	"google.golang.org/api/googleapi"
)

const (
	StNestThermostatDeviceProfileID string = "bd2e8c4a-0e4b-475f-b8ff-273fb5f5cef5"
)

type NestHandler struct {
	sdmClient      sdmapi.SmartDeviceManagement
	oauthStateFile string
	stClientID     string
	stClientSecret string
}

func NewNestHandler(cli sdmapi.SmartDeviceManagement, oauthStateFile string, clientID string, clientSecret string) NestHandler {
	return NestHandler{
		sdmClient:      cli,
		oauthStateFile: oauthStateFile,
		stClientID:     clientID,
		stClientSecret: clientSecret,
	}
}

func (h *NestHandler) sendJSONResponse(w http.ResponseWriter, r *http.Request, d interface{}) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if err := enc.Encode(d); err != nil {
		logging.Logger(r.Context()).WithError(err).Error("sending json response")
	}
}

func googleApiErrorIs(err *googleapi.Error, reason string) bool {
	for _, i := range err.Errors {
		if i.Reason == reason {
			return true
		}
	}

	return false
}

func googleApiErrorIsGlobal(err error, requestWasForDevice bool) bool {
	switch v := err.(type) {
	case *googleapi.Error:
		switch v.Code {
		case 401:
			return true
		case 400:
			if requestWasForDevice {
				return false
			} else {
				return true
			}
		default:
			return true
		}
	}

	return true
}

func makeDeviceError(err error) models.DeviceStateDeviceErrorItems0 {
	errEnum := "DEVICE-UNAVAILABLE"
	errDetail := "device unavailable"

	switch v := err.(type) {
	case *googleapi.Error:
		errDetail = v.Message

		switch {
		case googleApiErrorIs(v, "failedPrecondition"):
			errEnum = "RESOURCE-CONSTRAINT-VIOLATION"
		}
	}

	deviceError := models.DeviceStateDeviceErrorItems0{
		Detail:    errDetail,
		ErrorEnum: &errEnum,
	}

	return deviceError
}

func (h *NestHandler) sendAPIErrorResponse(w http.ResponseWriter, r *http.Request, req models.SmartthingsRequest, err error) {
	logging.Logger(r.Context()).WithError(err).Errorf("querying Google SDM API : %s", err)

	//lint:
	switch v := err.(type) {
	case *googleapi.Error:
		if v.Code == 401 {
			// Assume token has expired, we can't tell..
			h.sendJSONResponse(w, r, NewGlobalErrorResponse(req, models.GlobalErrorErrorEnumTOKENEXPIRED, "token error"))
			return
		}
	}
	http.Error(w, "Down-stream API error", http.StatusBadGateway)
}

func (h *NestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req models.SmartthingsRequest
	ctxLogger := logging.Logger(r.Context())

	err := decodeJSONBody(w, r, &req)
	if err != nil {
		logging.Logger(r.Context()).WithError(err).Errorf("decoding JSON")
		http.Error(w, "unable to parse JSON", http.StatusBadRequest)
		return
	}

	if err := req.Validate(formats); err != nil {
		ctxLogger.WithError(err).Errorf("request validation failure")
		http.Error(w, "input validation failed", http.StatusBadRequest)
		return
	}

	switch req.Headers.InteractionType {
	case models.InteractionTypeDiscoveryRequest:
		h.HandleDiscoveryRequest(w, r, req)
	case models.InteractionTypeStateRefreshRequest:
		h.HandleStateRefreshRequest(w, r, req)
	case models.InteractionTypeCommandRequest:
		h.HandleCommandRequest(w, r, req)
	case models.InteractionTypeGrantCallbackAccess:
		h.HandleGrantCallbackAccess(w, r, req)
	case models.InteractionTypeIntegrationDeleted:
		ctxLogger.Warnf("unimplemented request type: %s", req.Headers.InteractionType)
		h.sendJSONResponse(w, r,
			NewGlobalErrorResponse(req, models.GlobalErrorErrorEnumINVALIDINTERACTIONTYPE, "Unimplemented Interaction Type"),
		)
	case models.InteractionTypeInteractionResult:
		h.HandleInteractionResult(w, r, req)
	default:
		ctxLogger.Errorf("unsupported request type: %s", req.Headers.InteractionType)
		h.sendJSONResponse(w, r,
			NewGlobalErrorResponse(req, models.GlobalErrorErrorEnumINVALIDINTERACTIONTYPE, "Unknown Interaction Type"),
		)
	}
}

// Interaction result type requests indicate a problem with data that we sent
// back to Smartthings from a previous request
//
func (h *NestHandler) HandleInteractionResult(w http.ResponseWriter, r *http.Request, req models.SmartthingsRequest) {
	ctxLogger := logging.Logger(r.Context())

	var msgs []string

	if req.DeviceState != nil {
		var devStrings []string
		for _, d := range req.DeviceState {
			var errStrings []string
			if d.DeviceError != nil {
				for i, e := range d.DeviceError {
					errString := fmt.Sprintf("%d: %s, Detail: %s", i, *e.ErrorEnum, e.Detail)
					errStrings = append(errStrings, errString)
				}
			}
			devString := fmt.Sprintf("{id: %s, errors: %s}", d.ExternalDeviceID, strings.Join(errStrings, ", "))
			devStrings = append(devStrings, devString)
		}

		msgs = append(msgs, "Devices: "+strings.Join(devStrings, ", "))
	}

	if req.GlobalError != nil {
		msgs = append(msgs, "Global error: "+req.GlobalError.Detail)
	}

	ctxLogger.Warnf("Received interaction result.  Originating Interaction: %s.  Details: %s",
		req.OriginatingInteractionType,
		strings.Join(msgs, ",  "),
	)
}

// The GrantCallbackAccess request provides us with the information that we need to
// request an access and refresh token from the Smartthings token service
func (h *NestHandler) HandleGrantCallbackAccess(w http.ResponseWriter, r *http.Request, req models.SmartthingsRequest) {
	ctxLogger := logging.Logger(r.Context())

	// New oauth state, will overwrite any existing state on disk
	state := stoauth.NewState().WithContext(r.Context()).WithClientSecret(h.stClientSecret)
	state.ClientID = req.CallbackAuthentication.ClientID
	state.Scope = req.CallbackAuthentication.Scope
	state.TokenURL = *req.CallbackUrls.OauthToken
	state.StateCallbackURL = *req.CallbackUrls.StateCallback

	// Make an authorization-code grant request to get an access/refresh token
	if err := state.AuthCodeFlow(*req.Headers.RequestID, req.CallbackAuthentication.Code); err != nil {
		ctxLogger.WithError(err).Error("fetching tokens from smartthings")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	ctxLogger.Infof("Smartthings oauth state: %+v", state)

	// Save state for future uses..
	if err := state.Save(h.oauthStateFile); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *NestHandler) HandleDiscoveryRequest(w http.ResponseWriter, r *http.Request, req models.SmartthingsRequest) {
	ctxLogger := logging.Logger(r.Context())

	c := h.sdmClient.WithAccessToken(*req.Authentication.Token)
	nestDevices, err := c.Devices()
	if err != nil {
		h.sendAPIErrorResponse(w, r, req, err)
		return
	}
	ctxLogger.Infof("Devices: %+v", nestDevices)

	manufacturer := "Google"
	model := "Nest Thermostat"

	var stDevices []*models.Device
	for _, nestDevice := range nestDevices {
		stDevice := models.Device{
			DeviceHandlerType: StNestThermostatDeviceProfileID,
			DeviceUniqueID:    nestDevice.ID,
			ExternalDeviceID:  nestDevice.ID,
			ManufacturerInfo: &models.Manufacturer{
				ManufacturerName: &manufacturer,
				ModelName:        &model,
			},
		}

		stDevices = append(stDevices, &stDevice)
	}

	resp := newDiscoveryResponse(req)
	resp.Devices = stDevices

	h.sendJSONResponse(w, r, resp)
}

func (h *NestHandler) HandleStateRefreshRequest(w http.ResponseWriter, r *http.Request, req models.SmartthingsRequest) {
	ctxLogger := logging.Logger(r.Context())
	c := h.sdmClient.WithAccessToken(*req.Authentication.Token)

	var states []*models.DeviceState

	for _, reqDevice := range req.Devices {
		deviceInfo := models.DeviceState{}

		// Ask Google for the Nest device info
		nestDevice, err := c.GetDevice(*reqDevice.ExternalDeviceID)
		if err != nil {
			h.sendAPIErrorResponse(w, r, req, err)
			return
		}

		nestTraits := nestDevice.Traits.TraitIDs()

		deviceInfo.ExternalDeviceID = nestDevice.ID
		deviceInfo.States = make([]*models.DeviceStateStatesItems0, 0, len(nestTraits))

		for _, nestTraitID := range nestTraits {
			nestTrait := nestDevice.Traits.Trait(nestTraitID)

			// Does the trait know how to expose itself to Smartthings?
			i, ok := nestTrait.(sdmapi.StCapability)
			if !ok {
				ctxLogger.Debugf("Ignoring Nest trait %s, no Smartthings adapter", nestTraitID.Name())
				continue
			}

			stStates := i.ToSmartthingsState(nestDevice.Traits)
			deviceInfo.States = append(deviceInfo.States, stStates...)
		}

		states = append(states, &deviceInfo)
	}

	resp := NewDeviceStateResponse(req)
	resp.DeviceState = states

	h.sendJSONResponse(w, r, resp)
}

func (h *NestHandler) HandleCommandRequest(w http.ResponseWriter, r *http.Request, req models.SmartthingsRequest) {
	ctxLogger := logging.Logger(r.Context())
	c := h.sdmClient.WithAccessToken(*req.Authentication.Token)

	var states []*models.DeviceState

	for _, device := range req.Devices {
		deviceInfo := models.DeviceState{
			ExternalDeviceID: *device.ExternalDeviceID,
		}

		for _, command := range device.Commands {
			sdmCommands, err := sdmapi.StCommandToSdmCommands(command)
			if err != nil {
				ctxLogger.WithError(err).Error("converting stcommand to sdmcommand")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			if sdmCommands == nil {
				errEnum := "CAPABILITY-NOT-SUPPORTED"
				deviceError := models.DeviceStateDeviceErrorItems0{
					Detail:    "unsupported command",
					ErrorEnum: &errEnum,
				}
				deviceInfo.DeviceError = []*models.DeviceStateDeviceErrorItems0{&deviceError}
				states = append(states, &deviceInfo)
				continue
			}

			for _, sdmCommand := range sdmCommands {
				if err := c.SendCommand(*device.ExternalDeviceID, sdmCommand); err != nil {
					if googleApiErrorIsGlobal(err, true) {
						h.sendAPIErrorResponse(w, r, req, err)
						return
					} else {
						deviceError := makeDeviceError(err)
						deviceInfo.DeviceError = []*models.DeviceStateDeviceErrorItems0{&deviceError}
					}
				}
			}
		}

		if deviceInfo.DeviceError == nil {
			// Ask Google for the Nest device info
			nestDevice, err := c.GetDevice(*device.ExternalDeviceID)
			if err != nil {
				continue
			}

			nestTraits := nestDevice.Traits.TraitIDs()

			deviceInfo.ExternalDeviceID = nestDevice.ID
			deviceInfo.States = make([]*models.DeviceStateStatesItems0, 0, len(nestTraits))

			for _, nestTraitID := range nestTraits {
				nestTrait := nestDevice.Traits.Trait(nestTraitID)

				// Does the trait know how to expose itself to Smartthings?
				i, ok := nestTrait.(sdmapi.StCapability)
				if !ok {
					ctxLogger.Debugf("Ignoring Nest trait %s, no Smartthings adapter", nestTraitID.Name())
					continue
				}

				stStates := i.ToSmartthingsState(nestDevice.Traits)
				deviceInfo.States = append(deviceInfo.States, stStates...)
			}
		}

		states = append(states, &deviceInfo)

	}

	resp := NewCommandResponse(req)
	resp.DeviceState = states

	h.sendJSONResponse(w, r, resp)

}
