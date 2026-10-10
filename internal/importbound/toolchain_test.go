package importbound

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Each chain's official toolchain serves development and testing, and vadora never reimplements or wraps it. On
// every chain it implements no validator or node management, contract or program deployment, keypair generation,
// arbitrary token transfers, swaps, bridges, or investment features (adapters:86, 002:39). The rules below read
// the module's production source with the Go parser, so a call that no test reaches still fails. The test
// doubles are the one place that generates keys, because they never link into the server, and the boundary test
// proves that no production code imports them.

// forbiddenNames are the functions and methods that production code never calls or declares, with the feature
// that each one belongs to.
var forbiddenNames = map[string]string{
	"NewRandomPrivateKey":           "keypair generation",
	"NewWallet":                     "keypair generation",
	"GenerateKey":                   "keypair generation",
	"NewTransferInstruction":        "arbitrary token transfers",
	"NewTransferCheckedInstruction": "arbitrary token transfers",
	"RequestAirdrop":                "validator or faucet features",
}

// forbiddenNamePrefixes are the prefixes of functions that belong to swaps and investment features.
var forbiddenNamePrefixes = map[string]string{
	"Swap":  "swaps",
	"Stake": "investment features",
	"Lend":  "investment features",
	"Yield": "investment features",
}

// forbiddenImports are the import paths, or parts of them, that belong to wallet generation, node management, swaps,
// bridges, and program deployment.
var forbiddenImports = []string{"bip39", "bip32", "go-ethereum", "jupiter", "raydium", "wormhole", "uniswap", "lifi", "debridge",
	"bpfloader", "bpf_loader"}

// forbiddenTools are the program names that production code never runs or names, because running one would wrap
// a chain's own toolchain.
var forbiddenTools = []string{"solana-test-validator", "solana-validator", "solana-keygen", "spl-token", "anvil", "hardhat", "forge"}

// execAllowed names the production files that may import os/exec, with the reason. The signer bridge runs the
// program that the person configures to reach their own remote wallet. It is a wallet interface, and no chain
// toolchain.
var execAllowed = map[string]string{
	"cli/signer.go": "the person's own signer bridge program",
}

// toolchainViolations lists what one production file does that the toolchain rule forbids.
func toolchainViolations(path string, source any) ([]string, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, spec := range parsed.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if imported == "os/exec" {
			if _, allowed := execAllowed[filepath.ToSlash(path)]; !allowed {
				found = append(found, path+" runs programs through os/exec")
			}
		}
		for _, part := range forbiddenImports {
			if strings.Contains(strings.ToLower(imported), part) {
				found = append(found, path+" imports "+imported)
			}
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			found = append(found, forbiddenName(path, typed.Sel.Name)...)
		case *ast.FuncDecl:
			found = append(found, forbiddenName(path, typed.Name.Name)...)
		case *ast.BasicLit:
			if typed.Kind == token.STRING {
				found = append(found, forbiddenTool(path, typed.Value)...)
			}
		}
		return true
	})
	return found, nil
}

func forbiddenName(path, name string) []string {
	if feature, forbidden := forbiddenNames[name]; forbidden {
		return []string{path + " uses " + name + ", which belongs to " + feature}
	}
	for prefix, feature := range forbiddenNamePrefixes {
		if strings.HasPrefix(name, prefix) {
			return []string{path + " uses " + name + ", which belongs to " + feature}
		}
	}
	return nil
}

func forbiddenTool(path, literal string) []string {
	text, err := strconv.Unquote(literal)
	if err != nil {
		return nil
	}
	var found []string
	for _, tool := range forbiddenTools {
		for _, word := range strings.Fields(text) {
			if strings.Trim(word, "\"'`,.;:()") == tool {
				found = append(found, path+" names the program "+tool)
			}
		}
	}
	return found
}

// mustRead reads a file for the parser, and the parser reports the error when the file cannot be read.
func mustRead(path string) any {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return raw
}

// productionToolchainViolations walks the production Go files of the module, outside the test doubles.
func productionToolchainViolations(t *testing.T, root string) ([]string, int) {
	t.Helper()
	var found []string
	read := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".jj", ".git", ".stylex", "testdata", "node_modules", "testdoubles":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		broken, err := toolchainViolations(filepath.ToSlash(relative), mustRead(path))
		if err != nil {
			return err
		}
		read++
		found = append(found, broken...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	return found, read
}

func TestNoProductionCodeWrapsOrReimplementsAChainToolchain(t *testing.T) {
	broken, read := productionToolchainViolations(t, moduleRoot(t))
	if read < 30 {
		t.Fatalf("the rule read only %d production files, so it would pass by inspecting nothing", read)
	}
	if len(broken) > 0 {
		t.Fatalf("production code uses what the toolchain rule forbids:\n%s", strings.Join(broken, "\n"))
	}
}

// TestTheToolchainRuleCatchesEachKindOfViolation plants one violation of each rule.
func TestTheToolchainRuleCatchesEachKindOfViolation(t *testing.T) {
	cases := map[string]string{
		"a generated keypair":      "package x\nimport \"github.com/gagliardetto/solana-go\"\nvar _ = solana.NewRandomPrivateKey\n",
		"a generated ed25519 key":  "package x\nimport \"crypto/ed25519\"\nvar _, _, _ = ed25519.GenerateKey(nil)\n",
		"a token transfer":         "package x\nvar _ = token.NewTransferCheckedInstruction\n",
		"a faucet request":         "package x\nvar _ = client.RequestAirdrop\n",
		"a swap":                   "package x\nfunc SwapTokens() {}\n",
		"a stake":                  "package x\nvar _ = pool.StakeAll\n",
		"a wallet library":         "package x\nimport _ \"github.com/tyler-smith/go-bip39\"\n",
		"a swap aggregator":        "package x\nimport _ \"example.com/jupiter/client\"\n",
		"a program run":            "package x\nimport \"os/exec\"\nvar _ = exec.Command\n",
		"a validator run":          "package x\nvar tool = \"solana-test-validator --reset\"\n",
		"a keygen run":             "package x\nvar tool = `solana-keygen new`\n",
		"a program deployment lib": "package x\nimport _ \"example.com/solana/bpfloader\"\n",
	}
	for name, source := range cases {
		broken, err := toolchainViolations("planted.go", source)
		if err != nil {
			t.Fatalf("%s: parsing the planted source: %v", name, err)
		}
		if len(broken) == 0 {
			t.Errorf("%s: the rule found no violation", name)
		}
	}
	innocent := "package x\nimport \"fmt\"\nfunc Describe() string { return fmt.Sprint(\"a signer bridge and a stakeholder note\") }\n"
	if broken, err := toolchainViolations("innocent.go", innocent); err != nil {
		t.Errorf("the rule refused innocent code: %v %v", broken, err)
	} else if len(broken) != 0 {
		t.Errorf("the rule refused innocent code: %v %v", broken, err)
	}
}
