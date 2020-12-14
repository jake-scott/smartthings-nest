package stoauth

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jake-scott/smartthings-nest/internal/pkg/logging"
	"github.com/pkg/errors"
)

const defaultMinAccessTokenValidity = time.Second * 60

type State struct {
	ClientID               string
	Scope                  string
	TokenURL               string
	StateCallbackURL       string
	MinAccessTokenValidity time.Duration

	// non-exported
	clientSecret      string
	accessToken       string
	accessTokenExpiry time.Time
	refreshToken      string
	ctx               context.Context
	fileName          string
}

// Version of state that we marshal/unmarshal
type stateMarshal struct {
	ClientID          string    `json:"client-id"`
	Scope             string    `json:"scope"`
	TokenURL          string    `json:"token-url"`
	StateCallbackURL  string    `json:"state-callback-url"`
	AccessToken       string    `json:"access-token"`
	AccessTokenExpiry time.Time `json:"access-token-expiry"`
	RefreshToken      string    `json:"refresh-token"`
}

func hashOf(s string) string {
	sum := sha1.Sum([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// obfuscate tokens/secrets when stringified
//
func (s State) String() string {
	return fmt.Sprintf("ClientID [%s], clientSecret [%s], Scope [%s] TokenURL [%s]  StateCallbackURL [%s]  accessToken: [%s]  accessTokenExpiry [%s]  refreshToken [%s]",
		s.ClientID, hashOf(s.clientSecret), s.Scope, s.TokenURL, s.StateCallbackURL,
		hashOf(s.accessToken), s.accessTokenExpiry, hashOf(s.refreshToken))
}

func NewState() State {
	return State{
		ctx:                    context.Background(),
		MinAccessTokenValidity: defaultMinAccessTokenValidity,
	}
}

func (s State) WithContext(ctx context.Context) State {
	s.ctx = ctx
	return s
}
func (s State) WithClientSecret(secret string) State {
	s.clientSecret = secret
	return s
}

func (s *State) Save(fileName string) error {
	sm := stateMarshal{
		ClientID:          s.ClientID,
		Scope:             s.Scope,
		TokenURL:          s.TokenURL,
		StateCallbackURL:  s.StateCallbackURL,
		AccessToken:       s.accessToken,
		AccessTokenExpiry: s.accessTokenExpiry,
		RefreshToken:      s.refreshToken,
	}

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0750)
	if err != nil {
		return errors.Wrapf(err, "opening smartthings oauth state %s for write", fileName)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sm); err != nil {
		return errors.Wrapf(err, "saving smartthings oauth state to %s", fileName)
	}

	// Store for later use
	s.fileName = fileName
	return nil
}

func (s *State) save() error {
	if s.fileName != "" {
		return s.Save(s.fileName)
	}

	logging.Logger(s.ctx).Warn("cannot save oauth state, no file name available")
	return nil
}

func (s *State) Load(fileName string) error {
	sm := stateMarshal{}

	file, err := os.OpenFile(fileName, os.O_RDONLY, 0750)
	if err != nil {
		return errors.Wrapf(err, "opening smartthings oauth state %s for read", fileName)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&sm); err != nil {
		return errors.Wrapf(err, "loading smartthings oauth state to %s", fileName)
	}

	s.ClientID = sm.ClientID
	s.Scope = sm.Scope
	s.TokenURL = sm.TokenURL
	s.StateCallbackURL = sm.StateCallbackURL
	s.accessToken = sm.AccessToken
	s.accessTokenExpiry = sm.AccessTokenExpiry
	s.refreshToken = sm.RefreshToken

	// Store for later use
	s.fileName = fileName

	return nil
}
