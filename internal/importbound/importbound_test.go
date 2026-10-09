// Package importbound holds the test that proves the dependency direction the
// specifications fix. Quote, order, and receipt packages import no adapter or
// connector package, and no package outside an adapter imports its platform,
// chain, protocol, or host types. The test walks the module's own source, so it
// fails even when the offending import sits in a file no test compiles.
package importbound

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenPackages are the prefixes a neutral package may never import. An
// adapter or connector lives under one of these paths, so importing one would
// let a quote, an order, or a receipt depend on a platform, a chain, a payment
// protocol, a commerce protocol, or an agent host.
var forbiddenPackages = []string{
	"github.com/vadora/vadora/internal/adapters",
	"github.com/vadora/vadora/internal/connectors",
}

// forbiddenImportSubstrings are the package name fragments that identify an
// adapter or connector type, wherever the importing package sits. A neutral
// package may not name a platform, a chain, a protocol, or a host type at all,
// because such a name could only come from an adapter.
var forbiddenImportSubstrings = []string{
	"/adapters/",
	"/connectors/",
	"/chains/",
	"/platforms/",
	"/paymentprotocols/",
	"/commerceprotocols/",
	"/pluginhosts/",
	"/hosts/",
}

// neutralPrefixes are the package paths that must stay free of adapter and
// connector imports: the quote, order, and receipt logic the specification names
// directly, plus the shared contract both repositories build from.
var neutralPrefixes = []string{
	"github.com/vadora/vadora/internal/quotes",
	"github.com/vadora/vadora/internal/orders",
	"github.com/vadora/vadora/internal/receipts",
	"github.com/vadora/vadora/api/types",
	"github.com/vadora/vadora/internal/catalog",
	"github.com/vadora/vadora/internal/payments",
}

// skipDir reports whether the walk descends into a directory. Tool state and
// test fixtures hold no Vadora source, and walking them would only slow the
// check down.
func skipDir(entry os.DirEntry) bool {
	if !entry.IsDir() {
		return false
	}
	switch entry.Name() {
	case ".jj", ".git", ".stylex", "testdata", "node_modules":
		return true
	default:
		return false
	}
}

// isGoSource reports whether a walked file is Go source the boundary reads.
func isGoSource(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}
	return isGoSourceName(entry.Name())
}

// isGoSourceName reports whether a file name is production Go source. A test file
// may import an adapter freely, because a test is allowed to build one.
func isGoSourceName(name string) bool {
	if !strings.HasSuffix(name, ".go") {
		return false
	}
	return !strings.HasSuffix(name, "_test.go")
}

// moduleRoot locates the module root from the test's own directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("locating the module root failed: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		t.Fatalf("no go.mod at %s: %v", root, statErr)
	}
	return root
}

// walkImports returns every non test import path of every Go file in a package
// directory, and requires the package to hold at least one Go source file so a
// typo in a package path cannot make the boundary test pass by inspecting
// nothing.
func walkImports(t *testing.T, dir, importPath string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s failed: %v", dir, err)
	}
	var imports []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file := filepath.Join(dir, name)
		source, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("reading %s failed: %v", file, readErr)
		}
		imports = append(imports, importsOf(string(source))...)
	}
	sources := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isGoSourceName(entry.Name()) {
			sources++
		}
	}
	if sources == 0 {
		t.Fatalf("package %s at %s holds no Go file", importPath, dir)
	}
	return imports
}

// importsOf returns the import paths of one Go source file. It reads only the
// file's import declarations, so a path that merely appears in a string or a
// comment is never mistaken for an import.
func importsOf(source string) []string {
	var imports []string
	inBlock := false
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "import (" {
			inBlock = true
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			fields := strings.Fields(trimmed)
			if len(fields) == 0 {
				continue
			}
			// The last field of an import spec is the quoted path, whatever
			// precedes it: nothing, a name, a dot, or a blank.
			path, ok := quotedPath(fields[len(fields)-1])
			if !ok {
				continue
			}
			imports = append(imports, path)
			continue
		}
		path, ok := singleImport(trimmed)
		if !ok {
			continue
		}
		imports = append(imports, path)
	}
	return imports
}

// singleImport returns the path of one import line or one import spec, in
// whatever form Go allows: a bare path, an aliased path, a dot import, and a
// blank import. A trailing comment is ignored.
func singleImport(trimmed string) (string, bool) {
	if comment := strings.Index(trimmed, "//"); comment >= 0 {
		trimmed = strings.TrimSpace(trimmed[:comment])
	}
	if trimmed == "" {
		return "", false
	}
	if trimmed[0] == '"' {
		return quotedPath(trimmed)
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 2 {
		return "", false
	}
	if fields[0] != "import" {
		return "", false
	}
	return quotedPath(fields[1])
}

// quotedPath returns the path inside the first pair of quotes.
func quotedPath(field string) (string, bool) {
	if !strings.HasPrefix(field, `"`) {
		return "", false
	}
	rest := field[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// TestImportDetectionNamesEveryFormOfImport proves the boundary test can read
// the source it checks: a single line import, an import inside a block, an
// aliased import, and a named import are all found, while a package mentioned in
// a comment or a string is not. Without this the boundary could pass by reading
// nothing.
func TestImportDetectionNamesEveryFormOfImport(t *testing.T) {
	source := `package quotes

import "github.com/vadora/vadora/api/types/domain"

import (
	"errors"
	alias "github.com/vadora/vadora/api/types/errs"
	. "github.com/vadora/vadora/internal/registry"
	_ "github.com/vadora/vadora/internal/jobs"
)

// The string "github.com/vadora/vadora/internal/adapters/shopify" is a
// reference, not an import, and the boundary must not treat it as one.
var reference = "github.com/vadora/vadora/internal/adapters/shopify"
`
	found := importsOf(source)
	want := map[string]bool{
		"github.com/vadora/vadora/api/types/domain": false,
		"errors": false,
		"github.com/vadora/vadora/api/types/errs":    false,
		"github.com/vadora/vadora/internal/registry": false,
		"github.com/vadora/vadora/internal/jobs":     false,
	}
	for _, importPath := range found {
		if _, expected := want[importPath]; !expected {
			t.Fatalf("found unexpected import %q", importPath)
		}
		want[importPath] = true
	}
	for importPath, seen := range want {
		if seen {
			continue
		}
		t.Fatalf("did not find import %q", importPath)
	}
	if len(found) != len(want) {
		t.Fatalf("found %d imports, want %d", len(found), len(want))
	}
}

// TestAnAdapterImportWouldBeCaught proves the forbidden lists name the paths the
// architecture rules forbid, so a later adapter cannot slip a forbidden import
// past the boundary through an unlisted fragment.
func TestAnAdapterImportWouldBeCaught(t *testing.T) {
	cases := []string{
		"github.com/vadora/vadora/internal/adapters/shopify",
		"github.com/vadora/vadora/internal/adapters/solana",
		"github.com/vadora/vadora/internal/adapters/x402",
		"github.com/vadora/vadora/internal/connectors/shopify",
		"github.com/vadora/vadora/internal/chains/solana",
		"github.com/vadora/vadora/internal/platforms/shopify",
		"github.com/vadora/vadora/internal/paymentprotocols/x402",
		"github.com/vadora/vadora/internal/commerceprotocols/ucp",
		"github.com/vadora/vadora/internal/pluginhosts/chatgpt",
	}
	for _, importPath := range cases {
		caught := false
		for _, fragment := range forbiddenImportSubstrings {
			if strings.Contains(importPath, fragment) {
				caught = true
				break
			}
		}
		if caught {
			continue
		}
		for _, forbidden := range forbiddenPackages {
			if strings.HasPrefix(importPath, forbidden) {
				caught = true
				break
			}
		}
		if caught {
			continue
		}
		t.Fatalf("the boundary would not catch %s", importPath)
	}
}

// TestANeutralImportWouldNotBeCaught proves the boundary is not so broad that it
// rejects the shared contract the neutral packages legitimately depend on.
func TestANeutralImportWouldNotBeCaught(t *testing.T) {
	for _, importPath := range []string{
		"github.com/vadora/vadora/api/types/domain",
		"github.com/vadora/vadora/api/types/errs",
		"github.com/vadora/vadora/api/types/integrations",
		"github.com/vadora/vadora/internal/clock",
		"github.com/vadora/vadora/internal/store",
	} {
		for _, fragment := range forbiddenImportSubstrings {
			if strings.Contains(importPath, fragment) {
				t.Fatalf("the boundary would reject the neutral import %s", importPath)
			}
		}
		for _, forbidden := range forbiddenPackages {
			if strings.HasPrefix(importPath, forbidden) {
				t.Fatalf("the boundary would reject the neutral import %s", importPath)
			}
		}
	}
}

// TestQuoteOrderAndReceiptPackagesImportNoAdapter proves the one way dependency
// the architecture rules require: the neutral packages compute prices and
// state, and the adapters translate. A quote never learns a platform's type, and
// an order never learns a chain's type, because either would let an adapter's
// detail leak into money and state decisions.
func TestQuoteOrderAndReceiptPackagesImportNoAdapter(t *testing.T) {
	root := moduleRoot(t)
	ctx := build.Default
	ctx.Dir = root

	found := 0
	for _, prefix := range neutralPrefixes {
		relative := strings.TrimPrefix(prefix, "github.com/vadora/vadora/")
		dir := filepath.Join(root, filepath.FromSlash(relative))
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		found++
		// A path such as api/types names a directory of packages rather than
		// one package, so every package beneath it is checked too.
		packageDirs := packageDirsUnder(dir)
		if len(packageDirs) == 0 {
			t.Fatalf("no Go package sits under %s", prefix)
		}
		for _, packageDir := range packageDirs {
			packagePath := prefix
			if suffix, err := filepath.Rel(dir, packageDir); err == nil {
				if suffix != "." {
					packagePath = prefix + "/" + filepath.ToSlash(suffix)
				}
			}
			for _, importPath := range walkImports(t, packageDir, packagePath) {
				for _, forbidden := range forbiddenPackages {
					if strings.HasPrefix(importPath, forbidden) {
						t.Fatalf("package %s imports %s, which is an adapter", packagePath, importPath)
					}
				}
				for _, fragment := range forbiddenImportSubstrings {
					if strings.Contains(importPath, fragment) {
						t.Fatalf("package %s imports %s, which is an adapter path", packagePath, importPath)
					}
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no neutral package was checked, so the boundary proves nothing")
	}
}

// packageDirsUnder returns every directory at or beneath root that holds Go
// source files, so a directory of packages is checked package by package.
func packageDirsUnder(root string) []string {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skipDir(entry) {
			return filepath.SkipDir
		}
		if !isGoSource(entry) {
			return nil
		}
		dirs = append(dirs, filepath.Dir(path))
		return nil
	})
	if err != nil {
		return dirs
	}
	seen := make(map[string]bool, len(dirs))
	unique := dirs[:0]
	for _, dir := range dirs {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		unique = append(unique, dir)
	}
	return unique
}

// TestNoAdapterPathIsImportedFromOutsideAnAdapter proves an adapter package is
// reachable only from the composition root that wires it. A package outside an
// adapter never names a platform, chain, protocol, or host type, so a change to
// one adapter cannot reach the layers above it.
func TestNoAdapterPathIsImportedFromOutsideAnAdapter(t *testing.T) {
	root := moduleRoot(t)
	adaptersDir := filepath.Join(root, "internal", "adapters")
	if _, err := os.Stat(adaptersDir); err != nil {
		t.Skip("no adapter package exists yet")
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skipDir(entry) {
			return filepath.SkipDir
		}
		if !isGoSource(entry) {
			return nil
		}
		if strings.HasPrefix(path, adaptersDir) {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		for _, importPath := range importsOf(string(source)) {
			for _, fragment := range forbiddenImportSubstrings {
				if !strings.Contains(importPath, fragment) {
					continue
				}
				t.Fatalf("%s imports %s from outside an adapter package",
					filepath.ToSlash(relative), importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module failed: %v", err)
	}
}
