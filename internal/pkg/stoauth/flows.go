package stoauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jake-scott/smartthings-nest/generated/models"
	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/pkg/errors"
)

func newAccessTokenRequest(requestID string) models.AccessTokenRequest {
	stSchema := "st-schema"
	stVersion := "1.0"

	return models.AccessTokenRequest{
		Headers: &models.Headers{
			Schema:          &stSchema,
			Version:         &stVersion,
			RequestID:       &requestID,
			InteractionType: models.InteractionTypeAccessTokenRequest,
		},
		CallbackAuthentication: &models.AccessTokenRequestCallbackAuthentication{},
	}
}

func newRefreshTokenRequest(requestID string) models.RefreshAccessTokenRequest {
	stSchema := "st-schema"
	stVersion := "1.0"

	return models.RefreshAccessTokenRequest{
		Headers: &models.Headers{
			Schema:          &stSchema,
			Version:         &stVersion,
			RequestID:       &requestID,
			InteractionType: models.InteractionTypeRefreshAccessTokens,
		},
		CallbackAuthentication: &models.RefreshAccessTokenRequestCallbackAuthentication{},
	}
}

func (s *State) AuthCodeFlow(requestID string, code string) error {
	ctxLogger := logging.Logger(s.ctx)

	if s.clientSecret == "" {
		return errors.New("no clientSecret, cannot execute authorization code grant")
	}

	stGrantType := "authorization_code"

	// Make a request to the token URL for an access/refresh token
	tokenReq := newAccessTokenRequest(requestID)
	tokenReq.CallbackAuthentication.ClientID = &s.ClientID
	tokenReq.CallbackAuthentication.ClientSecret = &s.clientSecret
	tokenReq.CallbackAuthentication.Code = &code
	tokenReq.CallbackAuthentication.GrantType = &stGrantType

	reqBody, err := json.Marshal(tokenReq)
	if err != nil {
		return errors.Wrap(err, "encoding smartthing auth code token request")
	}

	ctxLogger.Debugf("Sending access token request to Smartthings URL [%s]: %s", s.TokenURL, reqBody)

	// Send request
	resp, err := http.Post(s.TokenURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return errors.Wrap(err, "executing authorization code grant")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "reading response body")
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("non-200 code from Smartthings token URL: %d (%s): %s", resp.StatusCode, resp.Status, bodyBytes)
	}

	ctxLogger.Debugf("Access Token response: %s", bodyBytes)

	tokenResp := models.AccessTokenResponse{}
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return errors.Wrap(err, "decoding smartthing token response")
	}

	if tokenResp.Headers.InteractionType != models.InteractionTypeAccessTokenResponse {
		return fmt.Errorf("got interaction type %s, expected accessTokenResponse", tokenResp.Headers.InteractionType)
	}

	s.accessToken = *tokenResp.CallbackAuthentication.AccessToken
	s.refreshToken = *tokenResp.CallbackAuthentication.RefreshToken
	s.accessTokenExpiry = time.Now().Add(time.Second * time.Duration(*tokenResp.CallbackAuthentication.ExpiresIn))

	return nil
}

func (s *State) refreshTokenFlow() error {
	ctxLogger := logging.Logger(s.ctx)

	if s.clientSecret == "" {
		return errors.New("no clientSecret, cannot execute refresh token flow")
	}

	stGrantType := "refresh_token"
	requestID := uuid.New().String()

	// Make a request to the token URL for an access/refresh token
	tokenReq := newRefreshTokenRequest(requestID)
	tokenReq.CallbackAuthentication.ClientID = &s.ClientID
	tokenReq.CallbackAuthentication.ClientSecret = &s.clientSecret
	tokenReq.CallbackAuthentication.GrantType = &stGrantType
	tokenReq.CallbackAuthentication.RefreshToken = &s.refreshToken

	reqBody, err := json.Marshal(tokenReq)
	if err != nil {
		return errors.Wrap(err, "encoding smartthing refresh token request")
	}

	ctxLogger.Debugf("Sending refresh token request to Smartthings URL [%s]: %s", s.TokenURL, reqBody)

	// Send request
	resp, err := http.Post(s.TokenURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return errors.Wrap(err, "executing refresh token grant")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "reading response body")
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("non-200 code from Smartthings token URL: %d (%s): %s", resp.StatusCode, resp.Status, bodyBytes)
	}

	ctxLogger.Debugf("Refresh Token response: %s", bodyBytes)

	tokenResp := models.AccessTokenResponse{}
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return errors.Wrap(err, "decoding smartthing token response")
	}

	if tokenResp.Headers.InteractionType != models.InteractionTypeAccessTokenResponse {
		return fmt.Errorf("got interaction type %s, expected accessTokenResponse", tokenResp.Headers.InteractionType)
	}

	s.accessToken = *tokenResp.CallbackAuthentication.AccessToken
	s.refreshToken = *tokenResp.CallbackAuthentication.RefreshToken
	s.accessTokenExpiry = time.Now().Add(time.Second * time.Duration(*tokenResp.CallbackAuthentication.ExpiresIn))

	return nil

}
