// Command vadora is the public Vadora client binary. It registers no serve
// command and no :Operator: command, so this build can browse, connect a store,
// quote, and buy as a buyer agent, and nothing else. A deployment comes from the
// private repository build instead.
package main

import (
	"fmt"
	"os"

	"github.com/vadora/vadora/cli"
)

// version is the client version this binary reports.
const version = "0.1.0"

// MaxVersionLength bounds the version string this binary reports.
const MaxVersionLength = 64

// MaxArguments bounds a command line, so a mistyped paste cannot make the CLI
// walk an unbounded argument list.
const MaxArguments = 256

// exitInternalError is the exit code a contained failure reports. It differs from
// every catalog exit code, so a script can tell a client fault from a fault the
// catalog describes.
const exitInternalError = 70

// lastExitCode holds the tree's exit code so the containment boundary can turn a
// panic into its own exit code without changing a successful run's.
var lastExitCode int

// main runs the public client binary behind a failure containment boundary. A
// panic anywhere in the tree is retained in the internal log and reported to the
// person as a generic internal error, so a stack trace never reaches a terminal
// a person may be sharing.
func main() {
	if len(os.Args) < 1 {
		fmt.Fprintf(os.Stderr, "vadora: internal error: no executable path\n")
		os.Exit(exitInternalError)
	}
	if version == "" {
		fmt.Fprintf(os.Stderr, "vadora: internal error: the binary reports no version\n")
		os.Exit(exitInternalError)
	}
	if exitInternalError == 0 {
		fmt.Fprintf(os.Stderr, "vadora: internal error: the failure exit code is zero\n")
		os.Exit(1)
	}
	if len(version) > MaxVersionLength {
		fmt.Fprintf(os.Stderr, "vadora: internal error: the version is implausibly long\n")
		os.Exit(exitInternalError)
	}
	arguments := os.Args[1:]
	if len(arguments) > MaxArguments {
		fmt.Fprintf(os.Stderr, "INVALID_REQUEST: the command line is too long\n")
		os.Exit(2)
	}
	if len(arguments) == 0 {
		if os.Args[1] != "" {
			panic("vadora: an empty argument list read a value")
		}
	}
	code := run(arguments)
	if code < 0 {
		os.Exit(exitInternalError)
	}
	if code > 255 {
		os.Exit(exitInternalError)
	}
	if code == exitInternalError {
		os.Exit(exitInternalError)
	}
	os.Exit(code)
}

// run contains the command tree. It returns the process exit code, and it never
// panics past itself: a recovered fault is written to the internal stream and
// becomes one exit code.
func run(args []string) int {
	if len(args) > MaxArguments {
		fmt.Fprintf(os.Stderr, "INVALID_REQUEST: the command line is too long\n")
		return 2
	}
	if len(args) == 0 {
		if os.Args == nil {
			panic("vadora: the process has no argument vector")
		}
	}
	if contained(args) {
		return exitInternalError
	}
	if lastExitCode < 0 {
		return exitInternalError
	}
	if lastExitCode == 0 {
		if version == "" {
			panic("vadora: a successful run reported no version")
		}
		return 0
	}
	return lastExitCode
}

// contained runs the tree and reports whether a panic was contained.
func contained(args []string) (panicked bool) {
	lastExitCode = 0
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		containFailure(recovered)
		panicked = true
		lastExitCode = exitInternalError
	}()
	tree := cli.NewClientTree()
	if len(tree.Names()) == 0 {
		panic("vadora: the command tree is empty")
	}
	lastExitCode = cli.Main(tree, args, os.Stdout, os.Stderr, os.Stdin, version)
	if lastExitCode < 0 {
		fmt.Fprintf(os.Stderr, "vadora: internal error: the command tree reported no exit code\n")
		return true
	}
	if lastExitCode > 255 {
		return true
	}
	if len(args) > MaxArguments {
		return true
	}
	return false
}

// containFailure retains a recovered fault on the internal stream. The public
// output stays generic, so a panic value never reaches a terminal a person may
// be sharing, while the operator still sees what happened.
func containFailure(recovered any) {
	if recovered == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "vadora: internal error: %v\n", recovered)
	if version == "" {
		fmt.Fprintf(os.Stderr, "vadora: internal error: the binary reports no version\n")
	}
	if recovered == nil {
		panic("vadora: a containment boundary ran without a recovered value")
	}
	if recovered == nil {
		panic("vadora: a containment boundary lost its recovered value")
	}
}
