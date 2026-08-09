# botapi

The bot API surface, which is a public contract third parties write against. It
sits outside `internal/` for that reason, and it holds the contract and nothing
else: no server state, no transport, no orchestration.

The contract itself is issue #50 and is not in this tree yet.

Everything under this directory is licensed Apache-2.0, and the terms are in
[LICENSE](LICENSE) beside this file. The rest of the repository, which is the
server, is AGPL-3.0-or-later under the [LICENSE](../LICENSE) at the root. Taking
the contract and writing a bot against it therefore does not put your bot under
the server's terms, which is the reason the two arms exist.

Which paths sit under which arm is a list in
[.github/check-licence-headers.sh](../.github/check-licence-headers.sh) rather
than a sentence here. That check refuses a file carrying the identifier of the
other arm, in both directions, so this paragraph cannot quietly stop being true.
