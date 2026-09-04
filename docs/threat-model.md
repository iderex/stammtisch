# Threat model

Raised by issue #82. Written at `89933e0`, against the tree at that commit.
The section for somebody on the network path, and that attacker's entry in
the admitted gaps, were rewritten at `979451f`, when the control they had
been waiting for landed. The structural half of the first attacker was
corrected at `080790b`, when the mechanism it named was removed. Every other
section reads as it did.

This model is about a service that carries personal communications on hardware
an operator controls. It names what is worth taking, where the lines are that an
attacker has to cross, who the attackers are, and for each of them either the
control that stops them or the fact that nothing does.

Read the last section first if you are deciding whether to run this software.
Most of what is written here is a statement about a design, because most of the
software does not exist yet, and a model that reads as a report on a running
service would be the more useful document and the false one.

## What state the tree is in, because it changes what every sentence means

Nothing in this repository terminates a connection today. There is a framing, a
version negotiation, a WebSocket binding, a credential and a session, an
orchestration domain and a permission function, and an entry point that prints
one line and exits:

    git grep -n 'nothing is implemented yet' -- main.go
    main.go:18:	if _, err := fmt.Fprintln(os.Stdout, "stammtisch: nothing is implemented yet"); err != nil {

There is no media plane. `internal/media` holds a package comment and no
interface:

    git ls-files internal/media
    internal/media/doc.go

So the controls below split into two kinds, and the split is marked at every
one. A control that is in the tree names the test that proves it. A control that
is a decision and not yet code says so, and names the issue that owes it. No
control here is asserted on the strength of a record alone without that being
visible in the same sentence.

## Assets

The conversation. The audio and video of people talking. This is the asset
the product exists for and the one whose loss is not recoverable by any later
action.

Who was talking to whom, and when. Occupancy is a fact about the present and
is deliberately narrow, but a stream of occupancy changes over time is a social
graph with timestamps on it, and it is readable by everybody who can see the
channel. `docs/decisions/presence-model.md` decides what an occupancy record
carries and, at more length, what it does not.

Credentials and sessions. What lets somebody be a particular person.

The space's structure. Channels, permissions and memberships. Losing control
of these is losing control of who may hear what, which reaches the first asset
through the front door rather than around it.

Availability. A community that cannot get into a voice channel has lost the
service, and for a self-hosted deployment there is nobody else to fail over to.

A recording, once one can exist. Entry 2 of issue #1 decided that recording
is present, off, and cannot be enabled without an indicator the server enforces.
Nothing in the tree implements any of that, and issue #149 is where it is built.
Until then a stored recording is not an asset this software has, and this model
says what changes when it does rather than pretending the answer is already
known.

## Trust boundaries

The unauthenticated peer and the server. An anonymous peer reaches the
WebSocket handshake and then the framing, and nothing else, until it has
presented a credential. This is the outermost line and the one an attacker
reaches without any cooperation from anybody.

The authenticated principal and the permission function. Every question
about what somebody may do crosses `Allow` in
`internal/orchestration/permission.go` and crosses it nowhere else.

The orchestration layer and the media plane. The port is a language-native
interface, specified in `docs/decisions/media-plane-port.md`, and it cannot
express an operation that reads a payload. That is a boundary in the sense that
matters here: what is on the far side of it is bytes the server never opens.

The server and the media unit. The unit is LiveKit, per
`docs/decisions/media-engine.md`, running in the operator's own deployment. It
holds the media path and this project does not own that code.

The server process and durable storage. Not yet a boundary in the tree,
because there is no store. Issue #27 is where it becomes one.

The person and their client. A client runs on the person's machine, under
their control, and can be replaced. Every rule that lives only in a client is a
rule on the far side of this boundary and is therefore a preference and not a
control.

The operator and everybody in the space. The operator holds the machine.
This is the boundary that cannot be moved by any amount of engineering, and the
section on the operator says what that means honestly.

## The attackers

### A participant who wants to hear a channel they cannot join

Controlled, and the control is in the tree. `Allow` refuses every channel
permission to a principal that does not hold `SeeChannel` for that channel, and
a channel a viewer cannot see is not disclosed at all rather than being
disclosed as forbidden. Occupancy fan-out is bounded by the same answer.

    go test ./internal/orchestration -run 'TestAPrincipalWhoCannotSeeAChannelCanDoNothingInIt|TestAChannelAViewerCannotSeeIsNotDisclosedAtAll|TestThePermissionMatrix|TestTheMatrixCoversEveryPermissionTheModelDeclares' -count=1

The structural half matters as much as the answer. Nothing outside `Allow`
decides a permission, and that is refused rather than reviewed:
`TestOnlyOnePlaceDecidesAPermission` parses every Go file in the package except
the model and its own, and refuses a call to `Granted` or to `has` and any
mention of the principal kind. A declaration of `Granted` is left alone, which
is how a test supplies grants, and telling that from a call is why this is a
syntax tree rather than a pattern.

This paragraph said `Greppable invariants` refused the same vocabulary by path
prefix and so already covered packages nobody has written. It does not, and
the claim was never worth as much as it sounded: the pattern rule named
`internal/orchestration/`, which is the package the guard above already
judges. #76 took the rule out rather than keep two mechanisms for one
property, and the reach a pattern rule buys is held for logging alone. So a
permission decided in a package nobody has written yet is refused by nothing
until that package is in the guard's own reach.

What is not controlled. Getting into the room is one half of hearing it. The
other half is being subscribed to the tracks, which is admission and credential
minting, and that is issue #41 and does not exist. A permission answer that the
media plane does not enforce is a comment. The model does not claim this
attacker is stopped once there is a media plane; it claims the decision surface
that would stop them is correct and single.

### A participant who wants to keep speaking after being muted

Not controlled. There is no mute. Issue #39 owes the server-side mute, and
`docs/decisions/per-person-volume.md` says in its own words that per-person
volume is a comfort control and never a moderation one, sending anything that
has to hold to #39.

The shape of the eventual control is already decided and is worth holding to,
because the wrong shape looks identical from the interface. A mute has to remove
the subscription so the bytes never reach the listener. A mute implemented as a
gain applied somewhere is a mute a modified client ignores. Issue #42's third
clause and issue #39 are the same property from two ends.

### Somebody on the network path

Controlled on the signalling leg, in the tree. Required rather than controlled
on the media leg. `docs/decisions/transport-confidentiality.md` decides the two
separately, because a proxy in front of the signalling connection says nothing
about media and collapsing them is how one gets read as an answer about the
other.

The signalling connection is confidential, and exactly two arrangements satisfy
that: this process terminates TLS itself, or something in front of it does and
forwards over a link the operator controls. Which one applies is an argument to
the function that produces a signalling connection, and the value that refuses
is the zero value, so a caller that says nothing about its deployment gets the
refusal:

    grep -n 'TLSHere Transit = iota' internal/transport/confidentiality.go

The refusal is proved by standing up the refused arrangement rather than by
asserting the happy path, and a `Transit` no constant names refuses as well
rather than being taken for a guarantee:

    go test ./internal/transport -run 'TestARequestThatDidNotArriveOverTLSIsRefused|TestTheRefusalOfANonConfidentialRequestIsAnswered|TestATransitValueNobodyHandledRefuses' -count=1

A refused request is answered 403 and no connection is made for it.

The media path is not this project's byte path, which
`docs/decisions/media-engine.md` records. RFC 8827 requires DTLS-SRTP and
defines no unprotected arm, so that leg is settled by the standard rather than
chosen here and an operator cannot turn it off from this side. The connection
between this process and the unit's own control interface is different: it
carries room admission and the credentials minted for it, it has to be TLS, and
nothing in this tree carries it because there is no adapter. Issue #40 is the
adapter and issue #41 is the admission that would travel over it.

Not controlled: any of this in a running service. Nothing here serves that
handler, the entry point prints one sentence and exits, and the only callers are
the suite:

    git grep -l 'transport\.Handler(' -- '*.go'
    internal/transport/confidentiality_test.go
    internal/transport/websocket_test.go

So what is in the tree binds the first process to serve the handler and refuses
nothing today. Issue #69 is the startup self-check, and it is where a deployment
that does not meet the record is made to say so at the moment an operator would
see it.

Not controlled: the forwarded arrangement itself. Nothing in this process can
tell a request handed over by a proxy on the same host from one that crossed a
network in the clear before it arrived, so that arrangement is the operator's
word and is trusted deliberately, because the alternative is refusing the
deployment shape almost every self-hosted service has. What the value buys is
that the trust is typed at a call site under a name that says what is being
claimed, rather than being the absence of a setting nobody looked at. Issue #66
is where it stops being a call-site argument and becomes a key an operator sets,
with the same refusing default.

Two things that are true today and are not a substitute for either half above.
The WebSocket binding refuses a request whose `Origin` names another host, so a
page on somebody else's host carrying a visitor's cookies cannot open a
connection: `TestARequestFromAnotherOriginIsRefused`, which goes red if
`InsecureSkipVerify` or an `OriginPatterns` entry is added. And the stored
credential is a memory-hard digest with its parameters written beside it, so a
network attacker who captures a login does not capture something the server
could have handed back. Neither of those is confidentiality in transit and
neither should be read as it.

### A malicious bot holding a legitimate token

Controlled at the permission surface, in the tree. A bot is a principal and
is evaluated by the same function as a person, with no branch anywhere on which
kind of principal it is:

    go test ./internal/orchestration -run 'TestABotIsEvaluatedByTheSameFunctionAsAPerson|TestOnlyOnePlaceDecidesAPermission' -count=1

`docs/decisions/bot-api-shape.md` adds the rule that a scope never widens a
permission: the permission model answers what a principal may do at all and a
scope narrows what a token may do out of that answer, so there is no arrangement
in which a scope reaches past `Allow`.

Not controlled: everything about the token itself. Issue #51 owes bot
identity, scopes and rate limits, and the bound within which a revocation takes
effect. A bot that is inside its permissions and outside any rate limit is an
availability problem rather than a confidentiality one, and issue #52's
slow-consumer clause is the other half of it.

Not controlled: what a bot does with audio it may legitimately receive. Voice
receive is a supported capability under a right of its own, decided in entry 2
of issue #1 and owed by issue #53. A bot that holds that right and writes what it
hears to disk is a recorder, and the control against that is the indicator issue
#149 owes and not the permission model. `docs/decisions/media-plane-port.md`
already names this as the only way a recording could be accounted for at all: a
recorder is a subscriber carrying an identity, a permission set and a credential,
rather than an operation on the port.

### A hostile client that ignores every client-side rule

Assumed, rather than defended against, and that assumption is already load
bearing. `docs/decisions/per-person-volume.md` says a third-party client can
ignore the volume setting entirely and that the server has no way to know. The
architecture leans on this being stated: anything that has to hold against a
client that does not cooperate is a server-side mechanism by construction.

The property that makes this containable is that the server decides what a
client receives rather than what it displays. A client cannot render audio it was
never subscribed to. So every rule this model relies on has to be a rule about
subscription and permission, and none of them may be a rule about rendering.

Partly controlled, in the tree, for the part a hostile peer reaches first.
The framing bound is compared against the header before anything is allocated
for it, over a pipe and through the real transport:

    go test ./internal/signalling ./internal/transport -run 'TestDecodeRefusesAnOversizedFrameWithoutAllocatingForIt|TestPropertyDecodeNeverAllocatesBeyondTheBound|TestTheFrameBoundIsRefusedThroughTheTransport' -count=1

An unauthenticated connection accepts the authentication frame and nothing else,
and a refusal is terminal rather than per frame, so a peer cannot sit on a
connection trying message kinds:

    go test ./internal/signalling ./internal/transport -run 'TestAnUnauthenticatedConnectionAcceptsNothingElse|TestARefusalIsTerminalRatherThanPerFrame|TestAPeerThatHasProvedNothingIsRefusedThroughTheTransport' -count=1

The decoder is driven by arbitrary bytes as a property rather than by examples,
and `.github/workflows/fuzz.yml` runs `FuzzDecode` and the credential parse on a
schedule.

Not controlled: how much a peer may do rather than how large one message may
be. There is no connection rate limit, no per-peer accounting and no cap on
connections. Issue #94 measures where the service falls over and issue #52
decides what happens to a consumer that cannot keep up. Neither is a limiter, and
this model does not have one to name.

### The operator

Not controlled, and it cannot be. The operator holds the machine, the process
and the memory in it. An operator who wants the audio can have the audio, and no
property of this software changes that. Saying otherwise would be the most
damaging sentence this document could contain, because it is the sentence a
community would be shown.

What the software can do is make the difference between an operator who takes
something and an operator who takes it silently. That is the whole content of
entry 2 of issue #1: recording is present, is off, and cannot be enabled without
an indicator the server enforces against every client including third-party
ones. Today none of it exists, so a participant cannot tell whether a recording
capability is running, because the answer is that nothing does. On the day it
does exist, that is issue #149's to enforce, and if it is built with the
indicator in the client rather than in the server then this attacker is
uncontrolled again and the sovereignty statement in issue #80 has to say so.

Three smaller things reduce what an operator collects by accident rather than
what they can take deliberately, and they are worth separating from the
paragraph above. Occupancy carries no audio level, no speaking state, no device,
no client version and no network address, and `docs/decisions/presence-model.md`
argues each omission. Issue #67 owes a log surface that carries no conversation
content, and `Greppable invariants` already refuses logging outside that surface
before the surface exists. Entry 3 of issue #1 decided that there is no
telemetry, not even opt-in, so nothing leaves the operator's machine to us.

None of those three is a control against a hostile operator. They are controls
against an ordinary one ending up holding more than they meant to, which is the
more common case and not the one this section is about.

## The admitted gaps, and the issue each one has

Issue #82's second clause asks that every admitted gap have an issue. These are
the gaps this model admits, in the order they appear above.

- Room admission and track subscription do not enforce the permission answer.
  Issue #41.
- There is no server-side mute. Issue #39.
- Nothing serves the signalling handler, so the refusal of a connection that
  is not confidential binds the first process to serve it and refuses nothing
  today. Issue #69 is the startup self-check that would report which
  arrangement a deployment is in, and issue #66 is the configuration key it
  would be read from.
- A bot token has no scopes, no rate limit and no stated revocation bound. Issue
  #51.
- There is no recording capability and no server-enforced indicator, so the
  decision taken in entry 2 of issue #1 has no artefact. Issue #149, opened with
  this model.
- Bot voice receive is not built, and it is the path a recorder would use. Issue
  #53.
- There is no limit on what one peer may do to the service. Issues #94 and #52.
- There is no log surface, so there is nothing yet that can be shown to carry no
  conversation content. Issue #67.
- There is no durable store, so nothing here says what is protected at rest.
  Issue #27.
- There is no media plane at all, so every control on the media path is a
  decision rather than a mechanism. Issues #36 and #40.

## What this model deliberately does not cover

The operator's infrastructure. The host, the reverse proxy, the operating
system, the container runtime and the network in front of them. An operator who
runs this behind a proxy that terminates and forwards in clear has a problem this
model cannot see. Issue #84 is the install guide and issue #88 is the bundle, and
what those two owe the operator is a different document from this one.

The chosen media unit's own security. LiveKit is not this project's code.
`docs/decisions/media-engine.md` records the decision and states the residual
risk that this project does not own the byte path. Nothing in this model reviews
that unit, and a vulnerability in it is a vulnerability in what an operator runs.

The supply chain, except by pointer. `Dependency lock`, `dependency-review`,
`Generate SBOM`, `CodeQL`, `Scorecard` and the scheduled fuzz run all exist and
each says what it refuses in its own workflow file. What none of them is is a
threat model of the build, and this document does not supply one.

Anything about abuse between people. Harassment, brigading and what a
moderator can do about them are a product question and a governance one.
`CODE_OF_CONDUCT.md` and `GOVERNANCE.md` are where that lives.

Legal exposure and lawful access. What an operator owes a court is not an
engineering property and is not decided here. What is decided is what exists to
be handed over, and the answer today is what entry 2 of issue #1 chose and issue
#149 will build.

Quantitative risk. Nothing here is scored, ranked or given a likelihood. A
number attached to a threat against software that does not run yet would be a
number nobody derived.

## Residual risk

Every control named above as being in the tree defends a surface no peer can
currently reach, because nothing listens. The permission function is correct and
is not enforced anywhere, the framing bound is proved and is not exposed to
anyone, and the credential is stored well and opens a session nothing consumes.
That is the honest summary of this model's own strength: the parts that exist are
good and the parts that carry a conversation are not written.

The largest single gap is confidentiality in transit, and it is larger than its
one line above suggests. It is the only gap here that an operator cannot see, the
only one that is invisible to every participant, and the one where a wrong
default silently produces the failure this whole project is a reaction to.

This model is written before most of the software and will be wrong in the
ordinary way: a control will be built in a shape this document did not anticipate
and the document will not notice. What it is for is to be the thing those changes
are argued against, so the disagreement happens while it is cheap.
