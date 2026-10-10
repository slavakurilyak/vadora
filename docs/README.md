# Developer documentation

This directory holds the public developer documentation for the Vadora clients. The published contract lives in code: the OpenAPI document in `api/openapi`, the MCP tool schemas in `api/mcp`, and the shared types in `api/types`.

## How a client reaches Vadora

1. A buyer API call carries a buyer credential in the Authorization header. A person gets a credential by signing in with an emailed code on the checkout page or through `vadora login --buyer`. The server stores only a hash of it.
2. A merchant or plugin call carries an OAuth access token from the host's sign in and the backend's OAuth with PKCE flow. The CLI uses the same OAuth server through `vadora login`.
3. Every request is decided on the server. A request carries no price, no total, no network class, and no status, because the server computes and fixes them.
4. Every error carries a code from the error catalog, a reason, and one client action, on REST, in MCP tool results, and in the CLI.

## The API

1. `GET /openapi.json` on the API origin serves the OpenAPI document, and `api/openapi` in this repository is its source.
2. A purchase without a payment payload returns HTTP 402 with the PAYMENT-REQUIRED header. A payment goes in the PAYMENT-SIGNATURE header, and the outcome comes back in the PAYMENT-RESPONSE header. Only version 2 headers are accepted.
3. A purchase carries an Idempotency-Key, and a retry returns the existing outcome.

## The CLI

Run `vadora help` for the command list. A buyer agent searches with `vadora products search`, asks for a quote with `vadora quote`, and pays with `vadora buy`. The buy command needs a buyer credential, a signing interface that you control, an owner authorized spending policy, and a maximum amount. It signs nothing outside that policy. The server applies its own rules to every request whatever the CLI does.

## The SDK

The `sdk` package is the API client. It checks what the server would refuse before it sends a request, and it never sends a price, a total, or a status. `sdk/install` writes a plugin package for an enabled host and nothing that runs a command.

## Plugins and skills

Everything inside a plugin package is public. `plugins/chatgpt` holds the ChatGPT package and the browser UI of its extension. `skills` holds the host independent skills, each of which is instruction text for the model and never authority.

## Fixtures

`fixtures` holds the labeled test store descriptions. A test store is always labeled TEST, and its results never count as revenue or as a live customer.
