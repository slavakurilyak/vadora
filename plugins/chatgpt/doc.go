// Package chatgpt provides presentation for Vadora's hosted tools. It verifies
// OpenAI MCP Extensions and official plugin documentation on October 9, 2026,
// MCP 2026-07-28 with legacy 2025-11-25, and MCP Apps 2026-01-26.
//
// Sources: https://developers.openai.com/plugins/reference,
// https://developers.openai.com/plugins/build/plugins,
// https://developers.openai.com/plugins/build/auth,
// https://developers.openai.com/plugins/build/extensions,
// https://github.com/openai/mcp-extensions/blob/main/docs/spec.md, and
// https://developers.openai.com/apps-sdk/reference.
//
// The Apps SDK reference, read on October 10, 2026, says that openExternal opens a vetted link in the person's browser and that
// ChatGPT appends ?redirectUrl=... to it for an approved redirect domain unless the call sets redirectUrl to false. The widget
// calls openExternal with the link alone, so the checkout page receives that address and reads it as the way back to the
// conversation, and the page accepts it only for an allowlisted agent host.
//
// Directory status is not submitted. No host acceptance run is claimed.
package chatgpt
