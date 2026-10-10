package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/vadora/vadora/api/openapi"
)

// Fingerprint is the SHA-256 of the whole published contract: the shopper and merchant tool sets, the extension
// tools, and the OpenAPI document with its wire types and error catalog. Any change to a published schema, a
// description, a flag, a path, or an error code changes it. The contract lock records the fingerprint of each
// contract version, so a change needs a new ContractVersion, and the private repository adopts the contract by
// the version that it records and not by what a neighbouring directory holds (012:115).
func Fingerprint() (string, error) {
	sets, err := BothToolSets()
	if err != nil {
		return "", err
	}
	spec, err := openapi.Build()
	if err != nil {
		return "", err
	}
	return fingerprintOf(sets, ExtensionTools(), spec)
}

// fingerprintOf hashes one contract. The encoder writes map keys in sorted order and slices in order, so the same
// contract always hashes the same.
func fingerprintOf(sets []ToolSet, extensions []Tool, spec openapi.Specification) (string, error) {
	document := struct {
		ContractVersion string                `json:"contract_version"`
		ProtocolVersion string                `json:"protocol_version"`
		Sets            []ToolSet             `json:"tool_sets"`
		Extensions      []Tool                `json:"extension_tools"`
		OpenAPI         openapi.Specification `json:"openapi"`
	}{ContractVersion, ProtocolVersion, sets, extensions, spec}
	raw, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("mcp: encoding the contract: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
