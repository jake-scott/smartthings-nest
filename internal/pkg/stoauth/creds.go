package stoauth

import (
	"fmt"
	"time"
)

func (s *State) GetAccessToken() (string, error) {
	// Do we have an existing unexpired token ?
	if s.accessToken != "" && (s.accessTokenExpiry != time.Time{}) {
		if s.accessTokenExpiry.After(time.Now().Add(s.MinAccessTokenValidity)) {
			return s.accessToken, nil
		}
	}

	// No, let's refresh
	if s.refreshToken == "" {
		return "", fmt.Errorf("access token expired or missing, and no refresh token found - call AuthCodeFlow() to populate")
	}

	if err := s.refreshTokenFlow(); err != nil {
		return "", err
	}

	s.save()
	return s.accessToken, nil
}
