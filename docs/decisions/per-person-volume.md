# Where per-person volume is applied

Status: decided, 2026-08-06. Raised by issue #10.

## The decision

Per-person volume is applied in the client. The setting is stored on the server.

## Why the client, and what the server-side alternative costs

A selective forwarding unit forwards discrete streams. Every listener already
receives each speaker as a separate stream, so a gain applied in the client is
exact, costs nothing, takes effect on the next audio buffer, and needs no round
trip.

Applying the gain at the server means the server has to decode each speaker's
stream, scale it, and re-encode it once per listener, because two listeners with
different settings can no longer share one encoded stream. That is a mixing unit
and not a forwarding one.

The two costs are not close.

CPU. Forwarding a stream is a copy and a header rewrite. Decoding, scaling and
re-encoding is an Opus decode plus an Opus encode per speaker per listener, so
the work stops scaling with the number of speakers and starts scaling with
speakers multiplied by listeners. The order-of-magnitude figure in issue #10 is
carried forward here as a claim and not as a measurement: no bench run backs it,
because the bench in issue #4 does not exist yet. What is not a claim is the
shape of the curve, which follows from the multiplication above.

Latency. A decode and a re-encode is a codec round trip inserted into the
forwarding path, which is the one segment issue #6 budgets at p99 at or below
5 ms. An Opus frame at 20 ms cannot be decoded and re-encoded inside a 5 ms
budget, so the server-side option does not miss that line by a margin, it makes
the line unreachable.

The design property that makes the whole architecture cheap is that the server
never looks inside the payload. Server-side gain is the change that gives it up.

## Where the setting is stored

Server-side, even though it is applied client-side.

The setting is per listener and per speaker. The key is the pair, listener
identity and speaker identity, and it is scoped to the person rather than to the
device or the session, so it survives a reconnect and follows the person to
another device. The client receives the set that applies to it when the session
starts, and receives an update as an event on the signalling channel when it
moves.

That makes per-person volume a synchronisation problem and not a media problem.
Nothing about it touches the media plane.

## Scale, range, and the ends of the range

The stored value is an integer meaning percent of unity amplitude gain, from 0 to
200. 100 is unity and is the default. A value equal to the default is not stored,
so a person who has never touched the control has no rows.

At 0 the listener hears nothing from that speaker. The stream is still
subscribed and still arriving, so the speaker still appears in the participant
list and still shows as speaking. This is deliberate: 0 is a comfort setting, and
a listener who has silenced somebody should still be able to see that they are
talking. It is not a mute, and nothing about it is visible to the speaker.

At 200 the amplitude is doubled, which is about +6 dB. The client applies its
output limiter after the per-speaker gains are summed, so boosting one speaker
cannot clip the mix.

The endpoints and the shape of the scale are chosen, not measured. Nothing has
been run that says 200 is the right ceiling.

## What a third-party client can do about it

A third-party client can ignore the setting entirely. It holds the decoded audio,
the gain is applied on its side, and the server has no way to know whether it was
applied.

So per-person volume is a comfort control and it is never a moderation control.
Anything that has to hold against a client that does not cooperate is a different
mechanism: a server-side mute that stops the forwarding, so the bytes never reach
the listener at all. That is issue #39, it is a separate feature, and this record
does not decide any of it beyond naming it as the thing that cannot be ignored.

A reader looking for a way to stop somebody being heard should stop here and go
to #39. This record is about a slider.
