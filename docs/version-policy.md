# The version policy and the upgrade path

Nothing has been released from this repository yet:

    git tag
    (no output)
    gh api repos/iderex/stammtisch/releases --jq 'length'
    0

That is the reason to write this now. An operator who upgrades finds out what a
version number meant either because it was decided or because they discovered
it, and the second way costs them their data or their clients. The first release
is issue #92 and it is the release this document is written ahead of.

Three numbers move in this project and only one of them is the release number.
The other two are in the code, they are negotiated or stamped rather than
announced, and most of what an upgrade does to an operator is decided by them.
This document says what each one means, which of them forces which part of the
release number to move, and what happens to a store and to a client across the
boundary.

## The release number

`major.minor.patch`, and the components are ordinary. What is not ordinary
enough to leave unstated is which changes land in which component, because that
is the whole content of a version scheme and it is where a project quietly
redefines its own promise one release at a time.

A patch release fixes behaviour that was already promised. It adds no setting,
adds no protocol version, removes nothing, and carries no change to the store
schema. An operator upgrades within a patch line by replacing the binary.

A minor release adds. A new capability, a new optional setting with a default
that preserves the previous behaviour, a new protocol version alongside the ones
already accepted, or a store migration that only adds. An operator upgrades by
replacing the binary, and the upgrade is one way for the store as soon as a
migration runs, which is the section below.

A major release is the one an operator or a client author has to act on before
upgrading rather than after. A protocol version leaving the accepted range, a
setting removed or narrowed, a default that changes who can reach what, or a
migration that drops or narrows something a previous version wrote.

The meaning of the components does not change at 1.0. A scheme that exempts the
period before 1.0 exempts exactly the period in which most breaking changes
happen, and an operator running an early release is the one least able to absorb
a surprise. What number the first release carries is issue #92's to pick.

Which release lines receive security fixes, and for how long, is stated in
`SECURITY.md` and not here, so that the two documents cannot drift apart while
both look authoritative.

## The store schema version, and the direction that is not supported

The store carries its own integer, and it is nothing to do with the release
number. It is a four-byte value in the SQLite header, written as the last
statement of the transaction that applies each migration, so a schema and its
stamp cannot disagree:

    git grep -n 'PRAGMA user_version = %d' -- internal/store/sqlite/migrate.go
    internal/store/sqlite/migrate.go:153:	statements = append(statements, fmt.Sprintf(`PRAGMA user_version = %d`, m.version))

The history is a numbered append-only list, and it runs from 1 without a gap or
a repeat. The tree holds one migration today:

    git grep -nE '^[[:space:]]+version: [0-9]+' -- internal/store/sqlite/migrate.go
    internal/store/sqlite/migrate.go:34:		version: 1,

Migrations are applied when the store is opened, which is at startup, so an
upgrade is: stop the service, replace the binary, start it. There is no separate
migration command to forget to run.

The direction that is not supported is downward. A build opening a store written
by a later build refuses it and does not serve:

    git grep -n 'the data is at schema' -- internal/store/sqlite/migrate.go
    internal/store/sqlite/migrate.go:119:		return fmt.Errorf("%w: the data is at schema %d and this build knows %d", store.ErrNewerSchema, current, latest)

The refusal is the supported behaviour and not a limitation to be worked around.
An older binary that carried on would write rows the newer schema's constraints
were meant to refuse, and the corruption would be found later by the newer
binary with no way back to the state before it.

Both halves are proved by the suite rather than asserted here:

    go test -count=1 -run 'TestEveryMigrationAppliedFromEmptyProducesTheSchema|TestADowngradeIsRefusedRatherThanSilentlyCorrupting' -v ./internal/store/sqlite
    === RUN   TestEveryMigrationAppliedFromEmptyProducesTheSchema
    --- PASS: TestEveryMigrationAppliedFromEmptyProducesTheSchema (0.02s)
    === RUN   TestADowngradeIsRefusedRatherThanSilentlyCorrupting
    --- PASS: TestADowngradeIsRefusedRatherThanSilentlyCorrupting (0.05s)
    PASS
    ok  	github.com/iderex/stammtisch/internal/store/sqlite	4.899s

So the way back from an upgrade is not a downgrade. It is a restore of the store
as it was before the newer binary opened it, and taking a copy first is the
operator's whole protection against a release they want to undo. What that copy
is taken with, and what proves it can be restored, is issue #70.

One thing this section cannot claim. Migration across the boundary between two
released versions has not been exercised, because there has been one release
fewer than that needs. What is proved is the mechanism and the refused
direction, on the history the tree holds.

## How far behind a client may be

The protocol version is negotiated per connection and is not the release number
either. A client proposes one, and a proposal outside the range this build
speaks is refused with the range in the refusal rather than with a bare
rejection.

The range is held in two constants, and this document names them rather than
repeating the numbers, because a number written here goes wrong on the day they
move and says nothing while it is wrong:

    git grep -n 'MinSupportedVersion Version\|MaxSupportedVersion Version' -- internal/signalling/negotiation.go
    internal/signalling/negotiation.go:35:	MinSupportedVersion Version = 1
    internal/signalling/negotiation.go:36:	MaxSupportedVersion Version = 1

Read them at the release you are running. They are equal today, so the window is
one version wide and there is nothing for a client to be behind by yet. That is
a fact about a project with one protocol version rather than a policy, and the
policy is what governs the two constants once they can differ.

A protocol version is added in a minor release and is accepted alongside the
ones already in the range. It leaves the range only in a major release, and
never in the release that introduces its successor. So a client author always
has at least one release line in which both the version their client speaks and
the version it is moving to are accepted, and upgrading a server never requires
every client to be updated in the same window.

The refusal is a mechanism rather than a sentence in this document, and it is
proved from both ends of the range:

    go test -count=1 -run 'TestAClientOlderThanTheServerIsRefused|TestAClientNewerThanTheServerIsRefused|TestTheRefusalNamesTheVersionsTheServerSupports' -v ./internal/signalling
    === RUN   TestAClientOlderThanTheServerIsRefused
    --- PASS: TestAClientOlderThanTheServerIsRefused (0.00s)
    === RUN   TestAClientNewerThanTheServerIsRefused
    --- PASS: TestAClientNewerThanTheServerIsRefused (0.00s)
    === RUN   TestTheRefusalNamesTheVersionsTheServerSupports
    --- PASS: TestTheRefusalNamesTheVersionsTheServerSupports (0.00s)
    PASS
    ok  	github.com/iderex/stammtisch/internal/signalling	1.461s

A client that is refused is told the range it missed, so the operator's report is
"this server speaks protocol versions x to y" rather than a connection that
closed.

## Configuration files across an upgrade

This document does not state how long a configuration file stays valid, and the
reason is that there is no configuration model to state it about. That is issue
#66, and the line is left open rather than filled with a promise nothing could
keep. When it lands, what belongs here is which changes to the file are a minor
release and which are a major one, and what a file from the previous version
does when the new build reads it: load, or fail naming what changed.

Nothing in the sections above depends on that gap being filled, and none of them
should be read as covering it.

## What this document does not settle

What the first release is numbered, which is issue #92.

What an operator does to take and restore the copy the store section relies on,
which is issue #70.

Which release lines get security fixes and for how long, which is `SECURITY.md`.
