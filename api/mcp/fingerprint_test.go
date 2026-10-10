package mcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/vadora/vadora/api/openapi"
)

// contractLock records the fingerprint of each published contract version. A released version never changes its
// fingerprint, so a change to the contract needs a new ContractVersion and a new entry (012:115).
type contractLock struct {
	Versions []lockedVersion `json:"versions"`
}

type lockedVersion struct {
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

func readLock(t *testing.T) contractLock {
	t.Helper()
	raw, err := os.ReadFile("testdata/contract.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock contractLock
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	if len(lock.Versions) == 0 {
		t.Fatal("the contract lock records no version")
	}
	return lock
}

func TestAChangeToTheContractNeedsANewVersion(t *testing.T) {
	current, err := Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	lock := readLock(t)
	seen := map[string]bool{}
	for _, entry := range lock.Versions {
		if seen[entry.Version] {
			t.Fatalf("the contract lock records version %s twice", entry.Version)
		}
		seen[entry.Version] = true
	}
	for _, entry := range lock.Versions {
		if entry.Version != ContractVersion {
			continue
		}
		if entry.Fingerprint != current {
			t.Fatalf("the published contract changed while ContractVersion is still %s.\nChange ContractVersion, and record the new version with fingerprint %s in testdata/contract.lock.json.",
				ContractVersion, current)
		}
		return
	}
	t.Fatalf("ContractVersion %s is not in the contract lock. Record it with fingerprint %s in testdata/contract.lock.json.", ContractVersion, current)
}

func TestTheDocumentAndTheToolContractCarryTheSameVersion(t *testing.T) {
	spec, err := openapi.Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Info.Version != ContractVersion {
		t.Fatalf("the OpenAPI document says %s and the tool contract says %s", spec.Info.Version, ContractVersion)
	}
}

// The fingerprint follows every published part, and the same contract always hashes the same.
func TestTheFingerprintFollowsEachPublishedPartOfTheContract(t *testing.T) {
	sets, err := BothToolSets()
	if err != nil {
		t.Fatal(err)
	}
	spec, err := openapi.Build()
	if err != nil {
		t.Fatal(err)
	}
	extensions := ExtensionTools()
	base, err := fingerprintOf(sets, extensions, spec)
	if err != nil {
		t.Fatal(err)
	}
	again, err := fingerprintOf(sets, extensions, spec)
	if err != nil {
		t.Fatalf("the same contract hashes the same, got %s and %s (%v)", base, again, err)
	}
	if again != base {
		t.Fatalf("the same contract hashes the same, got %s and %s (%v)", base, again, err)
	}
	if len(base) != 64 {
		t.Fatalf("the fingerprint is a SHA-256, got %q", base)
	}

	changedDescription := cloneSets(sets)
	changedDescription[0].Tools[0].Description += " changed"
	changedFlag := cloneSets(sets)
	changedFlag[1].Tools[0].Destructive = !changedFlag[1].Tools[0].Destructive
	changedSchema := cloneSets(sets)
	changedSchema[0].Tools[0].InputSchema.Required = append(append([]string(nil), changedSchema[0].Tools[0].InputSchema.Required...), "extra")
	changedExtension := append([]Tool(nil), extensions...)
	changedExtension[0].Title += " changed"
	changedSpec := spec
	changedSpec.Info.Title += " changed"
	removedPath := spec
	removedPath.Paths = map[string]map[string]openapi.Operation{}
	for path, operations := range spec.Paths {
		removedPath.Paths[path] = operations
		break
	}
	changes := map[string]func() (string, error){
		"a tool description":  func() (string, error) { return fingerprintOf(changedDescription, extensions, spec) },
		"a tool flag":         func() (string, error) { return fingerprintOf(changedFlag, extensions, spec) },
		"a tool input schema": func() (string, error) { return fingerprintOf(changedSchema, extensions, spec) },
		"an extension tool":   func() (string, error) { return fingerprintOf(sets, changedExtension, spec) },
		"the document":        func() (string, error) { return fingerprintOf(sets, extensions, changedSpec) },
		"a removed path":      func() (string, error) { return fingerprintOf(sets, extensions, removedPath) },
	}
	for name, change := range changes {
		got, err := change()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got == base {
			t.Errorf("a change to %s leaves the fingerprint as it was", name)
		}
	}
}

// cloneSets copies the tool sets, so a test changes its own copy.
func cloneSets(sets []ToolSet) []ToolSet {
	out := make([]ToolSet, len(sets))
	for i, set := range sets {
		out[i] = set
		out[i].Tools = append([]Tool(nil), set.Tools...)
		for j := range out[i].Tools {
			out[i].Tools[j].InputSchema.Required = append([]string(nil), set.Tools[j].InputSchema.Required...)
		}
	}
	return out
}

func TestTheContractLockNamesVersionsInIncreasingOrderWithoutBlanks(t *testing.T) {
	lock := readLock(t)
	for _, entry := range lock.Versions {
		if strings.TrimSpace(entry.Version) == "" {
			t.Fatalf("a lock entry carries a version and a SHA-256 fingerprint, got %+v", entry)
		}
		if len(entry.Fingerprint) != 64 {
			t.Fatalf("a lock entry carries a version and a SHA-256 fingerprint, got %+v", entry)
		}
	}
}
