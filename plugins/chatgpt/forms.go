package chatgpt

import (
	"github.com/vadora/vadora/api/types/errs"
	"net/url"
	"strings"
)

// Form and FormOption mirror OpenAIForm and OpenAIFormOption.
type Form struct {
	Type                 string               `json:"type"`
	Properties           map[string]FormField `json:"properties"`
	Required             []string             `json:"required"`
	AdditionalProperties bool                 `json:"additionalProperties"`
}
type FormField struct {
	Type    string       `json:"type"`
	Title   string       `json:"title"`
	OneOf   []FormOption `json:"oneOf,omitempty"`
	Items   *FormItem    `json:"items,omitempty"`
	Minimum *int         `json:"minimum,omitempty"`
	Maximum *int         `json:"maximum,omitempty"`
}
type FormItem struct {
	Type  string       `json:"type"`
	OneOf []FormOption `json:"oneOf"`
}
type FormOption struct {
	Const     string     `json:"const"`
	Title     string     `json:"title"`
	Thumbnail *Thumbnail `json:"x-openai-thumbnail,omitempty"`
}
type Thumbnail struct {
	Src string `json:"src"`
}
type Choice struct {
	ID    string
	Title string
	Image string
}

func (a *Adapter) SelectionForm(name string, choices []Choice, multiple bool, host Capabilities) (*Form, string, error) {
	if !host.Forms {
		return nil, "Choose the products in the Vadora UI, then confirm publication in app.vadora.xyz.", nil
	}
	if len(choices) > 50 {
		return nil, "", errs.New(errs.InvalidRequest, "too many form choices")
	}
	if name != "listing_ids" {
		if name != "variant_id" {
			return nil, "", errs.New(errs.InvalidRequest, "unsupported form field")
		}
	}
	options := make([]FormOption, 0, len(choices))
	for _, choice := range choices {
		if !identifier.MatchString(choice.ID) {
			return nil, "", errs.New(errs.InvalidRequest, "invalid offered identifier")
		}
		option := FormOption{Const: choice.ID, Title: choice.Title}
		if a.AllowedImage(choice.Image) {
			option.Thumbnail = &Thumbnail{Src: choice.Image}
		}
		options = append(options, option)
	}
	field := FormField{Type: "string", Title: "Choose a variant", OneOf: options}
	if multiple {
		field = FormField{Type: "array", Title: "Choose products", Items: &FormItem{Type: "string", OneOf: options}}
	}
	return &Form{Type: "object", Properties: map[string]FormField{name: field}, Required: []string{name}}, "", nil
}

func ValidateSelection(offered []Choice, selected []string) error {
	if len(selected) == 0 {
		return errs.New(errs.InvalidRequest, "choose an offered item")
	}
	if len(selected) > len(offered) {
		return errs.New(errs.InvalidRequest, "too many selected items")
	}
	seen := make(map[string]bool)
	for _, id := range selected {
		if seen[id] {
			return errs.New(errs.InvalidRequest, "duplicate selected item")
		}
		seen[id] = true
		found := false
		for _, choice := range offered {
			if id == choice.ID {
				found = true
			}
		}
		if !found {
			return errs.New(errs.InvalidRequest, "selection was not offered")
		}
	}
	return nil
}

// AllowedImage uses exact declared domains and raster formats.
func (a *Adapter) AllowedImage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	if u.User != nil {
		return false
	}
	if u.Port() != "" {
		return false
	}
	allowed := u.Host == strings.TrimPrefix(a.config.AssetOrigin, "https://")
	for _, domain := range a.config.ImageDomains {
		if u.Host == domain {
			allowed = true
		}
	}
	if !allowed {
		return false
	}
	for _, suffix := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif"} {
		if strings.HasSuffix(strings.ToLower(u.Path), suffix) {
			return true
		}
	}
	return false
}
