package importbound

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// privateModule is the private repository's module path. The public module never
// imports, links, or names a package of it (012:57, 012:87).
const privateModule = "github.com/vadora/vadora-private"

// TestThePublicModuleImportsNoPrivatePackage walks every Go file of the public
// module, tests included, and refuses any import of a private package.
func TestThePublicModuleImportsNoPrivatePackage(t *testing.T) {
	root := moduleRoot(t)
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skipDir(entry) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if imported == privateModule || strings.HasPrefix(imported, privateModule+"/") {
				t.Errorf("%s imports the private package %s", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if checked == 0 {
		t.Fatal("the check read no Go file, so it would pass by inspecting nothing")
	}
}
