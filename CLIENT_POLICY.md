# Client policy

This policy governs the license of this repository, the use of the Vadora name and marks by forks and third party clients, and the naming of unofficial clients. It grants no trust.

## The license

This repository is licensed under the Apache License, Version 2.0. The LICENSE file at the root holds the full text. You may use, copy, modify, and distribute the code under that license, in an open source client or a closed source client.

## The Vadora name and marks

1. The license grants no right to use the Vadora name, the Vadora logo, or any other Vadora mark. The one exception is a truthful statement of where your code came from, such as "based on the Vadora clients repository".
2. A fork, a rebuild, or a third party client uses its own name. It does not begin its name with Vadora, it does not name its binary vadora, and it does not use a name that a person could mistake for Vadora's own client.
3. A client never claims to be an official Vadora client, an official Vadora build, or an approved partner unless Vadora has said so in writing.
4. A plugin package that you publish for an agent host uses its own display name and its own description, and it does not imply that Vadora made it or endorses it.

## Unofficial clients

1. Say plainly that your client is unofficial and name its maintainer, in its README, its help text, and its listing.
2. Send the client's own name and version in the User-Agent header if you like. Vadora uses it for diagnostics only.
3. Do not copy the Vadora logo into your user interface, and do not present your client's pages as Vadora's checkout page or workspace.

## This policy grants no trust

The Vadora API server decides every request on its own evidence: it authenticates the caller, authorizes the request, validates every input, and computes every price, total, and status itself. It never relaxes a limit, never grants a capability, and never skips a check because a client claims a name, a version, an official build, or a signature. A fork, a modified client, and a client that skips its own spending limit work only within the permissions of their own credentials, and the server applies every rule to them.

This policy is never a substitute for that verification. It states what you may and may not call your client. The server decides what a client may do.

## Questions

Open an issue in this repository, or write to the support address on vadora.xyz.
