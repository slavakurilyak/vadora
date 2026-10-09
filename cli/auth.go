package cli

import (
	"fmt"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
)

// runLogin creates a buyer credential for an independent buyer agent. It asks for
// the email and then for the emailed code in the terminal, and it never takes a
// credential from a flag or an argument and never signs anything. The credential
// is stored once on this machine and printed nowhere (001:74, 001:531).
func runLogin(env *Env, args []string) error {
	set := flagSet("login", env)
	buyer := set.Bool("buyer", false, "create a revocable buyer credential for buyer API calls")
	if _, err := parse(set, args); err != nil {
		return err
	}
	if !*buyer {
		return errs.New(errs.InvalidRequest, "the account sign in for the merchant commands is not part of this build yet: run vadora login --buyer to create a buyer credential")
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
	request, err := api.RequestSignInCode(callContext(), email, true)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "A code was sent to %s. It works once, and only on Vadora.\n", email)
	code, err := prompt(env, "Code")
	if err != nil {
		return err
	}
	issued, err := api.CompleteBuyerSignIn(callContext(), request.RequestID, email, code)
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

// runLogout forgets the credential of this machine. The account page revokes it
// on the server.
func runLogout(env *Env, args []string) error {
	set := flagSet("logout", env)
	if _, err := parse(set, args); err != nil {
		return err
	}
	if err := env.credentials().Delete(env.APIBase); err != nil {
		return err
	}
	return render(env, "Signed out. Revoke the credential in your Vadora account to end it on the server too.\n",
		map[string]string{"status": "signed_out"})
}
