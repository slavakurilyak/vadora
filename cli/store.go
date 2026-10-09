package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
)

// CredentialStore keeps the buyer credential that this machine holds, one per API
// origin. The credential never comes from a flag, an argument, or an environment
// variable, so it never appears in a shell history or a process list (shared:120).
type CredentialStore interface {
	// Load returns the stored credential, or an empty string when none is stored.
	Load(apiBase string) (string, error)
	// Save stores the credential.
	Save(apiBase, credential string) error
	// Delete removes the stored credential.
	Delete(apiBase string) error
}

// credentialFile names the file of the default store.
const credentialFile = "credentials.json"

// fileStore keeps credentials in one file that only the owner can read. It is the
// store of every platform; a keychain integration can replace it per platform
// through Env.Store.
type fileStore struct{ env *Env }

// credentials returns the credential store of this run.
func (e *Env) credentials() CredentialStore {
	if e.Store != nil {
		return e.Store
	}
	return fileStore{env: e}
}

func (s fileStore) path() (string, error) {
	dir, err := configDir(s.env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credentialFile), nil
}

// read loads the file. A file that another user can read is refused, because a
// credential in it would no longer be private.
func (s fileStore) read() (map[string]string, string, error) {
	path, err := s.path()
	if err != nil {
		return nil, "", err
	}
	stored := map[string]string{}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stored, path, nil
	}
	if err != nil {
		return nil, path, errs.New(errs.Unauthenticated, "the credential file could not be read")
	}
	if err := requireOwnerOnly(path); err != nil {
		return nil, path, errs.New(errs.Unauthenticated, err.Error())
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, path, errs.New(errs.Unauthenticated, "the credential file is damaged: sign in again")
	}
	return stored, path, nil
}

func (s fileStore) Load(apiBase string) (string, error) {
	stored, _, err := s.read()
	if err != nil {
		return "", err
	}
	return stored[strings.TrimRight(apiBase, "/")], nil
}

func (s fileStore) Save(apiBase, credential string) error {
	if len(credential) < 16 {
		return errs.New(errs.Unauthenticated, "the credential is malformed")
	}
	if len(credential) > 4096 {
		return errs.New(errs.Unauthenticated, "the credential is malformed")
	}
	if strings.ContainsAny(credential, " \t\r\n") {
		return errs.New(errs.Unauthenticated, "the credential is malformed")
	}
	stored, path, err := s.read()
	if err != nil {
		return err
	}
	stored[strings.TrimRight(apiBase, "/")] = credential
	return s.write(path, stored)
}

func (s fileStore) Delete(apiBase string) error {
	stored, path, err := s.read()
	if err != nil {
		return err
	}
	origin := strings.TrimRight(apiBase, "/")
	if _, present := stored[origin]; !present {
		return errs.New(errs.Unauthenticated, "no credential is stored for this origin")
	}
	delete(stored, origin)
	return s.write(path, stored)
}

func (s fileStore) write(path string, stored map[string]string) error {
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if err := writePrivate(path, raw); err != nil {
		return errs.New(errs.Unauthenticated, "the credential could not be stored on this machine")
	}
	return nil
}
