package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-openapi/runtime/middleware/header"
	"github.com/go-openapi/strfmt"
	"github.com/jake-scott/smartthings-nest/generated/models"
)

// For generated request validation routines
var formats strfmt.Registry

func init() {
	// Default validators
	formats = strfmt.NewFormats()
}

func newDiscoveryResponse(req models.SmartthingsRequest) models.DiscoveryResponse {
	var h models.Headers = *req.Headers
	h.InteractionType = models.InteractionTypeDiscoveryResponse

	return models.DiscoveryResponse{
		Headers: &h,
	}
}

func NewDeviceStateResponse(req models.SmartthingsRequest) models.DeviceStateResponse {
	var h models.Headers = *req.Headers
	h.InteractionType = models.InteractionTypeStateRefreshResponse

	return models.DeviceStateResponse{
		Headers: &h,
	}
}

func NewCommandResponse(req models.SmartthingsRequest) models.CommandResponse {
	var h models.Headers = *req.Headers
	h.InteractionType = models.InteractionTypeCommandResponse

	return models.CommandResponse{
		Headers: &h,
	}
}

func NewGlobalErrorResponse(req models.SmartthingsRequest, errEnum string, detail string) models.InteractionResult {
	var h models.Headers = *req.Headers
	h.InteractionType = responseTypeFromRequestType(h.InteractionType)

	return models.InteractionResult{
		Headers: &h,
		GlobalError: &models.GlobalError{
			ErrorEnum: &errEnum,
			Detail:    detail,
		},
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	if r.Header.Get("Content-Type") != "" {
		value, _ := header.ParseValueAndParams(r.Header, "Content-Type")
		if value != "application/json" {
			return fmt.Errorf("expected JSON request, got %s", value)
		}
	}

	// 100kb max body
	reader := http.MaxBytesReader(w, r.Body, 100*1024)
	dec := json.NewDecoder(reader)

	if err := dec.Decode(&dst); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must only contain a single JSON object")
	}

	return nil
}

func responseTypeFromRequestType(in models.InteractionType) models.InteractionType {
	switch in {
	case models.InteractionTypeDiscoveryRequest:
		return models.InteractionTypeDiscoveryResponse
	case models.InteractionTypeStateRefreshRequest:
		return models.InteractionTypeStateRefreshResponse
	case models.InteractionTypeCommandRequest:
		return models.InteractionTypeCommandResponse
	}

	return models.InteractionTypeInteractionResult
}
