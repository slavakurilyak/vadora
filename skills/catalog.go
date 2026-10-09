// Package skills holds the canonical instruction only workflows.
package skills

import (
	"embed"
	"errors"
	"fmt"
	contract "github.com/vadora/vadora/api/mcp"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed setup/SKILL.md
var source embed.FS

type Entry struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Hosts   []string `json:"hosts"`
	Enabled bool     `json:"enabled"`
}

func Catalog() []Entry {
	return []Entry{{Name: "setup", Version: "0.1.0", Hosts: []string{"chatgpt", "codex"}, Enabled: true}}
}
func Setup() []byte {
	data, err := source.ReadFile("setup/SKILL.md")
	if err != nil {
		panic("canonical setup skill is missing")
	}
	return data
}

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var toolPattern = regexp.MustCompile("`([a-z]+_[a-z_]+)`")

func Validate(name string, data []byte) error {
	if !namePattern.MatchString(name) {
		return errors.New("invalid skill name")
	}
	text := string(data)
	if strings.Count(text, "\n") >= 500 {
		return errors.New("skill exceeds 500 lines")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) != 3 {
		return errors.New("skill frontmatter is missing")
	}
	if parts[0] != "" {
		return errors.New("skill frontmatter must come first")
	}
	var declared, description string
	for _, line := range strings.Split(parts[1], "\n") {
		if strings.HasPrefix(line, "name:") {
			declared = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(line, "description:") {
			description = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "description:")), "\"")
		}
	}
	if declared != name {
		return errors.New("skill name differs from its folder")
	}
	if len(description) < 1 {
		return errors.New("skill description is missing")
	}
	if len(description) > 1024 {
		return errors.New("skill description exceeds 1024 characters")
	}
	if !strings.Contains(text, "Explicit user instructions take priority") {
		return errors.New("skill must respect explicit user instructions")
	}
	for _, phrase := range []string{"vadora buy", "BEGIN PRIVATE KEY", "seed phrase:", "Bearer ", "change ownership", "grant permission", "sign the payment", "run a command"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(phrase)) {
			return errors.New("skill contains a forbidden instruction or credential")
		}
	}
	return validateTools(text)
}

func validateTools(text string) error {
	sets, err := contract.BothToolSets()
	if err != nil {
		return err
	}
	known := make(map[string]bool)
	for _, set := range sets {
		for _, tool := range set.Tools {
			known[tool.Name] = true
		}
	}
	for _, tool := range contract.ExtensionTools() {
		known[tool.Name] = true
	}
	for _, match := range toolPattern.FindAllStringSubmatch(text, -1) {
		if !known[match[1]] {
			return fmt.Errorf("unknown skill tool %q", match[1])
		}
	}
	return nil
}

func ValidateFolder(root fs.FS, folder string, metadata bool) error {
	return fs.WalkDir(root, folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("skill contains a symbolic link")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0111 != 0 {
			return errors.New("skill contains an executable")
		}
		relative := strings.TrimPrefix(path, folder+"/")
		if relative == "SKILL.md" {
			data, err := fs.ReadFile(root, path)
			if err != nil {
				return err
			}
			return Validate(folder[strings.LastIndex(folder, "/")+1:], data)
		}
		if metadata {
			if relative == "agents/openai.yaml" {
				return nil
			}
		}
		return errors.New("skill contains a script, hook, or unsupported file")
	})
}
