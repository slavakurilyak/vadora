package chatgpt

import (
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
