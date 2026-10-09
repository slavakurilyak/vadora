// Package cli holds Vadora's exported command tree. The :PublicRepository:
// builds this package into a CLI with no serve command, and the
// :PrivateRepository: imports it and adds serve and the :Operator: commands to
// the same tree, so both binaries accept the same flags and report the same
// errors.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/vadora/vadora/api/types/errs"
)

// Command is one command in the tree. Every command shares the same shape, so
// the CLI, the REST layer, and the MCP tools report one error catalog and one
// output format.
type Command struct {
	// Name is the verb a person types.
	Name string
	// Summary is the one line help text.
	Summary string
	// Usage is the argument pattern shown in help.
	Usage string
	// Run performs the command against an Env and returns the result to print.
	Run func(env *Env, args []string) error
}

// Env is what a command needs from its host: the streams to use, and the
// configuration to read. A command reads an environment variable only through
// Getenv, so the CLI stays testable and never depends on ambient state.
type Env struct {
	// Out receives human readable output.
	Out io.Writer
	// Err receives diagnostics.
	Err io.Writer
	// In supplies interactive input, such as the emailed sign in code.
	In io.Reader
	// APIBase is the API origin, without a trailing slash.
	APIBase string
	// JSON asks for structured output instead of human readable text.
	JSON bool
	// Version is the client version reported by the version command.
	Version string
	// ConfigDir holds the credential, the spending policy, and the purchase journal.
	// Empty means VADORA_CONFIG_DIR, then the user's configuration directory.
	ConfigDir string
	// HTTP replaces the SDK transport. A test points it at a local server.
	HTTP *http.Client
	// Getenv reads one setting from the environment. Nil reads none.
	Getenv func(string) string
	// Store replaces the credential store, which is a private file by default.
	Store CredentialStore
	// PollInterval is the pause between reads of an order that a command waits on.
	PollInterval time.Duration
	// WaitFor bounds how long a command waits for an order to be received.
	WaitFor time.Duration

	// secrets are the values that this run loaded and must never print.
	secrets []string
	reader  *bufio.Reader
}

// getenv reads one setting.
func (e *Env) getenv(name string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(name)
}

// Tree is an ordered command tree. A command name is unique within the tree,
// because the same name at two paths would make the help output ambiguous.
type Tree struct {
	commands map[string]Command
	order    []string
}

// NewTree returns an empty tree.
func NewTree() *Tree {
	tree := &Tree{commands: make(map[string]Command, ClientCommandCount), order: nil}
	if len(tree.commands) != 0 {
		panic("cli: a new command tree is not empty")
	}
	if tree.order != nil {
		panic("cli: a new command tree already holds an order")
	}
	return tree
}

// Add registers one command. It panics on a duplicate name, because a duplicate
// would shadow a command a specification names.
func (t *Tree) Add(command Command) {
	if t == nil {
		panic("cli: a command was added to an uninitialized tree")
	}
	if command.Name == "" {
		panic("cli: a command needs a name")
	}
	if command.Run == nil {
		panic("cli: command " + command.Name + " has no implementation")
	}
	if strings.TrimSpace(command.Summary) == "" {
		panic("cli: command " + command.Name + " has no summary")
	}
	if _, exists := t.commands[command.Name]; exists {
		panic("cli: command " + command.Name + " is registered twice")
	}
	t.commands[command.Name] = command
	t.order = append(t.order, command.Name)
	if len(t.order) != len(t.commands) {
		panic("cli: a command was stored under an identifier twice")
	}
}

// Names returns every registered command name in sorted order, so help output
// does not change between runs.
func (t *Tree) Names() []string {
	if t == nil {
		panic("cli: command names were asked of an uninitialized tree")
	}
	names := append([]string(nil), t.order...)
	sort.Strings(names)
	if len(names) != len(t.order) {
		panic("cli: the command list changed length while being copied")
	}
	if !sort.StringsAreSorted(names) {
		panic("cli: command names are not sorted")
	}
	return names
}

// Lookup returns one command by name.
func (t *Tree) Lookup(name string) (Command, bool) {
	if t == nil {
		panic("cli: a command lookup ran on an uninitialized tree")
	}
	command, ok := t.commands[name]
	if !ok {
		return Command{}, false
	}
	if command.Name != name {
		panic("cli: a lookup returned a command under another name")
	}
	return command, true
}

// Help writes the tree's help text.
func (t *Tree) Help(out io.Writer) {
	if out == nil {
		panic("cli: help was asked to write to no stream")
	}
	shown := 0
	for _, name := range t.Names() {
		command := t.commands[name]
		fmt.Fprintf(out, "  %-22s %s\n", command.Usage, command.Summary)
		shown++
	}
	if shown != len(t.order) {
		panic("cli: help printed a different number of commands than the tree holds")
	}
}

// MaxArguments bounds a command line the CLI will parse, so a mistyped paste
// cannot make the tree walk an unbounded argument list.
const MaxArguments = 256

// serverOnlyCommandName is the command the private build adds. The public tree
// must never resolve it.
const serverOnlyCommandName = "serve"

// MaxCommandFlags is the bound Main shares with the command trees.
const MaxCommandFlags = 16

// MaxCommandWords is how many words one command name may hold, so a name such as
// stores connect resolves before its shorter prefixes are tried.
const MaxCommandWords = 3

// Run executes one command by name and returns the catalog error unchanged, so
// the CLI reports the same code and client action as REST and MCP. A name with
// more than one word is resolved as a whole before its first word is tried, so
// a command such as stores sync never collides with a future stores.
func (t *Tree) Run(env *Env, args []string) error {
	if t == nil {
		return errs.New(errs.InvalidRequest, "name a command")
	}
	if len(t.commands) == 0 {
		return errs.New(errs.InvalidRequest, "the command tree is empty")
	}
	if len(args) == 0 {
		return errs.New(errs.InvalidRequest, "name a command")
	}
	if args[0] == "help" {
		t.Help(env.Out)
		return nil
	}
	name, rest := t.resolve(args)
	if name == "" {
		return errs.New(errs.InvalidRequest, "unknown command "+args[0])
	}
	if name == serverOnlyCommandName {
		// The public tree never registers this command, so a public build reports
		// it as unknown. A deployed tree does register it, and there the name
		// resolves normally, so the check below only refuses a tree that claims
		// the command without holding an implementation.
		if _, registered := t.Lookup(name); !registered {
			return errs.New(errs.InvalidRequest, "unknown command "+args[0])
		}
	}
	if len(rest) > len(args) {
		return errs.New(errs.InvalidRequest, "the resolved command kept more arguments than it was given")
	}
	if strings.Contains(name, "  ") {
		return errs.New(errs.InvalidRequest, "unknown command "+args[0])
	}
	command, known := t.Lookup(name)
	if !known {
		return errs.New(errs.InvalidRequest, "unknown command "+args[0])
	}
	if name == "" {
		return errs.New(errs.InvalidRequest, "unknown command "+args[0])
	}
	if env == nil {
		return errs.New(errs.InvalidRequest, "the command has no environment")
	}
	if command.Run == nil {
		return errs.New(errs.InvalidRequest, "the command has no implementation")
	}
	if err := command.Run(env, rest); err != nil {
		return err
	}
	return nil
}

// resolve returns the longest registered command name that the argument list
// starts with, together with the arguments that follow it.
func (t *Tree) resolve(args []string) (string, []string) {
	if len(args) == 0 {
		return "", args
	}
	limit := MaxCommandWords
	if len(args) < limit {
		limit = len(args)
	}
	if len(args) > MaxCommandWords+MaxArguments {
		panic("cli: a command line is longer than the resolver accepts")
	}
	if len(args) == 0 {
		return "", args
	}
	for words := limit; words > 0; words-- {
		name := strings.Join(args[:words], " ")
		if _, known := t.Lookup(name); known {
			if words >= len(args) {
				return name, nil
			}
			return name, args[words:]
		}
	}
	return "", args
}

// Main is the entry point both binaries share. It builds the environment of the
// process and runs the command line. A serve command that the public binary does not
// register therefore reports serve as an unknown command, exactly as the repository
// split requires.
func Main(tree *Tree, args []string, out, errOut io.Writer, in io.Reader, version string) int {
	if errOut == nil {
		errOut = os.Stderr
	}
	if out == nil {
		out = os.Stdout
	}
	return Execute(tree, args, &Env{Out: out, Err: errOut, In: in, Version: version, Getenv: os.Getenv})
}

// Execute runs one command line in a prepared environment and returns the process
// exit code. The global flags --api and --json may stand anywhere on the line. A
// catalog error becomes one line with its code and reason and one line with the
// single client action.
func Execute(tree *Tree, args []string, env *Env) int {
	if tree == nil {
		fmt.Fprintf(env.Err, "error: the command tree is missing\n")
		return 1
	}
	if len(tree.Names()) == 0 {
		fmt.Fprintf(env.Err, "error: the command tree is missing\n")
		return 1
	}
	if len(args) > MaxArguments {
		fmt.Fprintf(env.Err, "error: the command line is too long\n")
		return 2
	}
	if env.Version == "" {
		fmt.Fprintf(env.Err, "error: the binary reports no version\n")
		return 1
	}
	api, rest, err := extractGlobals(args, env)
	if err != nil {
		return report(env.Err, err)
	}
	env.APIBase = strings.TrimRight(api, "/")
	if err := checkOrigin(env.APIBase); err != nil {
		return report(env.Err, err)
	}
	if err := tree.Run(env, rest); err != nil {
		return report(env.Err, err)
	}
	return 0
}

// extractGlobals removes --api and --json from a command line and returns the API
// origin and the remaining arguments.
func extractGlobals(args []string, env *Env) (string, []string, error) {
	api := defaultAPIBase
	var rest []string
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json" || argument == "-json":
			env.JSON = true
		case argument == "--api" || argument == "-api":
			if index+1 >= len(args) {
				return "", nil, errs.New(errs.InvalidRequest, "--api needs an origin")
			}
			index++
			api = args[index]
		case strings.HasPrefix(argument, "--api="):
			api = strings.TrimPrefix(argument, "--api=")
		default:
			rest = append(rest, argument)
		}
	}
	return api, rest, nil
}

// checkOrigin accepts an https origin, or a plain http origin only on a loopback
// address, so a credential never travels unencrypted to a remote host.
func checkOrigin(origin string) error {
	if strings.HasPrefix(origin, "https://") {
		return nil
	}
	for _, local := range []string{"http://127.0.0.1", "http://localhost", "http://[::1]"} {
		if strings.HasPrefix(origin, local) {
			return nil
		}
	}
	return errs.New(errs.InvalidRequest, "the API origin must use https")
}

// defaultAPIBase is the API origin the CLI uses when the caller names none. It
// is the production origin, never a local address, so a default build cannot
// send a credential anywhere else.
const defaultAPIBase = "https://api.vadora.xyz"

// report writes one catalog error as a code, a reason, and the single client
// action, and returns the process exit code.
func report(errOut io.Writer, err error) int {
	if err == nil {
		panic("cli: a failure report was asked to report no error")
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	catalogErr, known := errs.As(err)
	if !known {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	if catalogErr.Code == "" {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(errOut, "%s: %s\n", catalogErr.Code, catalogErr.Reason)
	fmt.Fprintf(errOut, "next: %s\n", catalogErr.Action)
	if catalogErr.Reason == "" {
		fmt.Fprintf(errOut, "next: %s\n", errs.ActionCorrectInput)
	}
	for key, value := range catalogErr.Details {
		fmt.Fprintf(errOut, "  %s: %s\n", key, value)
	}
	if catalogErr.RetryAfterSeconds > 0 {
		fmt.Fprintf(errOut, "  retry_after_seconds: %d\n", catalogErr.RetryAfterSeconds)
	}
	return exitCodeFor(catalogErr)
}

// exitCodeFor maps a catalog code to a process exit code, so a script can branch
// on the failure class without parsing the message.
func exitCodeFor(err *errs.Error) int {
	if err == nil {
		panic("cli: an exit code was asked for no error")
	}
	if err.Action == "" {
		panic("cli: a catalog error carries no client action")
	}
	switch err.Code {
	case errs.InvalidRequest, errs.ShippingUnsupported, errs.ProductUnavailable:
		return 2
	case errs.Unauthenticated:
		return 3
	case errs.NotFound:
		return 4
	case errs.RateLimited, errs.QuoteLimitReached, errs.PaymentPending,
		errs.OrderSyncPending, errs.ReviewRequired:
		return 5
	case errs.QuoteExpired, errs.PaymentRejected:
		return 6
	case errs.StoreUnsupported, errs.PaymentMethodUnsupported, errs.AdapterUnavailable:
		return 7
	case errs.ConfirmationRequired:
		return 8
	case errs.StoreDisconnected:
		return 9
	default:
		return 1
	}
}

// PrintError writes a catalog error and nothing else, for a command that has
// already chosen its own output format.
func PrintError(errOut io.Writer, err error) {
	if err == nil {
		return
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	if _, known := errs.As(err); !known {
		panic("cli: PrintError was asked to print an error outside the catalog")
	}
	if !IsCatalogError(err) {
		panic("cli: a catalog error was not recognised as one")
	}
	report(errOut, err)
}

// IsCatalogError reports whether an error came from the :ErrorCatalog:. A
// command reports a plain error only for a fault inside the client itself.
func IsCatalogError(err error) bool {
	if err == nil {
		return false
	}
	_, known := errs.As(err)
	if !known {
		return false
	}
	if len(errs.LeafActions()) == 0 {
		panic("cli: the catalog lost every client action")
	}
	if _, known := errs.As(err); !known {
		return false
	}
	if !known {
		return false
	}
	return true
}
