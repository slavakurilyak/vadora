// Package install writes reviewed local plugin packages and standalone skills.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/vadora/vadora/plugins/chatgpt"
	"github.com/vadora/vadora/skills"
	"os"
	"path/filepath"
)

type Result struct {
	Path    string
	Changed bool
}

// Plugin writes only the ChatGPT package and a local marketplace entry.
func Plugin(root, host string) (Result, error) {
	if host != "chatgpt" {
		return Result{}, errors.New("this plugin host is Coming soon")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	files, err := chatgpt.PackageFiles()
	if err != nil {
		return Result{}, err
	}
	target := filepath.Join(".agents", "plugins", "vadora")
	result := Result{Path: filepath.Join(root, target)}
	for name, data := range files {
		changed, err := write(root, filepath.Join(target, filepath.FromSlash(name)), data)
		if err != nil {
			return Result{}, err
		}
		if changed {
			result.Changed = true
		}
	}
	changed, err := marketplace(root)
	if err != nil {
		return Result{}, err
	}
	if changed {
		result.Changed = true
	}
	return result, nil
}

// Skill writes the canonical workflow to the selected Codex skills root.
func Skill(root, name, host string) (Result, error) {
	if host != "codex" {
		return Result{}, errors.New("standalone skills support Codex")
	}
	if name != "setup" {
		return Result{}, errors.New("unknown skill")
	}
	data := skills.Setup()
	if err := skills.Validate(name, data); err != nil {
		return Result{}, err
	}
	changed, err := write(root, filepath.Join(name, "SKILL.md"), data)
	return Result{Path: filepath.Join(root, name), Changed: changed}, err
}

func marketplace(root string) (bool, error) {
	path := filepath.Join(".agents", "plugins", "marketplace.json")
	if err := safePath(root, path); err != nil {
		return false, err
	}
	value := map[string]any{"name": "vadora-local", "plugins": []any{}}
	data, err := os.ReadFile(filepath.Join(root, path))
	if err == nil {
		if err := json.Unmarshal(data, &value); err != nil {
			return false, errors.New("local marketplace is invalid")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	items, ok := value["plugins"].([]any)
	if !ok {
		return false, errors.New("local marketplace plugins must be an array")
	}
	entry := map[string]any{"name": "vadora", "source": map[string]any{"source": "local", "path": "./.agents/plugins/vadora"}, "policy": map[string]any{"installation": "AVAILABLE", "authentication": "ON_INSTALL"}, "category": "Productivity"}
	found := false
	for i, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return false, errors.New("local marketplace entry is invalid")
		}
		if object["name"] == "vadora" {
			items[i] = entry
			found = true
		}
	}
	if !found {
		items = append(items, entry)
	}
	value["plugins"] = items
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return false, err
	}
	return write(root, path, append(encoded, '\n'))
}

func safePath(root, relative string) error {
	if filepath.IsAbs(relative) {
		return errors.New("installation path must be relative")
	}
	clean := filepath.Clean(relative)
	if clean == ".." {
		return errors.New("installation path leaves the selected root")
	}
	path := root
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("selected root is a symbolic link")
	}
	if !info.IsDir() {
		return errors.New("selected root is not a directory")
	}
	for _, part := range splitPath(clean) {
		if part == ".." {
			return errors.New("installation path leaves the selected root")
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("installation path contains a symbolic link")
		}
	}
	return nil
}

func splitPath(path string) []string {
	var parts []string
	for path != "." {
		directory, file := filepath.Split(path)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		next := filepath.Clean(directory)
		if next == path {
			break
		}
		path = next
	}
	return parts
}

func write(root, relative string, data []byte) (bool, error) {
	if err := safePath(root, relative); err != nil {
		return false, err
	}
	path := filepath.Join(root, relative)
	existing, err := os.ReadFile(path)
	if err == nil {
		if bytes.Equal(existing, data) {
			return false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".vadora-install-")
	if err != nil {
		return false, err
	}
	temporary := file.Name()
	defer func() {
		if err := os.Remove(temporary); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return
			}
		}
	}()
	if _, err := file.Write(data); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return false, closeErr
		}
		return false, err
	}
	if err := file.Chmod(0644); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return false, closeErr
		}
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temporary, path); err != nil {
		return false, err
	}
	return true, nil
}
