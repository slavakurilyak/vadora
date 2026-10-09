package chatgpt

import (
	"embed"
	"github.com/vadora/vadora/skills"
)

//go:embed plugin.json mcp.json skills/setup/SKILL.md skills/setup/agents/openai.yaml
var packaged embed.FS

// PackageFiles returns a fresh copy of the reviewed, instruction only package.
func PackageFiles() (map[string][]byte, error) {
	files := make(map[string][]byte)
	for _, name := range []string{"plugin.json", "mcp.json", "skills/setup/SKILL.md", "skills/setup/agents/openai.yaml"} {
		data, err := packaged.ReadFile(name)
		if err != nil {
			return nil, err
		}
		files[name] = data
	}
	if err := skills.Validate("setup", files["skills/setup/SKILL.md"]); err != nil {
		return nil, err
	}
	return files, nil
}
