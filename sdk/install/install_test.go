package install

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vadora/vadora/skills"
)

func TestPluginInstallIsIdempotentAndPreservesExistingEntries(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents/plugins")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	seed := []byte(`{"name":"my-marketplace","custom":"keep","plugins":[{"name":"other","source":{"source":"local","path":"./other"}}]}`)
	if err := os.WriteFile(filepath.Join(path, "marketplace.json"), seed, 0644); err != nil {
		t.Fatal(err)
	}
	first, err := Plugin(root, "chatgpt")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Changed {
		t.Fatal("first install reported no change")
	}
	second, err := Plugin(root, "chatgpt")
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatal("repeat install rewrote files")
	}
	data, err := os.ReadFile(filepath.Join(path, "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value["custom"] != "keep" {
		t.Fatal("other marketplace fields changed")
	}
	entries := value["plugins"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries %v", entries)
	}
	if entries[0].(map[string]any)["name"] != "other" {
		t.Fatal("existing entry changed")
	}
	data, err = os.ReadFile(filepath.Join(first.Path, "skills/setup/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(skills.Setup()) {
		t.Fatal("installed skill differs from canonical copy")
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("installer wrote an unrelated policy")
	}
}
func TestInstallerRejectsSymlinkAndUnavailableHost(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
		t.Fatal(err)
	}
	if _, err := Plugin(root, "chatgpt"); err == nil {
		t.Fatal("symlink accepted")
	}
	items, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatal("installer wrote outside root")
	}
	if _, err := Plugin(t.TempDir(), "claude-code"); err == nil {
		t.Fatal("deferred host installed")
	}
}
func TestStandaloneSkillWritesOnlyCanonicalInstruction(t *testing.T) {
	root := t.TempDir()
	result, err := Skill(root, "setup", "codex")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("standalone skill has unexpected files")
	}
	if entries[0].Name() != "SKILL.md" {
		t.Fatal("standalone metadata or executable was installed")
	}
}

// Installing a plugin locally writes the skill files and the manifest, and nothing that runs a
// command: no script, no executable, no hook, and no file outside the package and the local
// marketplace entry (013:76, 012:105).
func TestInstallingAPluginWritesNothingThatRunsACommand(t *testing.T) {
	root := t.TempDir()
	if _, err := Plugin(root, "chatgpt"); err != nil {
		t.Fatal(err)
	}
	var written []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		written = append(written, filepath.ToSlash(relative))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o111 != 0 {
			t.Errorf("%s is executable", relative)
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".json", ".yaml":
		default:
			t.Errorf("%s is not an instruction or a manifest", relative)
		}
		if strings.Contains(strings.ToLower(relative), "hook") {
			t.Errorf("%s is a hook", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".agents/plugins/marketplace.json", ".agents/plugins/vadora/plugin.json", ".agents/plugins/vadora/mcp.json",
		".agents/plugins/vadora/skills/setup/SKILL.md", ".agents/plugins/vadora/skills/setup/agents/openai.yaml"} {
		if !slices.Contains(written, want) {
			t.Errorf("the package lacks %s, wrote %v", want, written)
		}
	}
	if len(written) != 5 {
		t.Fatalf("the install writes the package and the marketplace entry only, wrote %v", written)
	}
}
