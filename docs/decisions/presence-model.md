# The presence model behind a participant list visible before joining

Status: decided, 2026-08-06. Raised by issue #9.

## What this decides

Seeing who is in a voice channel before you join it is the feature that makes the
commercial product feel alive, and it is the one the open alternatives most
consistently omit. It requires the server to publish occupancy for channels the
viewer is not in, which is a permissions question, a fan-out question and a
privacy question at once. This record answers all three.

The vocabulary is the one `docs/decisions/channel-and-room-model.md` fixed. An
occupancy is one member currently present in one channel, it is a fact about right
now, and it does not survive a restart.

## The occupancy record

Four fields, and each one is here because something a viewer does needs it.

Channel identity. Which channel this occupancy is in. Without it the record
cannot be placed, and occupancy is delivered as a stream of changes rather than
as a whole list, so every change has to say where it lands.

Member identity. Which person is present. It is the person, not the device
and not the session, which is the same key `docs/decisions/per-person-volume.md`
stores settings under. A person on two devices in one channel is one occupancy,
because the viewer is looking at a room full of people and not a room full of
connections.

The record carries the identity and nothing else about the person. The display
name and the avatar are resolved by the client from the space's member directory
it already holds. That is not only a size argument. An occupancy is delivered to
everybody who can see the channel, so every field in it is disclosed at that
width, and a field that can be looked up separately should be.

Present since, in whole seconds on the server clock. It gives the list a
stable order that does not jump as unrelated changes arrive, and it answers the
question a viewer actually has, which is whether this is a conversation that has
been going for an hour or two people who just walked in.

Whole seconds rather than the millisecond the event carries, because the precision
buys nothing for either use and the cost is real: this field is the one that says
how long a named person has been sitting in a room, to everybody who can see that
room.

Self-mute state. Whether the member has muted their own microphone. It is
here because it is the difference between five people talking and five people
idle, and that is the decision the viewer is making. It is set by the member
themselves and is not a moderation state.

### What the record does not contain

Nothing about what is being said. No audio level, no speaking state, no text, no
duration of speech, no count of anything spoken.

Speaking state is deliberately not in the occupancy record even though the media
plane port reports it. It changes several times a second per speaker, and
publishing it to everybody who can see the channel is the fan-out problem
multiplied by the speech rate. It is sent only to clients that are in the room,
over the same subscription that carries their media session.

No device, no client version, no operating system, no network address, no region.
None of it is needed to render a participant list, and all of it is a signal about
a person's habits that an occupancy broadcast would spread to everybody in the
space.

A server-side mute is a moderation state and is issue #39. Whether it appears in
the pre-join view is decided there and not here.

## Permissions

The view permission and the join permission are separate, and this is the change
that makes the feature possible at all.

View means the channel exists for you. You see its name, its position in the
list, and its occupancy.

Join means you can enter it.

A channel you can view but cannot join shows its occupancy in full. That is the
useful case, not an edge case: a channel for one team, visible to the rest of the
space, tells everybody who is currently in a meeting without letting them walk in.

A channel you cannot view shows nothing, including its existence. No name, no
count, no placeholder, no gap in the ordering, and no error that a client can tell
apart from asking about a channel that was never created. A viewer who can
distinguish those two answers has learned that the channel exists, which is the
thing the permission was withholding.

Occupancy is filtered by channel visibility and by nothing else. If you can view
the channel you see every occupancy in it. There is no per-member hiding inside a
visible channel, because a list that silently omits people is worse than no list:
a viewer who joins a room they were told was empty has been misled by the feature
that exists to tell them the truth.

## The subscription model

Two tiers, and the split is what keeps the fan-out survivable.

Counts are pushed unasked. A client that has a space open is sent, without
asking, the occupancy count of every channel in that space it can view. This is
the tier that never stops, because a count per visible channel is what the channel
list renders.

Membership is subscribed to. The identities behind a count are sent only for
channels the client has explicitly subscribed to. A client subscribes to the
channel it is in, and to the channels whose participant lists it is actually
showing. It unsubscribes when it stops showing them.

Changes are coalesced. The server holds changes for a fixed interval, currently
500 ms, and sends each client at most one message per interval carrying every
change in that window that the client is entitled to and subscribed to. A message
with no changes in it is not sent.

## The worked count

A space of 200 channels and 1000 connected clients. One person moves from channel
A to channel B. Assume the worst case for fan-out, which is that every channel is
viewable by everybody, so no client is filtered out by permissions. Assume A held
5 members and B held 3 before the move, and that no client has any participant
list expanded beyond the channel it is in.

The move is two changes: the count of A goes down by one, the count of B goes up
by one.

Pushed to every connected client, one message per change, the naive design sends
2000 messages.

Under the model above, the two changes land in the same 500 ms window and are
coalesced, so each of the 1000 clients receives one message carrying two count
deltas. That is 1000 messages and 2000 deltas.

The identity of the person who moved goes only to the clients subscribed to the
membership of A or B, which here is the 5 members of A plus the 3 of B, so 8
messages. The other 992 clients learn that a count changed and never learn who.

For one move the saving is a factor of two on messages, and that is not the case
the design is for. Fifty people moving inside one 500 ms window, which is what the
end of a scheduled event looks like, is 100 changes: 100,000 messages naively, and
still 1000 messages under this model, because the count of messages is bounded by
the number of clients per interval and not by the number of changes.

The bound is the point. One person walking into a room costs at most one message
per client per interval, however many people walk at once.

None of these figures is measured. They are counted from the model, and the
measurement is the fan-out cost under load, which is issue #94.

## Staleness

The bound is p95 at or below 2 seconds from the event to the list being correct,
for a channel the viewer has not joined. It is the participant list staleness line
in issue #6.

That line does not yet live in a record. Issue #6 holds the budget as a set of
provisional figures, and until it lands as a decision record with a command behind
each figure, the 2 seconds is a number this record is designed against rather than
a threshold anything is held to.

The 500 ms coalescing interval was chosen to sit inside that bound with room for
the queue and the network, not to be as fast as possible. A shorter interval
raises the message count per client for a staleness improvement nothing asked for.
A longer one eats the margin between the interval and the bound, and the margin is
where the rest of the path lives.

## Residual risk

Occupancy is a presence signal, and this record publishes it to everybody who can
view a channel. Somebody who can see a space can tell when a given person is at
their desk, which room they are in, and how long they have been there. That is
inherent in the feature and it is not mitigated by anything above. The fields were
cut to the minimum that renders the list, which reduces what is disclosed and does
not change what the disclosure is.

Nothing here has been measured. The 500 ms interval, the two tiers and the counts
above are a design, and the numbers in the worked count follow from the design
rather than from a run.
