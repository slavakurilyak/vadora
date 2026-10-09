# Vadora clients

This repository holds every Vadora client and the shared contract that clients and
the server build from. It builds the `vadora` CLI without the `serve` command.

The API server, the hosted workspace, the checkout page server, every adapter and
connector, the database migrations, the deployment configuration, and the operator
commands live in the private repository, which imports this module at a pinned
version.

Everything inside a plugin package is public. A client holds no credential, no
signing code, and no server logic, and it grants no trust: the API server verifies
every request on its own evidence, whatever a client claims to be.

## Layout

1. `cmd/vadora` holds the entry point of the public CLI.
2. `cli` holds the exported command tree, so the private repository can add its
   own commands to the same tree.
3. `sdk` holds the API client and the plugin installer.
4. `api/types` holds the shared request and response types, `api/openapi` the
   OpenAPI document, and `api/mcp` the MCP tool schemas.
5. The five adapter registries live in the private repository, because the server loads them.

## Building

```
go build ./...
go vet ./...
go test ./...
```

A local development API origin is accepted over plain HTTP for loopback addresses
only. Every other origin must use HTTPS.
