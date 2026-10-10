package chatgpt

import (
	"encoding/json"
	"strings"
	"testing"
)

// The package names Vadora's hosted streamable HTTP endpoint as its single MCP server, carries no server
// code, and holds no lifecycle hook, so the directory takes the hosted endpoint and never Vadora's server
// code (010:77, 012:106).
func TestThePackageListsExactlyOneMCPServerAndNoLifecycleHookOrServerCode(t *testing.T) {
	files, err := PackageFiles()
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(files["mcp.json"], &config); err != nil {
		t.Fatalf("the package's MCP configuration is JSON: %v", err)
	}
	if len(config.Servers) != 1 {
		t.Fatalf("the package lists exactly one MCP server, got %d", len(config.Servers))
	}
	for name, server := range config.Servers {
		if server.Type != "streamable-http" {
			t.Fatalf("%s is a streamable HTTP server, got %q", name, server.Type)
		}
		if server.URL != "https://api.vadora.xyz/mcp" {
			t.Fatalf("%s names the hosted endpoint, got %q", name, server.URL)
		}
	}
	for name, data := range files {
		lower := strings.ToLower(string(data))
		if strings.HasSuffix(name, ".go") {
			t.Errorf("the package carries code: %s", name)
		} else if strings.HasSuffix(name, ".sh") {
			t.Errorf("the package carries code: %s", name)
		} else if strings.HasSuffix(name, ".py") {
			t.Errorf("the package carries code: %s", name)
		}
		for _, hook := range []string{"\"hooks\"", "\"lifecycle\"", "\"postinstall\"", "\"preinstall\"", "\"scripts\""} {
			if strings.Contains(lower, hook) {
				t.Errorf("%s declares a lifecycle hook or a script: %s", name, hook)
			}
		}
	}
	if !strings.Contains(string(files["plugin.json"]), "onboardingSkill") {
		t.Error("the manifest declares the onboarding skill")
	}
}
