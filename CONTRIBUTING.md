# Contributing

Thank you for helping with the Vadora clients. This repository holds the clients and the shared contract. The API server lives in a private repository and is not open for changes here.

## Rules

1. A change to behavior starts from the specification that the maintainers publish for it. Open an issue first when no specification covers your change.
2. Add or update a test for every requirement that your change touches. A test names the behavior that it proves, and a test of a rule is checked against a version of the code that breaks the rule.
3. Run these commands from the repository root before you open a pull request, and keep them green: `gofmt -l .`, `go build ./...`, `go vet ./...`, and `go test ./...`.
4. Keep every client free of secrets, signing code that runs on the server, and business logic. A client reaches the server only through its public API, and the server decides every request.
5. Put no credential, key, token, or delivery detail in a skill, a manifest, a fixture, a test, a log line, or a pull request. A secret scan runs on every change.
6. Do not import a private package, name a private path, or name a private host. The import boundary tests fail such a change.
7. Treat a change to a tool schema, an OpenAPI operation, or a UI resource URI as a versioned contract change. Say so in the pull request.
8. Write in full sentences with commas and periods, and use numbered lists instead of dash bullets.

## License of contributions

By contributing you agree that your contribution is licensed under the Apache License, Version 2.0, as in the LICENSE file. See CLIENT_POLICY.md for the rules about the Vadora name and marks.

## Reporting a security problem

Do not open a public issue for a security problem. Write to the security address on vadora.xyz and describe what you found, how to reproduce it, and which versions it affects.
