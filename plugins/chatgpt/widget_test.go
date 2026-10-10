package chatgpt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// widgetTestFiles are the TypeScript test files of the widget that the ledger cites: the link and state rules, the widget in
// the fullscreen view and in the inline card, the widget over the real MCP Apps bridge, the stylesheet, and the preview server.
var widgetTestFiles = []string{"state.test.ts", "widget.test.ts", "inline.test.ts", "protocol.test.ts", "style.test.ts", "preview.test.ts"}

// The widget is TypeScript that the conversation loads, and its tests run with Node in the ui folder. This test runs them
// with the same command that a developer runs, so a failing widget test fails the Go suite too. It runs the real widget
// against a simulated host and against the real MCP Apps bridge, and it checks the stylesheet, which together prove the
// card rules of 011:80, the first render of 011:106, and the checkout hand off of 011:112. It is skipped where Node or the
// installed dependencies are missing, and it fails when no tests run, because an empty run proves nothing.
func TestTheWidgetPassesItsTypeScriptTests(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is not installed, so the widget tests are not run here")
	}
	if _, err := os.Stat(filepath.Join("ui", "node_modules", "@modelcontextprotocol", "ext-apps")); err != nil {
		t.Skip("the widget dependencies are not installed: run npm ci in plugins/chatgpt/ui")
	}
	arguments := []string{"--experimental-strip-types", "--test", "--test-reporter=tap"}
	for _, name := range widgetTestFiles {
		if _, err := os.Stat(filepath.Join("ui", "test", name)); err != nil {
			t.Fatalf("the widget has the test file %s that the ledger cites: %v", name, err)
		}
		arguments = append(arguments, filepath.Join("test", name))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, arguments...)
	command.Dir = "ui"
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the widget tests fail: %v\n%s", err, output)
	}
	counts := regexp.MustCompile(`(?m)^# (tests|pass|fail) (\d+)$`).FindAllStringSubmatch(string(output), -1)
	seen := map[string]int{}
	for _, match := range counts {
		seen[match[1]], _ = strconv.Atoi(match[2])
	}
	if seen["tests"] < 50 {
		t.Fatalf("the widget has at least fifty tests, and the run reports %v\n%s", seen, output)
	}
	if seen["pass"] != seen["tests"] {
		t.Fatalf("every widget test passes, and the run reports %v\n%s", seen, output)
	}
	if seen["fail"] != 0 {
		t.Fatalf("every widget test passes, and the run reports %v\n%s", seen, output)
	}
}
