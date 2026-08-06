# The channel and room model, including always-on rooms

Status: decided, 2026-08-06. Raised by issue #8.

## The three things, kept apart

Most conferencing products conflate these, and the result is a call that has to be
started. The thing people want back is a place that is already there.

A channel is a durable, named, permissioned place. It exists whether or not
anybody is in it. It has an identity that outlives every session held in it, a
permission set, and a participant list, which may be empty.

A room is the live media session backing a channel. It may or may not be running.
A channel with no room is a normal state and not a fault.

An occupancy is one member currently present in one channel. It is a fact about
right now.

## What survives a server restart

The channel survives. It is persisted, and its identity, name and permissions come
back unchanged. Which store holds it is issue #27 and is not decided here.

The room does not survive. It is live session state on the media plane, and a
restart of either side ends it.

An occupancy does not survive. It is rebuilt as clients reconnect, so the
participant list of a channel after a restart is whatever has come back, and it is
correct rather than stale only once the reconnects have settled. Issue #34 owns
what the client does on that path.

So one of the three is durable and two are not, and the durable one is the one
the product is about.

## The lifecycle

A room is created lazily on the first join to its channel, and torn down after the
last leave plus a grace period.

The grace period exists so a network blip does not destroy and rebuild the session
under everybody who is still there.

## The grace period is 30 seconds

The number comes from the transport, because the failure it is defending against
is a transport failure.

The engine chosen in `docs/decisions/media-engine.md` is LiveKit, which is built
on Pion, and Pion's ICE agent carries the timeouts that decide how long a
connection can be interrupted before it is declared dead:

    gh api 'repos/pion/ice/contents/agent_config.go?ref=v4.4.0' --jq '.content' | base64 -d | grep -n 'defaultKeepaliveInterval = \|defaultDisconnectedTimeout = \|defaultFailedTimeout = '
    21:	defaultKeepaliveInterval = 2 * time.Second
    24:	defaultDisconnectedTimeout = 5 * time.Second
    27:	defaultFailedTimeout = 25 * time.Second

Five seconds of silence moves the agent to disconnected. Twenty-five more move it
to failed. So 30 seconds is the whole window in which the transport can still
recover on its own without the client rebuilding anything. Tearing the room down
earlier destroys a session that was about to come back. Holding it longer buys
nothing for this failure, because past 30 seconds ICE has already given up and the
client is establishing a new connection anyway, which is a join.

For comparison, the engine's own defaults at the version that carries them:

    gh api 'repos/livekit/livekit/contents/pkg/config/config.go?ref=v1.13.5' --jq '.content' | base64 -d | grep -n 'EmptyTimeout\|DepartureTimeout'
    214:	EmptyTimeout       uint32             `yaml:"empty_timeout,omitempty"`
    215:	DepartureTimeout   uint32             `yaml:"departure_timeout,omitempty"`
    500:		EmptyTimeout:          5 * 60,
    501:		DepartureTimeout:      20,

Neither of those is adopted as the grace period. `departure_timeout` at 20 seconds
is the engine deciding when a participant who left is gone, which is a different
event. `empty_timeout` at 300 seconds is the engine reclaiming an idle room on its
own schedule, which would keep every room alive for five minutes after the last
leave and make the per-channel always-on setting below meaningless. Both commands
were run on 2026-08-06 against the versions named in them.

Two consequences follow and are written down so they are not discovered later.
The engine's `empty_timeout` has to be configured to match our grace period for a
channel that is not always-on, or the engine's number silently wins. For an
always-on channel it has to be disabled outright, or the engine reclaims the room
we are deliberately holding.

Both timeouts above are defaults read from source. Neither the 30 seconds nor the
claim that it is the right number has been measured against a real network. The
evidence is what the transport declares about itself, not an observation of a
blip.

## What an always-on channel holds

Only orchestration state, plus a room object on the media plane that has no
transport resources allocated to it.

Always-on is a per-channel setting and never a global mode, because the idle cost
is real and an operator should be able to pay it only where it buys something.

The room is held rather than torn down, but a held empty room is not an expensive
thing. Ports, peer connections, SRTP contexts and jitter buffers are per
participant, and an empty room has no participants, so it allocates none of them.
What remains resident is a record on each side: ours, and the engine's registry
entry.

The budget line in issue #6 is one thousand empty always-on rooms held for one
hour under 200 MB resident and under 2 percent of one core, which is about 200 kB
per held room. The answer above is consistent with that line only if a held empty
room really is a record and not a transport allocation. That consistency is an
argument from where the allocations live and it is not a measurement. Issue #47
is the measurement, and until it has run, the 200 kB figure is a target that
nothing has been held to.

## When the media plane is unavailable

The channel does not disappear. That is the whole point of separating a channel
from a room.

An always-on channel with no reachable media plane still appears in the channel
list, still carries its permissions, and still has a participant list, which is
empty. Its always-on setting is untouched.

What changes is that a join fails, and it fails with a reason that says the media
plane is unavailable rather than with a reason that says the channel does not
exist. A person who sees the channel and cannot enter it has been told the truth.
A person whose channel vanished has been told a lie about a durable object.

When the media plane comes back, a normal channel gets its room on the next join.
An always-on channel gets its room without waiting for one, because that is what
the setting asked for.

Whether an always-on channel is re-established one at a time or all at once when a
media plane returns is not decided here, and a naive all-at-once is a thundering
herd against a service that has just come back. Issue #69 is where startup and
readiness are settled and is the right place for it.
