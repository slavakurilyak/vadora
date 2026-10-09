package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/cli"
)

// runMain runs the public binary's entry point over a command line and returns
// what it wrote to each stream plus the exit code.
func runMain(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	tree := cli.NewClientTree()
	code := cli.Main(tree, args, &out, &errOut, strings.NewReader(""), "0.1.0")
	return out.String(), errOut.String(), code
}

// TestThePublicBinaryReportsServeAsAnUnknownCommand proves the repository split:
// a binary built from the public repository cannot serve, so a deployment cannot
// come from the wrong build.
func TestThePublicBinaryReportsServeAsAnUnknownCommand(t *testing.T) {
	_, errOut, code := runMain(t, "serve")
	if !strings.Contains(errOut, "unknown command serve") {
		t.Fatalf("stderr = %q", errOut)
	}
	if !strings.Contains(errOut, string(errs.InvalidRequest)) {
		t.Fatalf("stderr does not name a catalog code: %q", errOut)
	}
	if code == 0 {
		t.Fatal("serve exited successfully from the public binary")
	}
}

// TestThePublicBinaryListsNoOperatorCommand proves the public help output carries
// no server or :Operator: command, so an operator action cannot be typed against
// this build.
func TestThePublicBinaryListsNoOperatorCommand(t *testing.T) {
	out, _, _ := runMain(t, "help")
	// The check is on the command column, not the whole line, because a command
	// summary may legitimately contain the word invited.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name := strings.Fields(line)
		if len(name) == 0 {
			continue
		}
		for _, forbidden := range []string{"serve", "invite", "operator", "review"} {
			if strings.HasPrefix(name[0], forbidden) {
				t.Fatalf("public help lists the server only command %q:\n%s", forbidden, out)
			}
		}
	}
	for _, required := range []string{"login", "stores connect", "products search", "buy"} {
		if !strings.Contains(out, required) {
			t.Fatalf("public help omits %q:\n%s", required, out)
		}
	}
}

// TestHelpListsEveryClientCommand proves the tree the specification names is the
// tree the binary runs.
func TestHelpListsEveryClientCommand(t *testing.T) {
	tree := cli.NewClientTree()
	names := tree.Names()
	if len(names) != cli.ClientCommandCount {
		t.Fatalf("client commands = %d, want %d", len(names), cli.ClientCommandCount)
	}
	for _, required := range []string{
		"login", "logout",
		"stores connect", "stores sync", "stores list", "stores readiness",
		"products search", "products get",
		"quote", "buy", "test purchase",
		"orders get", "orders list", "payments reconcile",
	} {
		if !cli.IsClientCommand(required) {
			t.Fatalf("the tree omits the client command %q", required)
		}
		if _, present := tree.Lookup(required); !present {
			t.Fatalf("the tree cannot resolve %q", required)
		}
	}
}

// TestTheCatalogIsBrowsableWithoutSigningIn proves public discovery needs no
// credential, because a shopper reads the catalog before creating an account.
func TestTheCatalogIsBrowsableWithoutSigningIn(t *testing.T) {
	out, errOut, code := runMain(t, "products", "search", "wool", "--api", "http://127.0.0.1:1")
	// The call fails because nothing answers on that port, which proves the
	// command reached the network rather than refusing to run unsigned.
	if code == 0 {
		t.Fatalf("a call to a closed port succeeded:\n%s", out)
	}
	if !strings.Contains(errOut, string(errs.StoreUnavailable)) {
		t.Fatalf("stderr = %q", errOut)
	}
}

// TestEveryErrorCarriesOneClientAction proves the CLI reports a code, a reason,
// and the single action the catalog defines, and nothing more.
func TestEveryErrorCarriesOneClientAction(t *testing.T) {
	_, errOut, _ := runMain(t, "orders", "get")
	if !strings.Contains(errOut, "next: ") {
		t.Fatalf("stderr has no client action: %q", errOut)
	}
	lines := strings.Split(strings.TrimSpace(errOut), "\n")
	if len(lines) < 2 {
		t.Fatalf("stderr is too short: %q", errOut)
	}
	if !strings.HasPrefix(lines[1], "next: ") {
		t.Fatalf("second line is not the action: %q", lines[1])
	}
}

// TestAnUnknownCommandIsRejectedBeforeAnyRequest proves a mistyped command never
// reaches the network.
func TestAnUnknownCommandIsRejectedBeforeAnyRequest(t *testing.T) {
	_, errOut, code := runMain(t, "storess", "list")
	if !strings.Contains(errOut, "unknown command storess") {
		t.Fatalf("stderr = %q", errOut)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid input", code)
	}
}

// TestNoCommandIsRejected proves an empty command line asks for a command rather
// than doing nothing.
func TestNoCommandIsRejected(t *testing.T) {
	_, errOut, _ := runMain(t)
	if !strings.Contains(errOut, "name a command") {
		t.Fatalf("stderr = %q", errOut)
	}
}
