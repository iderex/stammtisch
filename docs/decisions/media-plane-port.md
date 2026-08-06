# The media plane port

Status: decided, 2026-08-06. Raised by issue #3.

## What the port is

One language-native interface, implemented twice. Everything above it speaks
channels, rooms, members and gain. Nothing above it mentions SRTP, ICE, a track
identifier or a codec name. The engine chosen in `docs/decisions/media-engine.md`
sits underneath it and is reachable no other way.

The vocabulary is the one `docs/decisions/channel-and-room-model.md` fixed. A
channel is durable and the port never sees one. A room is the live session and is
the only thing the port creates. An occupancy is a fact about right now, and the
port reports the media half of it.

### Means

A language-native interface, not a network protocol. A wire format here would be
a second public contract with its own versioning and its own compatibility
promise, and the port has exactly one caller inside one process. It adds no
runtime and no dependency the tree does not already carry, because it is a type
declaration in whatever language issue #13 settles on. It is testable by the
harness this board plans rather than by a parallel one, which is the whole reason
the fake exists.

## The two implementations

The fake is in-memory, deterministic and driven by a fake clock. The entire
orchestration test suite runs against it, and issue #36 builds it.

The real one binds the chosen unit and is issue #40.

Both are required from the start. A port with one implementation is not a port,
it is the engine's API with different names on it.

## The operations

Each operation below states what has to hold before it is called, what holds
after it returns, and what it can fail with. An error not in an operation's list
cannot come out of that operation, and an implementation that needs one that is
not there is a change to this record rather than a detail of the binding.

Three errors are common to every operation and are not repeated per operation.
`Unavailable` means the unit could not be reached at all, and the caller learns
nothing about whether the work happened. `Cancelled` means the caller's own
deadline expired. `Internal` means the unit answered with something this port has
no vocabulary for, and it is the error that says a binding is incomplete.

`Unavailable` is the one the orchestration layer above has to have an answer for,
and `docs/decisions/channel-and-room-model.md` already wrote that answer: the
channel stays, the join fails, and the failure says the media plane is
unavailable rather than saying the channel does not exist.

### CreateRoom

Before: the caller holds a channel identity. No room exists for it, or the caller
accepts the existing one.

After: a room exists and is addressable by the identity the caller passed. The
operation is idempotent, so calling it for a room that already exists returns the
existing room and is not an error. That is deliberate, because the alternative is
every caller racing against its own retry.

Errors: `RoomLimitReached`, `InvalidIdentity`.

### DestroyRoom

Before: the caller holds a room identity.

After: no room exists under that identity, and every member of it has been
disconnected. Also idempotent: destroying a room that does not exist succeeds and
reports that it was already gone, because the caller's intent is a state and not
an event.

Errors: `InvalidIdentity`.

### AdmitMember

Before: the room exists. The caller holds a member identity and the permission
set that member is to have.

After: the unit is prepared to accept a connection from that member with exactly
those permissions, and the operation has returned whatever credential the unit
requires the client to present. The credential is opaque to everything above the
port. Nothing above it parses one, reads an expiry out of one, or constructs one.

Errors: `NoSuchRoom`, `MemberAlreadyAdmitted`, `PermissionsNotExpressible`,
`InvalidIdentity`.

`PermissionsNotExpressible` is the interesting one. It is how a unit says that a
permission the orchestration layer wants is not something it can enforce. The
caller's answer is to refuse the admission rather than to admit with a weaker set,
because a permission silently downgraded at this boundary is a permission the
layer above believes it has.

### RevokeMember

Before: the room exists.

After: the member is not in the room, the credential minted for them no longer
admits anybody, and a member who was connected has been disconnected. Idempotent
for a member who was never admitted.

Errors: `NoSuchRoom`.

### PublishSource and UnpublishSource

Before: the room exists and the member is admitted. For publish, the member's
permission set allows publishing.

After: the source is present in, or absent from, the room's set of sources. The
port names a source by member identity and a kind, which is audio or video.
Nothing above the port names a track.

Errors: `NoSuchRoom`, `NoSuchMember`, `NotPermitted`, `SourceAlreadyPublished`
for publish only.

### Subscribe and Unsubscribe

Before: the room exists, both members are admitted, and the source exists.

After: the subscriber does or does not receive that source. Subscription is per
pair, subscriber and source, which is what issue #42 manages above the port and
what makes the gain question below answerable at all.

Errors: `NoSuchRoom`, `NoSuchMember`, `NoSuchSource`, `NotPermitted`.

### SetSubscriberGain

Before: the subscription exists.

After: either the unit is applying that gain to that one subscriber's copy of
that source, or the operation has returned `GainNotSupported` and applied
nothing.

`GainNotSupported` is not a failure. It is the unit reporting a capability, and
`docs/decisions/per-person-volume.md` already decided that the client is where
the gain is applied, so the expected answer from a forwarding unit is exactly
this error. The operation exists so that a future unit which can do it server
side does not need a new port, and so that the answer is asked for rather than
assumed.

Errors: `GainNotSupported`, `NoSuchSubscription`, `GainOutOfRange`.

### SpeakingState

Before: the room exists.

After: the caller has, per member, whether the unit currently believes that
member is speaking. This is a report and it changes nothing.

Errors: `NoSuchRoom`, `NotObservable`.

### HopMetrics

Before: the room exists.

After: the caller has, per member, the delay and loss the unit observes on that
member's hop, and the clock the unit measured them against. The clock is returned
rather than assumed, because a figure with an unstated clock is not a figure
issue #46 can hold against a budget line.

Errors: `NoSuchRoom`, `NotObservable`.

### DrainRoom

Before: the room exists.

After: no new admission succeeds for that room, existing members are told the
room is closing, and the operation returns once they have left or the caller's
deadline expires. Drain is the graceful path and `DestroyRoom` is the abrupt one.
Both exist because a shutdown that only has the abrupt one drops conversations.

Errors: `NoSuchRoom`, `DrainDeadlineExceeded`.

## Where the fake and the real implementation must agree, and where they may not

The suite is only worth running if a pass against the fake means something about
the real unit. So the default is that they agree, and each departure is named
here with its reason. An implementation that departs anywhere else is wrong.

They must agree on every error listed above: which operations can return which
error, and which conditions produce it. This is the load-bearing one. The whole
purpose of the fake is that the orchestration layer's error handling is exercised
without a network, and a fake that cannot produce `PermissionsNotExpressible` or
`GainNotSupported` leaves that handling untested.

They must agree on idempotence, on the pre- and post-conditions above, and on the
observable effect of every operation on every subsequent read. A member revoked
is a member absent from the next `SpeakingState`, in both.

They may differ on timing, and they will. The fake returns on a fake clock and
does no work. Nothing in the orchestration layer may depend on how long an
operation takes, and a test that passes only because the fake is instant is a
test asserting something the port does not promise.

They may differ on the content of a credential. The fake mints a credential that
means nothing outside the fake. This is safe only because the credential is
opaque above the port, and it is the reason that opacity is a rule rather than a
convention.

They may differ on the values in `HopMetrics` and `SpeakingState`. The fake
returns what a test set, because the alternative is a fake that simulates a
network, which is a second media plane nobody will maintain. What they may not
differ on is the shape, including which clock is named.

They may differ on `RoomLimitReached`. The fake's limit is whatever a test sets
and the real one's is the unit's. The error has to be reachable in both.

## What the port deliberately cannot express

The port cannot express anything about the contents of a stream. There is no
operation that decodes, mixes, transcodes, records, or reads a payload. That is
not an omission to be filled in later. The property that the server never looks
inside the payload is what `docs/decisions/per-person-volume.md` identified as
the thing that makes the architecture cheap, and an operation here that took a
payload would be the change that gives it up.

A caller who needs the audio itself does not get it through this port. They get
it as a subscriber, over the same media path as any other member, which is what
the bot voice receive capability in issues #11 and #53 is. That path carries an
identity, a permission set and a credential, so what looks like a restriction is
also the only reason a recording capability could ever be accounted for. Entry 2
of issue #1 is where whether it exists at all is decided, and this record does not
pre-empt it.

Second, the port cannot express codec choice or bitrate policy. Those live inside
the unit and are issue #44. A caller who needs a different codec configures the
unit, not the port, and that configuration is deliberately not reachable from the
orchestration layer, because a layer that can set a codec is a layer that has to
know which codecs the unit has.

Third, the port cannot express a room that spans two units. `docs/decisions/`
carries the federation record beside this one, and the obligation it places here
is that a room may be backed by a unit that is not local, which the port already
allows, because a room identity says nothing about where the unit is. Two units
in one room is a different thing and is not expressible.

## Residual risk

The error lists above are derived from what the operations have to do, not from a
binding that exists. Nothing has been implemented against the chosen unit yet, so
`Internal` is the error that will be hit first, and every time it is hit it means
this record was incomplete rather than that the unit misbehaved. Issue #40 is
where that is found out.

The port is a claim that the engine is replaceable, and it stays a claim until
something other than the chosen unit has been put behind it. The fake is not that
proof. It is deterministic and it never has to make an engine's API fit, which is
where the tendrils would come from.
