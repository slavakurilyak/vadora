package chatgpt

// HostSessionKey is the request metadata key under which ChatGPT sends its anonymized
// conversation identifier with a tool call, "for correlating tool calls within the same
// ChatGPT session" (OpenAI Apps SDK reference, the _meta of a tool call, read on 2026-10-10).
const HostSessionKey = "openai/session"

// MaxHostSessionLength bounds an identifier that the server accepts as a host session.
const MaxHostSessionLength = 128

// HostSession returns the conversation identifier that ChatGPT supplies with a tool call,
// and an empty text when the call carries none, or one that is not a short run of printable
// characters. The server uses it only to bound anonymous catalog calls. Any client can send
// any value, so it never identifies a person and never authorizes a call.
func (a *Adapter) HostSession(meta map[string]any) string {
	value, ok := meta[HostSessionKey].(string)
	if !ok {
		return ""
	}
	if value == "" {
		return ""
	}
	if len(value) > MaxHostSessionLength {
		return ""
	}
	for _, character := range value {
		if character <= ' ' {
			return ""
		}
		if character >= 0x7f {
			return ""
		}
	}
	return value
}
