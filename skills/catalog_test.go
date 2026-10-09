package skills

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
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
