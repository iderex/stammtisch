# The audio codec and its parameters

Status: decided, 2026-09-04. Raised by issue #44.

## The decision

The audio configuration is fixed here rather than taken from whatever a library
defaults to. Six parameters, each with the reason it has that value:

| Parameter | Value | Why |
| --- | --- | --- |
| Codec | Opus | the latency budget is written for it, and nothing else was ever proposed |
| Frame duration | 20 ms | the budget's capture and playback lines assume it, and it is the one value those lines depend on |
| Sample rate | 48 kHz | Opus's native rate, so nothing resamples on either side |
| Channels | 1 | a conversation is voice, and a second channel doubles the forwarding cost for nothing the budget measures |
| Forward error correction | on | a lost frame concealed at the far end is cheaper than a retransmission the budget has no room for |
| Silence suppression | on | it is what makes a room of forty with two speaking cheap, and the forwarding path's sequence handling is tested against it rather than avoided by turning it off |

## Why this is a record and not a default

`docs/decisions/latency-budget.md` is written for this configuration and not for
a general one. Two of its lines are frame arithmetic over a 20 ms frame and
nothing else:

    git grep -n 'one 20 ms Opus frame plus a device buffer\|two 20 ms frames of nominal depth' -- docs/decisions/latency-budget.md

So a library that negotiated 40 ms frames because that is what it does when
nobody says otherwise would not make the budget wrong. It would make the budget
about a different system, and every figure in it would go on reading as though it
were about this one. That is the failure this record exists against, and it is
silent in exactly the way a wrong number in a document is silent.

## The two parameters that are more than arithmetic

Forward error correction and silence suppression are the two that change what
the forwarding path has to cope with, so each is written down with what it costs
rather than only with what it buys.

Forward error correction spends bandwidth on every packet so that a lost one is
concealed at the far end rather than asked for again. The budget has no room for
the second. A retransmission is at least one round trip, and the four segments
on the mouth-to-ear path that this project owns already come to 100 ms:

    git grep -n 'p95 at or below 30 ms\|p99 at or below 5 ms\|p95 at or below 40 ms with no loss\|p95 at or below 25 ms' -- docs/decisions/latency-budget.md

Against the 150 ms line that leaves 50 ms for both network legs and for
anything the list does not name. The sum is a conservative bound rather than a
prediction, because percentiles do not add and one of the four is quoted at
p99 where the others are at p95, so the true figure sits at or below it. Even
read that way there is no round trip in the remainder. So the trade is
bandwidth against time there is none of, and it goes to bandwidth.

Silence suppression is what makes a large channel cheap. In a room of forty with
two people speaking, a unit forwarding only what is spoken forwards two streams
rather than forty. It is also what makes the forwarding path's sequence handling
subtle, because a stream that stops and restarts leaves gaps that are not loss
and must not be treated as loss. The answer here is that the path is tested
against that rather than shielded from it: turning suppression off to keep
sequence handling simple would buy simplicity in the one place the product
cannot afford the cost.

## Video

Loose, deliberately. The budget does not depend on it, so fixing it now would fix
something no measurement asked for, and a video parameter chosen before anything
renders a frame is a parameter chosen from nothing. It is decided by whichever
issue first carries video.

## Bitrate is in this issue's title and not in its parameter list

Issue #44 is called "Codec and bitrate policy" and the six parameters it
enumerates carry no bitrate. Nothing here fixes one, and that is stated rather
than left as an omission a reader has to notice.

Opus is a variable-rate codec and the figure that would be pinned is an encoder
target, not a rate the wire carries. A target chosen today would be chosen from
nothing: the budget has no bandwidth line, and what a target costs is the
forwarding cost under it, which is #46 and #48 and is not measured. So the
honest state is that this is undecided, and the condition for deciding it is a
measurement rather than an argument.

## What is enforced today, and what is not

Nothing. This record fixes six values and no code reads them, because nothing in
this tree negotiates anything with a media plane:

    git ls-files internal/media
    internal/media/doc.go

The second Done-when line of #44 asks for a test that the negotiated
configuration matches the record, so a library default change is caught rather
than absorbed. That test needs something that negotiates, which is the port
declaration and the adapter behind it, and #36 and #40 own those. Until then this
is a decision and not a mechanism, which is the state the contribution guide
asks to be written where a reader will see it rather than assumed.

The third line asks that the bench's runs record the configuration they ran
under, so a figure can be traced to the parameters that produced it. The rig
records the network shaping profile in force and says in the flag's own text that
it does not apply it:

    git grep -n 'recorded verbatim and not run here' -- bench/cmd/mouth-to-ear/main.go

It records no audio configuration, and there is nothing yet for it to record one
of. The only implementation of `bench.System` in the tree is a fixture whose
whole behaviour is a delay, and it encodes nothing:

    git grep -n 'implementation here is DelayLine' -- bench/system.go

So that line waits on the same absence and on the bench runs that would carry
it.

## Residual risk

The six values are chosen against a budget whose every line is pinned rather
than measured, which `docs/decisions/latency-budget.md` states about itself. So
this record inherits that: the frame duration is right for the arithmetic in that
budget, and the arithmetic has not been checked against a running system.

Silence suppression on is the value most likely to be regretted, and the reason
is not the bandwidth. It is that the failure it introduces, a gap that is not
loss being read as loss, appears under exactly the conditions a test suite is
worst at reaching, and appears as degraded audio rather than as an error.

## When this is revisited

When forwarding delay is measured against the budget, which is #46, or when
behaviour under loss and constrained bandwidth is characterised, which is #48.
Either could move the forward error correction line, and #48 is where a bitrate
target would first have something to be chosen against.
