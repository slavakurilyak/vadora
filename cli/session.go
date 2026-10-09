package cli

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/sdk"
)

// The account session of vadora login is the OAuth access token and refresh token that
// the merchant commands send. It lives beside the buyer credential in the private
// credential file, under its own key, so the two never replace each other. The tokens
// come from no flag, no argument, and no environment variable, and the CLI prints none
// of them (shared:120).

// sessionKey names the stored account session of one API origin.
func sessionKey(apiBase string) string {
	return strings.TrimRight(apiBase, "/") + "#account-session"
}

// loadSession reads the stored account session. The bool reports whether one is stored.
func loadSession(env *Env) (client.Session, bool, error) {
	stored, err := env.credentials().Load(sessionKey(env.APIBase))
	if err != nil {
		return client.Session{}, false, err
	}
	if stored == "" {
		return client.Session{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(stored)
	if err != nil {
		return client.Session{}, false, errs.New(errs.Unauthenticated, "the stored account session is damaged: run vadora login")
	}
	var session client.Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return client.Session{}, false, errs.New(errs.Unauthenticated, "the stored account session is damaged: run vadora login")
	}
	if session.AccessToken == "" {
		return client.Session{}, false, errs.New(errs.Unauthenticated, "the stored account session is damaged: run vadora login")
	}
	return session, true, nil
}

// saveSession stores the account session, and registers its tokens as secrets that no
// output may carry.
func saveSession(env *Env, session client.Session) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return errs.New(errs.Unauthenticated, "the account session could not be stored")
	}
	env.secrets = append(env.secrets, session.AccessToken, session.RefreshToken)
	return env.credentials().Save(sessionKey(env.APIBase), base64.RawURLEncoding.EncodeToString(raw))
}

// newMerchantClient builds the SDK client of a merchant command: the account session of
// this machine, renewed first when its access token has ended. A refresh token that the
// server refuses ends the session, and the person signs in again.
func newMerchantClient(env *Env) (*client.Client, error) {
	session, found, err := loadSession(env)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errs.New(errs.Unauthenticated, "run vadora login to sign in to your account for the store commands")
	}
	env.secrets = append(env.secrets, session.AccessToken, session.RefreshToken)
	if session.Expired(env.now()) {
		session, err = renewSession(env, session)
		if err != nil {
			return nil, err
		}
	}
	return newClientWith(env, session.AccessToken)
}

// renewSession trades the refresh token for a new session and stores it. The server ends
// the old refresh token as it issues the new one, so the new session is stored before
// anything else happens with it.
func renewSession(env *Env, session client.Session) (client.Session, error) {
	anonymous, err := newClientWith(env, "")
	if err != nil {
		return client.Session{}, err
	}
	anonymous.Now = env.now
	renewed, err := anonymous.RefreshSession(callContext(), session)
	if err != nil {
		failure, known := errs.As(err)
		if known {
			if failure.Code == errs.Unauthenticated {
				_ = env.credentials().Delete(sessionKey(env.APIBase))
				return client.Session{}, errs.New(errs.Unauthenticated, "the account session has ended: run vadora login")
			}
		}
		return client.Session{}, err
	}
	if err := saveSession(env, renewed); err != nil {
		return client.Session{}, err
	}
	return renewed, nil
}
