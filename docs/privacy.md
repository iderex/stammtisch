# What this software holds about the people using it

An operator running this for a community is, in most jurisdictions, the party
responsible for the personal data it processes. This document is the material
they need to write their own policy: what is stored, why, for how long, what
never leaves the machine, and what deletion does and does not reach.

It is not legal advice and it is not a privacy policy for a service anybody
here runs. There is no such service. Entry 6 of #1 decided that this project
never operates a hosted instance, so every deployment of this software belongs
to somebody else and the obligations that follow are theirs.

Everything below describes the tree it ships in. Where it asserts a fact it
carries the command that produced it, so a reader can check the sentence rather
than trust it, and so a sentence that stops being true is one somebody can
catch.

## What is stored durably

One SQLite file, written by `internal/store/sqlite`, and nothing else. The
schema is applied by a numbered migration history and the version travels
inside the file, so the inventory below is the whole of what a running service
writes to disk.

Nothing in the list expires. There is no scheduled deletion, no retention timer
and no compaction, so a row lives until something removes it, and the Deletion
section below is where that gets uncomfortable. The Retention column says what
ends each row rather than repeating that sentence thirteen times.

| Item | What it holds | Why | Retention |
| --- | --- | --- | --- |
| `channel.id` | The channel's identifier, written `local@host` | A channel has to be nameable across a restart, and every other table refers to it by this | Lives as long as the channel row |
| `channel.space` | The identifier of the space the channel belongs to | A channel exists inside one space and the store answers "which channels are in this space" | Lives as long as the channel row |
| `channel.name` | The name a person gave the channel | It is what a member sees in the list | Lives as long as the channel row |
| `channel.always_on` | Whether the room survives the last person leaving | The always-on lifecycle is per channel and has to come back after a restart | Lives as long as the channel row |
| `membership.space` | A space identifier | Half of the pair that says who belongs to what | Lives as long as the membership row |
| `membership.member` | A person's or a bot's identifier | The other half. This is the column that says a named person is a member of a named space | Lives as long as the membership row |
| `permission_grant.principal` | The identifier of the person or bot the grant is about | A permission is answered per principal | Lives as long as the grant row |
| `permission_grant.space` | The space the grant applies in | Every grant has a subject and every subject sits in a space | Lives as long as the grant row |
| `permission_grant.channel` | The channel the grant applies to, or the empty string where the subject is the space itself | A grant on one channel is not a grant on the space | Lives as long as the grant row |
| `permission_grant.permission` | Which permission is granted, by name | The permission model answers from stored grants rather than from a role somebody typed | Lives as long as the grant row |
| `gain.listener` | The identifier of the person who set a volume | Per-person volume is one listener's setting | Lives as long as the gain row |
| `gain.speaker` | The identifier of the person the setting is about | A setting is about somebody. The pair records that one named person turned another named person up or down | Lives as long as the gain row |
| `gain.percent` | The setting, as a percentage of unity | It is the setting | Lives as long as the gain row |

Two of those deserve a second look before an operator writes their policy.

`gain.listener` and `gain.speaker` together are a statement about one person's
opinion of another. Nothing in the product shows it to the speaker, and it is
still a record of a personal relationship rather than a technical setting. A
row exists only where somebody moved the control away from the default, which
is why there is no default column and no row for a listener who never touched
it.

`channel.name` is free text a person typed. Most channel names are not personal
data. A channel named after a person is, and nothing in the software can tell
the two apart.

The inventory is not maintained by hand. A test opens a migrated database,
reads its schema back, and compares it to the table above in both directions,
so a column added without a line here reds the suite, and so does a line here
naming a column the schema does not have:

    go test -count=1 -run TestThePrivacyDocumentNamesEveryStoredColumn ./internal/store/sqlite

## What is held only while the service is running

None of this is written to the file and none of it survives a restart.

Sessions, in `internal/auth`. A session holds an owner and its timestamps, and
the table is keyed on a digest of the token rather than on the token, so a copy
of it is not a set of working tokens. A token is valid for fifteen minutes:

    git grep -n 'TokenLifetime = ' -- internal/auth/session.go
    internal/auth/session.go:51:const TokenLifetime = 15 * time.Minute

An expired session is refused on lookup and left in place rather than swept,
so its digest, owner and timestamps stay in memory until the process ends or
the owner revokes it. That is deliberate and the source says why: removing it
on lookup would make the answer depend on who asked first, and an owner listing
their own sessions is owed a true picture.

Occupancy, which is who is in which channel right now. The channel and room
model puts occupancy in memory on purpose: it is a fact about the present
moment, and a restart that resurrected it would be reporting people as present
who are not.

Accounts have no durable home yet. They reach the authentication path through
an interface with one method, and nothing in this tree implements it, so today
there is no account table and no stored account:

    git grep -n -A3 'type Accounts interface' -- internal/auth/authenticate.go
    internal/auth/authenticate.go:17:type Accounts interface {
    internal/auth/authenticate.go-18-	// Stored returns the stored credential for name, or ErrNoSuchAccount.
    internal/auth/authenticate.go-19-	Stored(name string) (string, error)
    internal/auth/authenticate.go-20-}

## What is never held

Conversation content. The media plane port carries no operation that decodes,
mixes, transcodes, records or reads a payload, which is a property of the
interface rather than a promise about the implementation behind it:

    git grep -n 'operation that decodes, mixes, transcodes, records, or reads a payload' -- docs/decisions/media-plane-port.md
    docs/decisions/media-plane-port.md:214:operation that decodes, mixes, transcodes, records, or reads a payload. That is

A credential in a form the server could return. What is stored for an account
is the output of a memory-hard function over the credential and a random salt,
written out with the parameters that produced it.

A recording. Nothing in this tree records, and this is the part of the document
most likely to be read as more than it says. The decision on entry 2 of #1 is
that recording will be present, off, and announced by an indicator the server
enforces rather than one a client is asked to draw. None of that exists yet.
Issue #149 owes the capability and the indicator together, and this section is
rewritten when it lands rather than before.

## What leaves the machine

No telemetry, in either direction and under no setting. Entry 3 of #1 decided
none at all, not even as an opt-in, and there is nothing to switch off. Nothing
in the tree opens an outbound connection:

    git grep -nE '(^|[^[:alnum:]_.])(http\.(Get|Post|Head|PostForm|NewRequest)|net\.Dial|tls\.Dial|websocket\.Dial|http\.Client\{)' -- '*.go' ':!*_test.go' ; echo "exit=$?"
    exit=1

Exit 1 is no match. The only network surface in the tree is the signalling
websocket the server accepts, in `internal/transport`, and a connection there
is one somebody made to the operator's machine.

That command is a read of the source and not a measurement of a running
process. It says nothing was written that phones home; it does not say a
deployment cannot be made to, by an operator's own reverse proxy or by a
dependency on a future day. Nothing here watches for that.

## Logs

An operator's logs are the most likely route for a personal communication to
leave the boundary this document describes, because logs are shipped somewhere
else by design and nobody reads them on the way past.

What stops that here is the shape of one package rather than a rule contributors
are asked to keep. `internal/logging` is the only place a log line is written,
and its surface takes a closed set of events and a closed set of fields. Every
field constructor takes an identifier, a number or a duration, and none of them
takes a string, so a caller holding a channel name, a display name or a decoded
payload has nothing to pass it to and the compiler refuses the call.

The field set a line can carry, which is the whole of it:

    git grep -n -A11 'var declaredKeys = ' -- internal/logging/logging.go

Those are identifiers, a protocol version, a count and a duration. What is not
there is the part worth reading twice: no name, no message, no reason, no free
text under any spelling. `channel.name`, which the inventory above lists as the
one column that can be a person's name, has no field to be written into.

An identifier is `local@host`, and `NewIdentifier` admits nothing else: no
space, no character that does not print, one separator, a bounded length. That
is what stops free text being turned into an identifier first and logged that
way. What it does not stop is a value that already has that shape, so a caller
who deliberately built an identifier out of somebody's words could still log
them. Nothing in the tree does, and nothing in the tree would refuse it.

That there is one surface rather than several is refused by a machine rather
than reviewed. The greppable invariants check carries a rule over everything
under `internal/`, with the one file that writes on its exempt line, so a
package that starts printing for itself reds the check:

    git grep -n -A2 'rule: logging-outside-the-log-surface' -- .github/workflows/invariants.yml

The suite next to the surface drives a session through signalling, auth and
orchestration, reads the log it produced, and refuses a field outside the
declared set:

    go test -count=1 -run TestAFullSessionCarriesNoFieldOutsideTheDeclaredSet ./internal/logging

Now the part this section must not be read as saying. Nothing calls that surface
yet, because there is no server to call it: the entry point still prints one
line and exits, and what wires a running service is the configuration model in
#66 and the endpoints in #69.

    git grep -l '"github.com/iderex/stammtisch/internal/logging"' -- '*.go' ':!internal/logging/*' ; echo "exit=$?"
    exit=1

Exit 1 is no match. So an operator running this today has no logs at all rather
than narrow ones, and the paragraphs above describe what a log line will be able
to carry when something writes one, not what anybody's log holds now.

## Deletion

The store exposes no deletion operation. Not for a person, not for a channel,
not for a grant. Its whole surface is nine methods on one interface, and none
of them removes anything:

    git grep -nE '^	[A-Z][A-Za-z]*\(' -- internal/store/store.go
    internal/store/store.go:60:	PutChannel(ctx context.Context, c *orchestration.Channel) error
    internal/store/store.go:62:	Channel(ctx context.Context, id orchestration.ID) (*orchestration.Channel, error)
    internal/store/store.go:67:	ChannelsInSpace(ctx context.Context, space orchestration.ID) ([]*orchestration.Channel, error)
    internal/store/store.go:71:	PutMember(ctx context.Context, space, member orchestration.ID) error
    internal/store/store.go:73:	Members(ctx context.Context, space orchestration.ID) ([]orchestration.ID, error)
    internal/store/store.go:79:	Grant(ctx context.Context, principal orchestration.ID, subject orchestration.Subject, granted ...orchestration.Permission) error
    internal/store/store.go:84:	SetGain(ctx context.Context, listener, speaker orchestration.ID, percent int) error
    internal/store/store.go:88:	Gain(ctx context.Context, listener, speaker orchestration.ID) (int, error)
    internal/store/store.go:92:	Close() error

So an operator asked to delete somebody's data cannot do it through the
software today, and that is the honest state of this section rather than a gap
in the writing.

What an operator can do instead. Stop the service and remove the database file,
which removes everything for everybody. Or operate on the file with the
`sqlite3` tool, which is deleting rows out from under a schema the software
believes it owns, and is a thing to do with a backup taken first and the
service stopped.

What deletion does not reach, whichever route is taken.

Backups. A copy of the database file taken before the deletion still holds
every row, and nothing in the software knows the copy exists.

The memory of a running process. Sessions and occupancy are held in memory, so
a person removed from the file is still connected until their session ends or
the service restarts.

Whatever a participant's own client kept. A third-party client is somebody
else's software on somebody else's machine.

Nothing else, and that is the one comfortable line in this section. There is no
second copy on anybody's infrastructure, because nothing leaves the machine.

When a deletion path lands it belongs with a store operation and a test, and
this section changes with it.

## What this document is not

It is not legal advice. It is not a compliance assessment and it does not
decide whether a given deployment is lawful, which depends on the jurisdiction,
the community and what the operator does beside running this software.

It is not a promise about a future version. Every sentence describes the tree
it ships in, and the commands above are how a reader tells whether it still
does.
