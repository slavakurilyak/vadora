package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/sdk"
)

// runLogin signs an account in from the terminal. It asks for the email and then for the
// emailed code, and it never takes a credential from a flag or an argument and never
// signs anything (001:74, 001:46, 001:535).
//
// Without a flag it runs the OAuth sign in with PKCE and keeps the account session that
// the store commands use. With --buyer it creates a revocable buyer credential for an
// independent buyer agent, which is the only credential that buyer API calls carry. No
// sign in creates a third credential type.
func runLogin(env *Env, args []string) error {
	set := flagSet("login", env)
	buyer := set.Bool("buyer", false, "create a revocable buyer credential for buyer API calls, instead of an account session for the store commands")
	if _, err := parse(set, args); err != nil {
		return err
	}
	email, err := prompt(env, "Email")
	if err != nil {
		return err
	}
	if len(email) > 320 {
		return errs.New(errs.InvalidRequest, "enter one valid email address")
	}
	if strings.Count(email, "@") != 1 {
		return errs.New(errs.InvalidRequest, "enter one valid email address")
	}
	api, err := newClientWith(env, "")
	if err != nil {
		return err
	}
	api.Now = env.now
	request, err := api.RequestSignInCode(callContext(), email, *buyer)
	if err != nil {
		return err
	}
	line(env, "A code was sent to %s. It works once, and only on Vadora.", email)
	code, err := prompt(env, "Code")
	if err != nil {
		return err
	}
	if *buyer {
		return completeBuyerLogin(env, api, request.RequestID, email, code)
	}
	return completeAccountLogin(env, api, request.RequestID, email, code)
}

// completeBuyerLogin trades the code for a buyer credential and stores it once.
func completeBuyerLogin(env *Env, api *client.Client, requestID, email, code string) error {
	issued, err := api.CompleteBuyerSignIn(callContext(), requestID, email, code)
	if err != nil {
		return err
	}
	env.secrets = append(env.secrets, issued.Credential)
	if err := env.credentials().Save(env.APIBase, issued.Credential); err != nil {
		return err
	}
	return render(env, "Signed in. The buyer credential is stored on this machine, and it is shown nowhere.\n",
		map[string]string{"status": "signed_in", "credential_id": issued.ID, "expires_at": issued.ExpiresAt})
}

// completeAccountLogin trades the code for an account session through the terminal OAuth
// flow and stores it. The session carries the merchant scope only: it administers the
// account's own stores, grants no spending authority, and cannot sign or submit a payment.
func completeAccountLogin(env *Env, api *client.Client, requestID, email, code string) error {
	session, err := api.TerminalSignIn(callContext(), requestID, email, code)
	if err != nil {
		return err
	}
	if err := saveSession(env, session); err != nil {
		return err
	}
	return render(env, "Signed in. The account session is stored on this machine, and it is shown nowhere. It runs the store commands and moves no money.\n",
		map[string]string{"status": "signed_in", "scope": session.Scope, "expires_at": session.ExpiresAt.UTC().Format(time.RFC3339)})
}

// runLogout forgets what this machine holds for the API origin: the account session, which
// Vadora ends on its server too, and the buyer credential, which the account page revokes.
func runLogout(env *Env, args []string) error {
	set := flagSet("logout", env)
	if _, err := parse(set, args); err != nil {
		return err
	}
	buyer, err := env.credentials().Load(env.APIBase)
	if err != nil {
		return err
	}
	session, haveSession, err := loadSession(env)
	if err != nil {
		return err
	}
	if buyer == "" {
		if !haveSession {
			return errs.New(errs.Unauthenticated, "no credential is stored for this origin")
		}
	}
	revoked := false
	if haveSession {
		revoked = revokeQuietly(env, session)
		if err := env.credentials().Delete(sessionKey(env.APIBase)); err != nil {
			return err
		}
	}
	if buyer != "" {
		if err := env.credentials().Delete(env.APIBase); err != nil {
			return err
		}
	}
	return render(env, loggedOutText(haveSession, revoked, buyer != ""),
		map[string]any{"status": "signed_out", "session_revoked": revoked, "buyer_credential_removed": buyer != ""})
}

// revokeQuietly asks Vadora to end the account session, and reports whether it did. A
// server that cannot be reached does not stop the machine from forgetting the session.
func revokeQuietly(env *Env, session client.Session) bool {
	api, err := newClientWith(env, "")
	if err != nil {
		return false
	}
	return api.RevokeSession(callContext(), session) == nil
}

// loggedOutText words what logout did.
func loggedOutText(hadSession, revoked, hadBuyer bool) string {
	var text strings.Builder
	text.WriteString("Signed out.")
	if hadSession {
		if revoked {
			text.WriteString(" The account session ended on Vadora too.")
		} else {
			text.WriteString(" Vadora could not be reached to end the account session, so end it on your Vadora account page.")
		}
	}
	if hadBuyer {
		text.WriteString(" Revoke the buyer credential in your Vadora account to end it on the server too.")
	}
	return fmt.Sprintln(text.String())
}
