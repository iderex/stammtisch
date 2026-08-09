# The bot API shape, and what makes it first-class

Status: decided, 2026-08-09. Raised by issue #11.

## The decision

Three surfaces and no more. One authenticated event stream, one request surface,
one media attachment path. Scopes are granted per bot rather than per space. The
request surface carries its version in the path. Voice receive is a supported
capability of this interface and it is held under a right of its own.

First-class is a claim, so this record says what it costs in specific
capabilities rather than in adjectives. The three that follow are the ones the
commercial ecosystem either does not offer or offers by accident: voice receive
as documented behaviour, rate limits as published numbers with a test behind
them, and delivery semantics a bot author can act on instead of guess at.

## The event stream rides the signalling protocol

A bot opens the same framed connection a client opens, and receives the same
events narrowed to what it may see.

The permission model already treats it as the same kind of thing. A bot and a
person are one type, produced by two constructors, and the kind is unexported
with no accessor:

    git grep -n 'func Bot(id ID) Principal' -- internal/orchestration/permission.go
    internal/orchestration/permission.go:121:func Bot(id ID) Principal { return Principal{id: id, kind: kindBot} }

A second event path would be a second place deciding what a principal may see,
and one place deciding that is the property `internal/orchestration` is built
around. The connection also already agrees a version on its first frame, refuses
a payload larger than our own bound before allocating for it, and refuses a
request whose origin is another host. A bot on a transport of its own would owe
each of those a second implementation, and the second one is the one nobody
fuzzes.

Webhooks were the alternative worth taking seriously and they are refused. They
require the server to reach the bot, which for a self-hosted service puts the bot
author behind the same router problem #49 records for the operator, one level in.
A bot running on somebody's home machine is the normal case here rather than the
exception, and an interface that works only for bots with a public address is an
interface for hosted bots.

Server-Sent Events are refused for a smaller reason. They carry one direction, so
anything a bot sends needs a second channel, and the version exchange this
protocol makes its first frame has nowhere to happen.

## The request surface is HTTP, and it is separate on purpose

Requests that expect an answer go over HTTP with JSON bodies, not down the
stream.

A request over a stream needs correlation identifiers, per-request timeouts and a
retry rule, and each bot library then writes its own. Writing those four times in
four languages is the concrete thing that turns first-class into a word, because
the differences between the libraries become the interface a bot author actually
meets. HTTP carries all four already and every language has a client that does.

It is also what lets the rate limit contract be stated in a vocabulary bot
authors already hold, which is a status code meaning refused for rate and a
header saying how long to wait.

Two transports mean two paths a credential travels, and that cost is held to one
credential rather than argued away. The same bot token is presented in the
credential frame on the stream and in the authorisation header on the request
surface, carries the same scopes, and is revoked in one place. #51 owns the token
itself, its scopes and the bound within which a revocation takes effect.

## Media attaches to the media plane, never to the signalling connection

A bot that receives or sends audio attaches to the room the way any other
participant does, with a credential minted for one room and one member by #41.

Carrying audio down the signalling connection would mean the server terminating,
decoding and re-muxing it per bot. The design property the whole architecture
rests on is that the server never looks inside the payload, and
`docs/decisions/per-person-volume.md` makes exactly this argument against
applying gain at the server. The argument does not change because the listener is
a program.

Attaching through the media plane is also what gives #53 its first condition
without anything being built for it. A selective forwarding unit forwards each
publisher as its own stream, so a subscribing bot receives decoded audio per
speaker rather than a mix, because per speaker is what any subscriber receives.
That property comes from the engine decision in `docs/decisions/media-engine.md`
rather than from this record, and it is the thing to re-read if that decision
moves.

The cost is real and belongs in the open. A bot author needs a WebRTC stack,
which is heavier than an HTTP client and is the single largest obstacle between
somebody and a working bot. It is paid down by the client and bot libraries
carrying it rather than by pretending it is small, and those libraries are under
the permissive arm of entry 1 of #1 so that a bot author can actually take them.
Which paths sit under that arm is #142 and is not settled yet.

## Two version numbers, and why not one

The request surface carries its version as the first path segment. Entry 7 of #1
decides that the stability promise starts at a named later release, and that
until then the instability stands in the endpoint path rather than only in a line
of documentation, so that segment is the word `unstable` and becomes a number at
that release.

The event stream has no path. Its version is the protocol version agreed on the
first frame, by the exchange `internal/signalling` already implements, and a bot
reads the agreement rather than assuming its own proposal was taken whole:

    git grep -n 'KindVersionAgreed Kind' -- internal/signalling/negotiation.go
    internal/signalling/negotiation.go:48:	KindVersionAgreed Kind = 4

So a bot author holds two numbers. That is the cost and it is chosen. One number
covering both surfaces would make a change on either one a bump on the other, and
they move for unrelated reasons: the protocol version moves when the framing or
the message set moves, and the request surface moves when a resource does. Tying
them means a bot that touches neither has to be re-released.

Which release starts the promise is not named here, because entry 7 does not name
it. #55 writes the procedure the promise will refer to and #91 states how far
behind a client may be.

## Scopes attach to the bot

A token names one bot and one space. The scopes it carries are the bot's, not the
space's, so a bot cannot hold a wider scope in one space than in another and
adding it to a second space grants nothing it did not already have.

A scope never widens a permission. The permission model answers what a principal
may do at all, and the scope narrows what this token may do out of that answer.
There is no arrangement in which a scope reaches something the permission model
refused.

Per bot rather than per space, because a scope set per space is a matrix somebody
maintains, and its failure mode is a bot behaving differently in one place for a
reason nobody can see from either side. Where something is allowed here and not
there is already what permissions answer, against a subject, and duplicating that
into scopes gives one question two authorities.

## Voice receive, and what a bot must hold to use it

Voice receive is a supported capability: documented, versioned, and tested. This
is the gap the record is built around. In the commercial ecosystem the same
capability is undocumented behaviour that the libraries doing it warn about, and
every transcription bot, recording bot, accessibility tool and voice assistant
there stands on it.

Three things, all of them, and the first is not a formality:

- The permission a person needs to be in that channel, which is `JoinChannel` in
  `internal/orchestration/permission.go`, evaluated through the same function and
  the same `Principal` type a person goes through.
- A scope of its own for voice receive. It is never implied by a broader scope
  and never arrives as part of a general read scope. Entry 2 of #1 decides it in
  those terms: the bot interface may receive audio, and only under a right of its
  own.
- A media credential for that room and that member, minted by #41, expiring on
  the lifetime that issue states and refused once the member's occupancy has
  ended.

What the channel holds while a bot receives is not a detail either. Entry 2
decides that recording is present, off, and carries an indicator the server
enforces rather than the client, and that the recording state is carried in the
signalling protocol where every participant sees it. A bot receiving voice is
announced on that same mechanism: the people in the channel are told by the
server, and no client-visible setting suppresses it. #53 is where that is built
and proved, and its second condition is the proof.

Receiving and recording stay separate here. A transcription bot that writes
nothing to disk is still receiving personal communications, so it is announced. A
recording is a further thing and carries the indicator entry 2 describes on top of
the announcement.

## Delivery semantics of the event stream

Events on one connection arrive in the order the server applied them and each
arrives once. Order holds within a channel and is not promised across channels,
so two events about different channels may arrive in either order, and a bot
depending on that interleaving is depending on something this interface does not
offer. Nothing is buffered for a bot that is not connected, so a bot away for a
minute does not get that minute back. On a new connection the server sends the
state that bot may see as a snapshot and then resumes live events, which means a
bot has to be able to apply a snapshot at any moment and has to treat every event
as idempotent, because an event delivered before the break can also be reflected
in the snapshot that follows it. There is no replay and no cursor to rewind to.

One case in that area is deliberately not decided here. What happens to a bot too
slow to keep up is #52's to choose between buffering, dropping and disconnecting,
and it is a property of the server's back pressure rather than of the shape. It is
named rather than left out because it is the case that is always underspecified
and always the one that bites.

## The rate limit model, and no number in this record

A limit is a token bucket with a sustained rate and a burst. It is evaluated per
bot and per space, not per token, so minting a second token buys no second
budget. It covers the request surface and the actions a bot sends on the stream.
Events the server sends to a bot are not rate limited; back pressure in that
direction is the slow-consumer case above.

A refusal names the wait. On the request surface that is the status meaning too
many requests, carrying the number of seconds to wait. On the stream it is a
refusal frame carrying the same number. A refusal that does not say how long to
wait is the thing this interface is a reaction to, because in the commercial
ecosystem the number is discovered by hitting it and handling it correctly is
each library's private problem.

The numbers are published and they are tested. Published in the contract on #50,
as part of the document a client is written from rather than as a page beside it.
Tested by #51, whose third condition is that each limit is a published number,
that the refusal names the wait, and that a test drives the limit and asserts
both.

No number is fixed here, and that is deliberate rather than an omission. A rate
limit written into a decision record before anything has served a request is a
number with nothing behind it.

## What this record does not decide

- The messages, the fields, the errors and the limit numbers. That is the
  contract on #50, generated from the definitions the server compiles.
- The bot token's format, its scope vocabulary and its revocation bound, #51.
- The slow-consumer behaviour on the event stream, #52.
- When the stability promise starts, which entry 7 of #1 leaves to a named later
  release, and the procedure it refers to, #55.
- Which paths carry the permissive arm of entry 1 of #1, #142.

## When this is revisited

If the engine decision moves away from a selective forwarding unit, the sentence
about decoded audio per speaker stops following from anything and this record is
re-read rather than kept.

If `docs/decisions/client-platform.md` stops making the first client a browser
application, the transport argument for the event stream loses the constraint it
was made against, and the alternatives refused above are worth asking again.
