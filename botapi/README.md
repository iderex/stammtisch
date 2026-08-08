# botapi

The bot API surface, which is a public contract third parties write against. It
sits outside `internal/` for that reason, and it holds the contract and nothing
else: no server state, no transport, no orchestration.

The contract itself is issue #50 and is not in this tree yet.
