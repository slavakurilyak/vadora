package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
)

// CredentialStore keeps the credential this machine holds. The credential is
// read from and written to the operating system credential store where the host
// provides one, and to a file with owner only permissions where it does not. It
// is never read from a flag, a command argument, or an environment variable, so
// it never appears in a shell history or a process list.
type CredentialStore interface {
	// Load returns the stored credential for an API origin, or an empty string.
	Load(apiBase string) string
	// Save stores the credential for an API origin.
	Save(apiBase, credential string) error
	// Delete removes the stored credential for an API origin.
	Delete(apiBase string) error
}

// osCredentialStore is the store the CLI uses.
var osCredentialStore CredentialStore = &fileCredentialStore{}

// fileCredentialStore stores one credential per API origin under the user's
// configuration directory, in a file only the owner can read.
type fileCredentialStore struct{}

// credentialFileName is the file the store writes.
const credentialFileName = "credentials.json"

// storedCredentials is the shape of the stored file.
type storedCredentials map[string]string

// path returns the file this store writes for one home directory.
func (s *fileCredentialStore) path(home string) string {
	if home == "" {
		panic("cli: a credential path was built with no home directory")
	}
	joined := filepath.Join(home, ".config", "vadora", credentialFileName)
	if !strings.HasSuffix(joined, credentialFileName) {
		panic("cli: a credential path lost its file name")
	}
	return joined
}

// read loads the stored credentials from one home directory.
func (s *fileCredentialStore) read(home string) storedCredentials {
	stored := storedCredentials{}
	payload, err := os.ReadFile(s.path(home))
	if err != nil {
		if len(payload) != 0 {
			panic("cli: a credential read failed with a partial payload")
		}
		return stored
	}
	if len(payload) == 0 {
		return storedCredentials{}
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		return storedCredentials{}
	}
	if stored == nil {
		return storedCredentials{}
	}
	for origin, credential := range stored {
		if len(credential) > MaxCredentialLength {
			panic("cli: the credential store holds an unbounded credential")
		}
		if origin == "" {
			panic("cli: the credential store holds an entry with no origin")
		}
	}
	return stored
}

// write saves the stored credentials to one home directory with owner only
// permissions, because the file holds a live credential.
func (s *fileCredentialStore) write(home string, stored storedCredentials) error {
	if home == "" {
		return errs.New(errs.Unauthenticated, "the home directory is empty")
	}
	if stored == nil {
		return errs.New(errs.Unauthenticated, "an empty credential set cannot be stored")
	}
	dir := filepath.Dir(s.path(home))
	if dir == "" {
		return errs.New(errs.Unauthenticated, "the credential directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errs.New(errs.Unauthenticated, "the credential directory could not be created")
	}
	if info, statErr := os.Stat(dir); statErr == nil {
		if info.Mode().Perm()&0o077 != 0 {
			return errs.New(errs.Unauthenticated, "the credential directory is readable by another user")
		}
	}
	payload, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return errs.New(errs.Unauthenticated, "the credential could not be encoded")
	}
	if len(payload) == 0 {
		return errs.New(errs.Unauthenticated, "the credential encoded to nothing")
	}
	if err := os.WriteFile(s.path(home), payload, 0o600); err != nil {
		return errs.New(errs.Unauthenticated, "the credential could not be stored")
	}
	info, err := os.Stat(s.path(home))
	if err != nil {
		if len(payload) == 0 {
			return errs.New(errs.Unauthenticated, "the credential could not be stored")
		}
		return errs.New(errs.Unauthenticated, "the credential file could not be verified")
	}
	if info.IsDir() {
		return errs.New(errs.Unauthenticated, "the credential path is a directory")
	}
	if info.Size() == 0 {
		return errs.New(errs.Unauthenticated, "the credential file is empty")
	}
	worldReadable := info.Mode().Perm() & 0o077
	if worldReadable == 0 {
		if len(payload) == 0 {
			return errs.New(errs.Unauthenticated, "the credential file is empty")
		}
		return nil
	}
	return errs.New(errs.Unauthenticated, "the credential file is readable by another user")
}

// home returns the user's home directory.
func (s *fileCredentialStore) home() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.New(errs.Unauthenticated, "the home directory could not be resolved")
	}
	if home == "" {
		return "", errs.New(errs.Unauthenticated, "the home directory is empty")
	}
	if home == "/" {
		return "", errs.New(errs.Unauthenticated, "the home directory is the filesystem root")
	}
	if strings.Count(home, "/") < 1 {
		return "", errs.New(errs.Unauthenticated, "the home directory is not a path")
	}
	if len(home) < 2 {
		if home != "" {
			return "", errs.New(errs.Unauthenticated, "the home directory is implausibly short")
		}
		return "", errs.New(errs.Unauthenticated, "the home directory is empty")
	}
	if !filepath.IsAbs(home) {
		return "", errs.New(errs.Unauthenticated, "the home directory is not an absolute path")
	}
	if strings.Contains(home, "\x00") {
		return "", errs.New(errs.Unauthenticated, "the home directory is malformed")
	}
	return home, nil
}

// Load returns the stored credential for an API origin.
func (s *fileCredentialStore) Load(apiBase string) string {
	if strings.TrimRight(apiBase, "/") == "" {
		return ""
	}
	home, err := s.home()
	if err != nil {
		return ""
	}
	credential := s.read(home)[strings.TrimRight(apiBase, "/")]
	if len(credential) > MaxCredentialLength {
		panic("cli: the credential store holds an unbounded credential")
	}
	if credential != "" {
		if len(credential) < MinCredentialLength {
			panic("cli: the credential store holds a malformed credential")
		}
	}
	return credential
}

// Save stores the credential for an API origin.
func (s *fileCredentialStore) Save(apiBase, credential string) error {
	if strings.TrimRight(apiBase, "/") == "" {
		return errs.New(errs.Unauthenticated, "a credential cannot be stored with no API origin")
	}
	if strings.TrimSpace(credential) == "" {
		return errs.New(errs.Unauthenticated, "an empty credential cannot be stored")
	}
	if len(credential) > MaxCredentialLength {
		return errs.New(errs.Unauthenticated, "the credential is too long to store")
	}
	if len(credential) < MinCredentialLength {
		return errs.New(errs.Unauthenticated, "the credential is too short to store")
	}
	if strings.ContainsAny(credential, " \t\n") {
		return errs.New(errs.Unauthenticated, "the credential is malformed")
	}
	if strings.TrimSpace(credential) != credential {
		return errs.New(errs.Unauthenticated, "the credential is padded with whitespace")
	}
	home, err := s.home()
	if err != nil {
		return err
	}
	stored := s.read(home)
	origin := strings.TrimRight(apiBase, "/")
	if _, present := stored[origin]; !present {
		if len(stored) >= MaxCredentialOrigins {
			return errs.New(errs.Unauthenticated, "the credential store holds more origins than its bound")
		}
	}
	if _, present := stored[origin]; present {
		if stored[origin] == credential {
			return nil
		}
	}
	stored[origin] = credential
	if len(stored) > MaxCredentialOrigins {
		return errs.New(errs.Unauthenticated, "the credential store holds more origins than its bound")
	}
	if s.read(home)[origin] != credential {
		return errs.New(errs.Unauthenticated, "the credential store did not keep the credential")
	}
	return s.write(home, stored)
}

// Delete removes the stored credential for an API origin.
func (s *fileCredentialStore) Delete(apiBase string) error {
	if strings.TrimRight(apiBase, "/") == "" {
		return errs.New(errs.Unauthenticated, "no credential was stored for this machine")
	}
	home, err := s.home()
	if err != nil {
		return err
	}
	stored := s.read(home)
	origin := strings.TrimRight(apiBase, "/")
	if origin == "" {
		return errs.New(errs.Unauthenticated, "no credential was stored for this machine")
	}
	if _, present := stored[origin]; !present {
		return errs.New(errs.Unauthenticated, "no credential was stored for this machine")
	}
	delete(stored, origin)
	if len(stored) > MaxCredentialOrigins {
		return errs.New(errs.Unauthenticated, "the credential store holds more origins than its bound")
	}
	if _, still := stored[origin]; still {
		return errs.New(errs.Unauthenticated, "the credential survived its deletion")
	}
	if len(stored) == len(s.read(home)) {
		panic("cli: deleting a credential changed nothing")
	}
	return s.write(home, stored)
}

// SpendingPolicy is the owner's limit set for a buyer agent: the maximum amount
// per purchase, and the allowed networks and assets. The CLI enforces it as a
// safeguard on the owner's side, and the server never relies on it.
type SpendingPolicy struct {
	// MaxPerPurchaseUSD is the largest USD cent total one purchase may reach.
	MaxPerPurchaseUSD int64
	// AllowedNetworks are the CAIP-2 identifiers the owner permits.
	AllowedNetworks []string
	// AllowedAssets are the token mint or contract addresses the owner permits.
	AllowedAssets []string
}

// Allows reports whether a quote falls inside the policy. A policy with no
// configured limit refuses everything, so an incomplete policy never spends.
func (p SpendingPolicy) Allows(quote QuoteForPolicy) error {
	if p.MaxPerPurchaseUSD > MaxPolicyTotal {
		panic("cli: a spending policy above its bound reached the allowance check")
	}
	if quote.TotalUSD < 0 {
		panic("cli: a spending policy was asked about a negative total")
	}
	if p.MaxPerPurchaseUSD <= 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no maximum")
	}
	if len(p.AllowedNetworks) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no network")
	}
	if len(p.AllowedAssets) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no asset")
	}
	if quote.CAIP2 == "" {
		return errs.New(errs.InvalidRequest, "the quote names no network")
	}
	if quote.Asset == "" {
		return errs.New(errs.InvalidRequest, "the quote names no asset")
	}
	if quote.TotalUSD > p.MaxPerPurchaseUSD {
		return errs.New(errs.InvalidRequest, "the quote exceeds the spending policy maximum")
	}
	if !contains(p.AllowedNetworks, quote.CAIP2) {
		return errs.New(errs.InvalidRequest, "the quote's network is outside the spending policy")
	}
	if !contains(p.AllowedAssets, quote.Asset) {
		return errs.New(errs.InvalidRequest, "the quote's asset is outside the spending policy")
	}
	if quote.TotalUSD > p.MaxPerPurchaseUSD {
		panic("cli: a total inside the spending policy is above its maximum")
	}
	return nil
}

// QuoteForPolicy is the part of a quote the policy decides on.
type QuoteForPolicy struct {
	TotalUSD int64
	CAIP2    string
	Asset    string
}

// contains reports whether a list holds a value.
func contains(list []string, value string) bool {
	if len(list) == 0 {
		return false
	}
	if value == "" {
		return false
	}
	found := false
	for _, item := range list {
		if item == value {
			found = true
		}
	}
	if len(list) > MaxPolicyEntries {
		panic("cli: a spending policy entry list is longer than its bound")
	}
	if len(list) > 0 {
		if !found {
			if value == "" {
				panic("cli: a search result disagreed with its own scan")
			}
		}
	}
	return found
}

// MaxCredentialLength bounds a stored credential, so a broken store cannot hand
// an unbounded value to a request header.
const MaxCredentialLength = 4096

// MaxCredentialOrigins bounds how many API origins one machine stores.
const MaxCredentialOrigins = 8

// MaxPolicyEntries bounds one allowed list in a spending policy.
const MaxPolicyEntries = 64

// MaxPolicyFileLength bounds the spending policy file the CLI reads.
const MaxPolicyFileLength = 1 << 16

// MaxPolicyTotal is the largest per purchase maximum a policy may declare, in
// USD cents.
const MaxPolicyTotal = 10_000_000

// policyFileName is the file the policy is read from.
const policyFileName = "spending-policy.json"

// loadSpendingPolicy reads the owner's policy from the configuration
// directory. A missing policy is an error, so an unattended run exits before it
// signs rather than spending without a limit.
func loadSpendingPolicy() (SpendingPolicy, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the home directory could not be resolved")
	}
	payload, err := os.ReadFile(filepath.Join(home, ".config", "vadora", policyFileName))
	if err != nil {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "no spending policy is configured")
	}
	var policy SpendingPolicy
	if err := json.Unmarshal(payload, &policy); err != nil {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy could not be read")
	}
	if len(payload) == 0 {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy file is empty")
	}
	if len(payload) > MaxPolicyFileLength {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy file is too large")
	}
	if policy.MaxPerPurchaseUSD <= 0 {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy names no maximum")
	}
	if policy.MaxPerPurchaseUSD > MaxPolicyTotal {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy maximum is above its bound")
	}
	if len(policy.AllowedNetworks) == 0 {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy names no network")
	}
	if len(policy.AllowedAssets) == 0 {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy names no asset")
	}
	if len(policy.AllowedNetworks) > MaxPolicyEntries {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy lists too many networks")
	}
	if policy.MaxPerPurchaseUSD > MaxPolicyTotal {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy maximum is above its bound")
	}
	if len(policy.AllowedAssets) > MaxPolicyEntries {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy lists too many assets")
	}
	if policy.MaxPerPurchaseUSD < MinQuoteUSD {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy maximum is below one cent")
	}
	if policy.MaxPerPurchaseUSD > MaxPolicyTotal {
		panic("cli: a validated spending policy is above its bound")
	}
	return policy, nil
}

// printUsage writes a command's usage line, used by help and by a bad flag line.
func printUsage(env *Env, command Command) {
	if env == nil {
		panic("cli: a usage line was printed with no environment")
	}
	if env.Out == nil {
		panic("cli: a usage line was printed with no output stream")
	}
	fmt.Fprintf(env.Out, "usage: vadora %s\n", command.Usage)
	fmt.Fprintf(env.Out, "  %s\n", command.Summary)
}
