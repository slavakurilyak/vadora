package chatgpt

import (
	"os"
	"strings"
	"testing"

	contract "github.com/vadora/vadora/api/mcp"
)

// The adapter is the host's presentation for the server, so a compile time check keeps the
// two contracts in step.
var _ contract.Presentation = (*Adapter)(nil)

// ChatGPT sends its anonymized conversation identifier under openai/session on every tool
// call. The adapter reads only a short run of printable characters from that one key, and
// anything else counts as no session, so a client cannot smuggle a value into the server's
// rate limit tables (001:316).
func TestTheHostSessionIsOnlyAShortPrintableValueUnderTheOpenAIKey(t *testing.T) {
	adapter := testAdapter(t)
	cases := []struct {
		name string
		meta map[string]any
		want string
	}{
		{"a conversation identifier", map[string]any{"openai/session": "conv_a1B2-c3.D4"}, "conv_a1B2-c3.D4"},
		{"no metadata", nil, ""},
		{"another key", map[string]any{"openai/subject": "user_1", "session": "x"}, ""},
		{"an empty identifier", map[string]any{"openai/session": ""}, ""},
		{"a number", map[string]any{"openai/session": 42}, ""},
		{"an identifier with a space", map[string]any{"openai/session": "a b"}, ""},
		{"an identifier with a line break", map[string]any{"openai/session": "a\nb"}, ""},
		{"an identifier with a non ASCII letter", map[string]any{"openai/session": "café"}, ""},
		{"an identifier at the length bound", map[string]any{"openai/session": strings.Repeat("a", MaxHostSessionLength)}, strings.Repeat("a", MaxHostSessionLength)},
		{"an identifier past the length bound", map[string]any{"openai/session": strings.Repeat("a", MaxHostSessionLength+1)}, ""},
	}
	for _, c := range cases {
		if got := adapter.HostSession(c.meta); got != c.want {
			t.Errorf("%s: HostSession = %q, want %q", c.name, got, c.want)
		}
	}
}

// The entrypoint icon follows the published guidelines: a monochrome SVG with a transparent background that
// takes the text color through currentColor, a 20 by 20 viewport, and 1.33 pixel strokes, and each entrypoint
// tool carries it (011:66).
func TestTheEntrypointIconsAreMonochromeTwentyByTwentySVGsThatFollowTheTextColor(t *testing.T) {
	adapter := testAdapter(t)
	for _, tool := range []string{"open_marketplace", "open_cart"} {
		icons := adapter.Icons(tool)
		if len(icons) != 1 {
			t.Fatalf("%s carries one icon, got %d", tool, len(icons))
		}
		if icons[0].MIMEType != "image/svg+xml" {
			t.Fatalf("%s carries a 20 by 20 SVG, got %+v", tool, icons[0])
		}
		if len(icons[0].Sizes) != 1 {
			t.Fatalf("%s carries a 20 by 20 SVG, got %+v", tool, icons[0])
		}
		if icons[0].Sizes[0] != "20x20" {
			t.Fatalf("%s carries a 20 by 20 SVG, got %+v", tool, icons[0])
		}
	}
	if icons := adapter.Icons("search_products"); len(icons) != 0 {
		t.Fatalf("only an entrypoint tool carries an icon, got %v", icons)
	}
	raw, err := os.ReadFile("assets/cart.svg")
	if err != nil {
		t.Fatal(err)
	}
	svg := string(raw)
	for _, want := range []string{`viewBox="0 0 20 20"`, `stroke="currentColor"`, `stroke-width="1.33"`, `fill="none"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("the icon says %s", want)
		}
	}
	for _, forbidden := range []string{"#", "rgb(", "<image", "<script", "<style", "gradient", "opacity"} {
		if strings.Contains(strings.ToLower(svg), forbidden) {
			t.Errorf("a monochrome icon holds no %q", forbidden)
		}
	}
}
