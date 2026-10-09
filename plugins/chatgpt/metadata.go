package chatgpt

// Metadata mirrors OpenAIUiToolMetadata with MCP Apps visibility.
type Metadata struct {
	UI              *ToolUI          `json:"ui,omitempty"`
	OpenAIUI        *Entrypoints     `json:"openai/ui,omitempty"`
	Extensions      *ToolExtensions  `json:"openai/extensions,omitempty"`
	SecuritySchemes []SecurityScheme `json:"securitySchemes"`
}
type ToolUI struct {
	ResourceURI string   `json:"resourceUri,omitempty"`
	Visibility  []string `json:"visibility,omitempty"`
}
type Entrypoints struct {
	Entrypoints []Entrypoint `json:"entrypoints"`
}
type Entrypoint struct {
	Type string `json:"type"`
}
type ToolExtensions struct {
	MentionSearch *struct{} `json:"mentions/search,omitempty"`
}
type SecurityScheme struct {
	Type   string   `json:"type"`
	Scopes []string `json:"scopes,omitempty"`
}

// ResourceMetadata mirrors MCP Apps resource security and display mode fields.
type ResourceMetadata struct {
	UI        ResourceUI   `json:"ui"`
	OpenAIUI  DisplayModes `json:"openai/ui"`
	LegacyCSP LegacyCSP    `json:"openai/widgetCSP"`
}
type ResourceUI struct {
	Domain        string `json:"domain"`
	CSP           CSP    `json:"csp"`
	PrefersBorder bool   `json:"prefersBorder"`
}
type CSP struct {
	ConnectDomains  []string `json:"connectDomains"`
	ResourceDomains []string `json:"resourceDomains"`
}
type DisplayModes struct {
	Available []string `json:"availableDisplayModes"`
	Preferred string   `json:"preferredDisplayMode"`
}
type SettingsCapability struct {
	ReadTool   string `json:"readTool"`
	UpdateTool string `json:"updateTool"`
}

type LegacyCSP struct {
	ConnectDomains  []string `json:"connect_domains"`
	ResourceDomains []string `json:"resource_domains"`
	RedirectDomains []string `json:"redirect_domains"`
}
