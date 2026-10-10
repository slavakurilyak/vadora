package mcp

import "github.com/vadora/vadora/api/openapi"

// Resource is a host independent registration supplied by a plugin adapter.
type Resource struct {
	URI      string
	MIMEType string
	Text     string
	Meta     map[string]any
}

type Icon struct {
	Src      string   `json:"src"`
	MIMEType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// Presentation carries optional metadata without coupling the server to a host.
type Presentation interface {
	Metadata(string) map[string]any
	Resources() []Resource
	Icons(string) []Icon
	Capabilities() map[string]any
	// HostSession reads the host's own identifier of the conversation from the request
	// metadata of a tool call, and answers an empty text when the host supplies none. The
	// server bounds anonymous catalog calls with it and never identifies anyone by it.
	HostSession(map[string]any) string
}

// ExtensionTools are presentation tools that call the existing services.
func ExtensionTools() []Tool {
	empty := openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{}}
	read := func(name, title, scope string) Tool {
		return Tool{Name: name, Title: title, Description: "Reads the account view. May return: " + codeList(sortedCodes()), InputSchema: empty, ReadOnly: true, RequiresAccount: scope != "", Scope: scope}
	}
	tools := []Tool{read("open_marketplace", "Marketplace", ""), read("open_cart", "Cart", "shopper"), read("read_settings", "Preferences", "shopper")}
	tools = append(tools, Tool{Name: "update_settings", Title: "Update preferences", Description: "Overwrites the changed display preferences only. May return: " + codeList(sortedCodes()), RequiresAccount: true, Scope: "shopper", Destructive: true,
		InputSchema: openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{
			"mode":                {Type: "string", Enum: []string{"shop", "sell", "both"}, MaxLength: lengthPointer(4)},
			"destination_country": {Type: "string", MaxLength: lengthPointer(2)},
		}}})
	tools = append(tools, Tool{Name: "search_mentions", Title: "Find products to mention", Description: "Reads published products and stores matching a typeahead query. May return: " + codeList(sortedCodes()), ReadOnly: true, RequiresAccount: true, Scope: "shopper",
		InputSchema: openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{"query": stringField("The typeahead search, at most 128 characters.")}, Required: []string{"query"}}})
	return tools
}

// SchemaJSON renders the shallow tool schemas as draft 2020-12 JSON Schema.
// The public tool contract has at most one object and one array nesting level.
func SchemaJSON(schema openapi.Schema) map[string]any {
	result := fieldJSON(schema)
	properties := make(map[string]any, len(schema.Properties))
	for name, field := range schema.Properties {
		value := fieldJSON(field)
		if field.Items != nil {
			value["items"] = fieldJSON(*field.Items)
		}
		properties[name] = value
	}
	result["properties"] = properties
	result["additionalProperties"] = false
	return result
}

func fieldJSON(field openapi.Schema) map[string]any {
	value := map[string]any{"type": field.Type}
	if field.Description != "" {
		value["description"] = field.Description
	}
	if field.MaxLength != nil {
		value["maxLength"] = *field.MaxLength
	}
	if field.MinLength != nil {
		value["minLength"] = *field.MinLength
	}
	if field.MaxItems != nil {
		value["maxItems"] = *field.MaxItems
	}
	if field.Minimum != nil {
		value["minimum"] = *field.Minimum
	}
	if field.Maximum != nil {
		value["maximum"] = *field.Maximum
	}
	if len(field.Required) > 0 {
		value["required"] = field.Required
	}
	if len(field.Enum) > 0 {
		value["enum"] = field.Enum
	}
	if field.Type == "object" {
		value["additionalProperties"] = field.AdditionalProperties
	}
	return value
}
