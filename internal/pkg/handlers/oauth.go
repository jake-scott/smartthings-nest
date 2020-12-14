package handlers

import (
	"net/http"
)

/*
 * OauthHandler redirects the caller to the Nest Services authorization endpoint
 * for the Smart Devices Management project, adding the access_type and
 * prompt query string values that are required by SDM but not included by
 * the Smartthings client
 */

type oauthHandler struct {
	sdmProjectID string
}

func NewOauthHandler(sdmProjectID string) oauthHandler {
	return oauthHandler{
		sdmProjectID: sdmProjectID,
	}
}

func (h *oauthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Copy of the request URL (so we don't modify the original)
	u := *r.URL

	// Set required query parameters
	queryValues := u.Query()
	queryValues.Set("access_type", "offline")
	queryValues.Set("prompt", "consent")
	u.RawQuery = queryValues.Encode()

	// Set the URI path
	u.Scheme = "https"
	u.Host = "nestservices.google.com"
	u.Path = "/partnerconnections/" + h.sdmProjectID + "/auth"

	http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
}
