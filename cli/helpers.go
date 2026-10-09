package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vadora/vadora/api/types/domain"
	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/sdk"
)

// maxPromptLength bounds one interactive value read from the terminal.
const maxPromptLength = 4096

// flagSet builds a command's own flag set. A flag the command does not know is
// reported as INVALID_REQUEST, like every other malformed input.
func flagSet(name string, env *Env) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

// parse runs a command's flags. A flag may follow the positional arguments, so a
// person can type the listing first.
func parse(set *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for len(rest) > 0 {
		if err := set.Parse(rest); err != nil {
			return nil, errs.New(errs.InvalidRequest, "the command arguments could not be parsed: "+firstLine(err.Error()))
		}
		rest = set.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	return positional, nil
}

// firstLine returns the first line of a message.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// oneArgument returns the single positional argument that a command takes.
func oneArgument(positional []string, name string) (string, error) {
	if len(positional) != 1 {
		return "", errs.New(errs.InvalidRequest, "the command takes exactly one "+name)
	}
	value := strings.TrimSpace(positional[0])
	if value == "" {
		return "", errs.New(errs.InvalidRequest, "the "+name+" is malformed")
	}
	if len(value) > 512 {
		return "", errs.New(errs.InvalidRequest, "the "+name+" is malformed")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", errs.New(errs.InvalidRequest, "the "+name+" is malformed")
	}
	return value, nil
}

// callContext is the context of one command.
func callContext() context.Context { return context.Background() }

// newClient builds the SDK client of a command: the stored credential, the API
// origin, and the transport that the host chose.
func newClient(env *Env) (*client.Client, error) {
	credential, err := env.credentials().Load(env.APIBase)
	if err != nil {
		return nil, err
	}
	return newClientWith(env, credential)
}

// newClientWith builds an SDK client with a given credential.
func newClientWith(env *Env, credential string) (*client.Client, error) {
	built, err := client.New(env.APIBase, credential)
	if err != nil {
		return nil, errs.New(errs.InvalidRequest, err.Error())
	}
	if env.HTTP != nil {
		built.HTTP = env.HTTP
	}
	if credential != "" {
		env.secrets = append(env.secrets, credential)
	}
	return built, nil
}

// requireCredential refuses a buyer command that has no stored credential, with
// the one action that fixes it.
func requireCredential(env *Env) error {
	credential, err := env.credentials().Load(env.APIBase)
	if err != nil {
		return err
	}
	if credential == "" {
		return errs.New(errs.Unauthenticated, "run vadora login --buyer to create a buyer credential on this machine")
	}
	return nil
}

// render prints a result as human readable text or as JSON. It refuses to print a
// secret that the command loaded, such as the buyer credential or a signed
// payment, because a terminal log is shared output (shared:120).
func render(env *Env, human string, value any) error {
	text := human
	if env.JSON {
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return errs.New(errs.InvalidRequest, "the result could not be encoded")
		}
		text = string(encoded) + "\n"
	}
	for _, secret := range env.secrets {
		if secret != "" {
			if strings.Contains(text, secret) {
				return errs.New(errs.InvalidRequest, "the command output would carry a secret")
			}
		}
	}
	_, err := io.WriteString(env.Out, text)
	return err
}

// line prints one human progress line unless the caller asked for JSON.
func line(env *Env, format string, args ...any) {
	if !env.JSON {
		fmt.Fprintf(env.Out, format+"\n", args...)
	}
}

// prompt reads one line from the interactive input.
func prompt(env *Env, label string) (string, error) {
	fmt.Fprintf(env.Out, "%s: ", label)
	if env.reader == nil {
		env.reader = bufio.NewReader(env.In)
	}
	text, readErr := env.reader.ReadString('\n')
	value := strings.TrimSpace(text)
	if value == "" {
		return "", errs.New(errs.InvalidRequest, "no value was entered for "+strings.ToLower(label))
	}
	if readErr != nil {
		if !errors.Is(readErr, io.EOF) {
			return "", errs.New(errs.InvalidRequest, "the input could not be read for "+strings.ToLower(label))
		}
	}
	if len(value) > maxPromptLength {
		return "", errs.New(errs.InvalidRequest, "the value entered for "+strings.ToLower(label)+" is too long")
	}
	return value, nil
}

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

// dollarsToCents reads a decimal dollar amount such as 12.50 into whole cents,
// refusing anything that is not an exact amount.
func dollarsToCents(text string) (int64, error) {
	whole, fraction, hasFraction := strings.Cut(strings.TrimSpace(text), ".")
	if whole == "" {
		return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
	}
	if len(fraction) > 2 {
		return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
	}
	if hasFraction {
		if fraction == "" {
			return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
		}
	}
	dollars, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
	}
	if dollars < 0 {
		return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
	}
	if dollars > 1_000_000_000 {
		return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
	}
	cents := int64(0)
	if hasFraction {
		padded := fraction + strings.Repeat("0", 2-len(fraction))
		cents, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return 0, errs.New(errs.InvalidRequest, "write a price in dollars, such as 12.50")
		}
	}
	return dollars*100 + cents, nil
}

// usd renders US cents as dollars.
func usd(cents int64) string {
	return fmt.Sprintf("%d.%02d USD", cents/100, cents%100)
}

// usdc renders token base units of a six decimal stablecoin.
func usdc(base int64) string {
	return fmt.Sprintf("%d.%06d USDC", base/1_000_000, base%1_000_000)
}

// configDir returns the directory of the CLI's own files.
func configDir(env *Env) (string, error) {
	if env.ConfigDir != "" {
		return env.ConfigDir, nil
	}
	if base := env.getenv("VADORA_CONFIG_DIR"); base != "" {
		return base, nil
	}
	if base := env.getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "vadora"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.New(errs.InvalidRequest, "the configuration directory could not be resolved")
	}
	if home == "" {
		return "", errs.New(errs.InvalidRequest, "the configuration directory could not be resolved")
	}
	return filepath.Join(home, ".config", "vadora"), nil
}

// writePrivate writes a file that only the owner can read, through a temporary
// file so a crash never leaves half a file.
func writePrivate(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// requireOwnerOnly refuses a secret file that another user can read.
func requireOwnerOnly(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is readable by another user: run chmod 600 on it", path)
	}
	return nil
}

// httpClientOr returns the host's HTTP client or the default one.
func httpClientOr(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// deliveryFlags are the flags that describe where an order ships.
type deliveryFlags struct {
	file       *string
	name       *string
	streets    *stringList
	city       *string
	region     *string
	postalCode *string
	country    *string
	phone      *string
}

// addDeliveryFlags registers the delivery flags on a command.
func addDeliveryFlags(set *flag.FlagSet) deliveryFlags {
	flags := deliveryFlags{streets: &stringList{}}
	flags.file = set.String("delivery-file", "", "a JSON file with the delivery details, which keeps them out of the shell history")
	flags.name = set.String("name", "", "the recipient's name")
	set.Var(flags.streets, "street", "a street line, repeatable")
	flags.city = set.String("city", "", "the city")
	flags.region = set.String("region", "", "the state or region")
	flags.postalCode = set.String("postal-code", "", "the postal code")
	flags.country = set.String("country", "", "the two letter destination country")
	flags.phone = set.String("phone", "", "a contact phone number")
	return flags
}

// delivery assembles the delivery details from a file or from the flags.
func (f deliveryFlags) delivery() (domain.DeliveryDetails, error) {
	if *f.file != "" {
		raw, err := os.ReadFile(*f.file)
		if err != nil {
			return domain.DeliveryDetails{}, errs.New(errs.InvalidRequest, "the delivery file could not be read")
		}
		var details domain.DeliveryDetails
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&details); err != nil {
			return domain.DeliveryDetails{}, errs.New(errs.InvalidRequest, "the delivery file is not the delivery document")
		}
		return details, nil
	}
	details := domain.DeliveryDetails{Name: *f.name, StreetLines: []string(*f.streets), City: *f.city, Region: *f.region,
		PostalCode: *f.postalCode, Country: strings.ToUpper(*f.country), Phone: *f.phone}
	return details, nil
}
