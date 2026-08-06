# Federation, and what stays possible without it

Status: decided, 2026-08-06. Raised by issue #7.

## The decision

The first release does not federate. One server holds a space, and a
conversation never spans two of them.

This is a decision against federation now, recorded so that it is not a deferral
that becomes permanent by accident. The difference is the obligations at the end,
which are the parts that keep the door open and which get built whether or not
anybody uses them.

## Why, in this order

### Federation multiplies the hard part

Two servers in one conversation means cascading media between selective
forwarding units, agreeing which unit is the focus for a given room, and trusting
a token minted somewhere else. The Matrix group call work is the honest reference
here. The backend for a call is chosen by whichever participant joined first,
advertised in room state, and the rest follow it. That is a workable design, and
it is also a large amount of protocol standing between a person and their first
conversation.

Every part of it lands on the layer this project is actually building. The media
plane port in `docs/decisions/media-plane-port.md` deliberately cannot express a
room that spans two units, and that is one line in one record only because
nothing above it has to reason about a room with two homes.

### The product claim is stronger without it

If media never leaves the operator's infrastructure, that is a sentence an
operator can put in front of their community and defend without a footnote.
Federation makes it conditional, and a conditional sovereignty claim is a weaker
thing to sell than the plain one.

### Federation is not what makes somebody leave the commercial product

The switching speed, the visible participant list and the finish are. Nobody has
stayed with the incumbent because their server could talk to another server.

## What has to stay possible

Three obligations, each carried by an issue on this board. They are cheap now and
expensive to retrofit, which is the whole reason for writing them down while the
decision is against the thing they are for.

Identifiers are globally addressable in form from the start. A room is
addressable as a name plus a host, even while there is only ever one host, so the
form does not have to change later and every stored identifier does not have to
be rewritten. Issue #26 carries this, because it is the domain model that fixes
the shape.

The bot API is host-relative rather than host-assuming. A bot addresses things
relative to the host it is talking to, and nothing in the contract encodes the
assumption that there is only one. Issue #50 carries this, because it is the
contract that would have to be versioned if the assumption got in.

The media plane port allows a room to be backed by a unit that is not local, even
though nothing will use that. Issue #3 carries this, and the record it produced
already holds it: a room identity says nothing about where its unit runs.

## What is not obliged

Nothing in the wire protocol reserves space for a remote server, no identifier is
resolved against anything remote, and no credential minted here is accepted
anywhere else. Those are the parts that would be speculative work for a feature
that has been decided against, and the three obligations above are deliberately
the ones that cost nothing today.

## When this is reopened

The condition is an operator who runs this software asking for it, and it is
written as a count rather than as a feeling: three separate operators, each
running an instance with members who are not the operator, asking on the tracker
for their instances to talk to each other, and each naming the conversation they
cannot have today.

That is observable, it does not arrive on a date, and it is not satisfied by
people who would like the feature in the abstract. It requires running instances,
which means the software works well enough to be run, which is the state in which
federation is worth the protocol it costs.

Reopening means opening the design question. It does not mean the first release
grows the feature, and it does not retire any of the three obligations, which are
what make the reopened question a design rather than a rewrite.

## Residual risk

A decision against federation is also a decision that a person on one instance
cannot talk to a person on another, and there is no workaround for them. They
join the other instance, or they do not have the conversation. For a community
that already spans two operators this is a reason not to adopt the software at
all, and the three obligations above do nothing for them.

The reopening condition is counted on the tracker, and the tracker is where
people who already left do not appear. So the condition is biased toward
operators who adopted despite the gap and against the ones the gap turned away,
and nothing here measures the second group.
