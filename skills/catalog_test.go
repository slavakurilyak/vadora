package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vadora/vadora/api/types/integrations"
)

func TestCanonicalSkillAndCatalog(t *testing.T) {
	if err := Validate("setup", Setup()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFolder(os.DirFS("."), "setup", false); err != nil {
		t.Fatal(err)
	}
	entries := Catalog()
	if len(entries) != 1 {
		t.Fatal("launch has extra skills")
	}
	if !entries[0].Enabled {
		t.Fatal("setup is disabled")
	}
	if strings.Join(entries[0].Hosts, ",") != "chatgpt,codex" {
		t.Fatal("unexpected host is published")
	}
}
func TestSkillValidatorRejectsUnsupportedInstructionsAndFiles(t *testing.T) {
	for _, change := range []string{"`unknown_tool`", "vadora buy", "BEGIN PRIVATE KEY", "change ownership", "grant permission"} {
		data := append(Setup(), []byte("\n"+change)...)
		if err := Validate("setup", data); err == nil {
			t.Fatalf("accepted %q", change)
		}
	}
	files := fstest.MapFS{"setup/SKILL.md": {Data: Setup()}, "setup/run.sh": {Data: []byte("bad"), Mode: 0755}}
	if err := ValidateFolder(files, "setup", false); err == nil {
		t.Fatal("script accepted")
	}
	if err := Validate("wrong", Setup()); err == nil {
		t.Fatal("folder mismatch accepted")
	}
}

// Every skill folder has a catalog entry, and every entry has a skill folder (013:62).
func TestEveryFolderHasACatalogEntryAndEveryEntryHasAFolder(t *testing.T) {
	folders, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	onDisk := map[string]bool{}
	for _, folder := range folders {
		if folder.IsDir() {
			onDisk[folder.Name()] = true
		}
	}
	listed := map[string]bool{}
	for _, entry := range Catalog() {
		listed[entry.Name] = true
		if !onDisk[entry.Name] {
			t.Errorf("the catalog lists %q, which has no skill folder", entry.Name)
		}
	}
	for name := range onDisk {
		if !listed[name] {
			t.Errorf("the folder %q has no catalog entry", name)
		}
	}
}

// A skill passes the format only with a matching name of the allowed pattern, a description of
// 1 to 1024 characters, and a SKILL.md of under 500 lines (013:63).
func TestTheSkillFormatLimitsAreEnforced(t *testing.T) {
	skill := func(name, description string, extraLines int) []byte {
		text := "---\nname: " + name + "\ndescription: \"" + description + "\"\n---\n\nExplicit user instructions take priority over these guidelines.\n"
		return []byte(text + strings.Repeat("A line.\n", extraLines))
	}
	if err := Validate("setup", skill("setup", "Set up Vadora.", 10)); err != nil {
		t.Fatalf("a valid skill was refused: %v", err)
	}
	if err := Validate("setup", skill("setup", strings.Repeat("d", 1024), 10)); err != nil {
		t.Fatalf("a 1024 character description is allowed: %v", err)
	}
	refused := map[string][]byte{
		"an empty description":        skill("setup", "", 10),
		"a 1025 character":            skill("setup", strings.Repeat("d", 1025), 10),
		"a file of 500 lines":         skill("setup", "Set up Vadora.", 500),
		"a name that is not the file": skill("other", "Set up Vadora.", 10),
	}
	for name, data := range refused {
		if err := Validate("setup", data); err == nil {
			t.Errorf("%s: the skill was accepted", name)
		}
	}
	for _, bad := range []string{"Setup", "-setup", "setup-", "set--up", "set_up", "", strings.Repeat("a", 65)} {
		if err := Validate(bad, skill(bad, "Set up Vadora.", 10)); err == nil {
			t.Errorf("the name %q was accepted", bad)
		}
	}
	if err := Validate(strings.Repeat("a", 64), skill(strings.Repeat("a", 64), "Set up Vadora.", 10)); err != nil {
		t.Fatalf("a name of 64 characters is allowed: %v", err)
	}
	if err := Validate("set-up", skill("set-up", "Set up Vadora.", 10)); err != nil {
		t.Fatalf("a name with single hyphens is allowed: %v", err)
	}
}

// A skill description set that exceeds the Codex initial list budget fails the check, and the
// launch set fits it with room to spare (013:65).
func TestADescriptionSetOverTheCodexListBudgetFailsTheCheck(t *testing.T) {
	launch, err := LaunchListItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(launch) != 1 {
		t.Fatalf("the launch set lists one skill for Codex, got %+v", launch)
	}
	if err := CheckCodexBudget(launch); err != nil {
		t.Fatalf("the launch skill fits the budget: %v", err)
	}
	var crowded []ListItem
	for i := 0; i < 20; i++ {
		crowded = append(crowded, ListItem{Name: "skill-number-" + strings.Repeat("x", i), Description: strings.Repeat("d", 300)})
	}
	err = CheckCodexBudget(crowded)
	if err == nil {
		t.Fatal("a skill set over the budget was accepted")
	}
	if !strings.Contains(err.Error(), "8000") {
		t.Fatalf("the failure names the budget, got %v", err)
	}
	if CodexListBudget != 8000 {
		t.Fatalf("the budget is the published 8,000 characters for an unknown context window, got %d", CodexListBudget)
	}
}

// A package copy of a skill differs from its canonical SKILL.md in no byte, and only an
// OpenAI host's package copy carries the metadata file (013:64, 013:36).
func TestThePackageCopyOfASkillDiffersFromTheCanonicalOneInNoByteAndOnlyOpenAIAddsMetadata(t *testing.T) {
	canonical, err := os.ReadFile("setup/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	packaged, err := os.ReadFile("../plugins/chatgpt/skills/setup/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != string(packaged) {
		t.Fatal("the ChatGPT package copy of the setup skill differs from the canonical SKILL.md")
	}
	entries, err := os.ReadDir("setup")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("the canonical skill holds SKILL.md only, so it stays host independent, got %d entries", len(entries))
	}
	if err := ValidateFolder(os.DirFS("../plugins/chatgpt"), "skills/setup", true); err != nil {
		t.Fatalf("the OpenAI package copy holds SKILL.md and the metadata file: %v", err)
	}
	if err := ValidateFolder(os.DirFS("../plugins/chatgpt"), "skills/setup", false); err == nil {
		t.Fatal("a host that is not an OpenAI host accepted the metadata file")
	}
	metadata, err := os.ReadFile("../plugins/chatgpt/skills/setup/agents/openai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(metadata), "allow_implicit_invocation:") {
		t.Fatal("the metadata file sets implicit invocation deliberately (013:48)")
	}
}

// The ChatGPT manifest names the onboarding skill in its OpenAI extension, and the path
// resolves to a skill that the package carries (013:73).
func TestTheManifestNamesTheOnboardingSkillAtAPathThatResolves(t *testing.T) {
	raw, err := os.ReadFile("../plugins/chatgpt/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Extensions map[string]struct {
			OnboardingSkill string `json:"onboardingSkill"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	path := manifest.Extensions["com.openai"].OnboardingSkill
	if path != "./skills/setup/SKILL.md" {
		t.Fatalf("the manifest declares the onboarding skill under extensions.com.openai, got %q", path)
	}
	if _, err := os.Stat(filepath.Join("../plugins/chatgpt", path)); err != nil {
		t.Fatalf("the declared path resolves to a file of the package: %v", err)
	}
}

// A skill is published only for a host that the launch scope enables: ChatGPT and Codex, and
// never Claude Code, which reads Coming soon (013:67).
func TestASkillIsPublishedOnlyForAHostThatTheLaunchScopeEnables(t *testing.T) {
	enabled := map[string]bool{"chatgpt": true, "codex": true}
	for _, entry := range Catalog() {
		for _, host := range entry.Hosts {
			if !enabled[host] {
				t.Errorf("the skill %q is published for %q, which the launch scope does not enable", entry.Name, host)
			}
		}
	}
	for _, item := range integrations.All() {
		if item.Category != integrations.AgentHosts {
			continue
		}
		if item.Status == integrations.StatusAvailable {
			if !enabled[item.ID] {
				t.Errorf("the launch scope enables the host %q, so the skill set must name it", item.ID)
			}
		}
	}
}
