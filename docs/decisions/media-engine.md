# The media engine

Status: decided, 2026-08-06. Raised by issue #2.

## The decision

This project adopts an existing selective forwarding unit rather than owning the
byte path, and it keeps the door open to owning it later. The unit is LiveKit.

The order of those two clauses is the decision. Adopting comes first because the
commodity parts of a media plane are unforgiving and are not what this project
is for. Keeping the door open comes second because the thing this project sells
is latency, and latency in a component nobody here maintains is a dependency on
somebody else's roadmap.

## Why adopt rather than own

Owning the byte path means owning SRTP, ICE, bandwidth estimation, simulcast and
layer selection, packet loss concealment, retransmission and keyframe policy.
Those are years of work and their failure modes are subtle and load-dependent,
which means they surface under real load and not in a test.

The commercial incumbent did write its own, in C++, and it also skipped ICE
entirely, because every one of its clients reaches a relay it owns. That shortcut
is available to an operator of hundreds of media servers across a dozen regions.
It is not available here. The operator this project is built for sits behind a
home router, which is the case that needs the full ICE and TURN machinery that a
mature unit already carries.

## Why LiveKit

It is Apache-2.0 and it is Go, so an operator gets the one static binary they
expect and the tree carries no second runtime.

It is built on Pion, which is MIT and Go. That matters for the door being open:
dropping the LiveKit layer later and driving Pion directly is a change of
component within one language and one ecosystem, not a rewrite into another.

It already models rooms, participants and tracks server-side, which is most of
the surface the orchestration layer has to talk to.

## The alternatives and why each was not taken

Owning the byte path directly on Pion. Not taken because it buys control of
latency at the price of the entire commodity list above, and that list has to be
paid before the first conversation works at all. This is the option the decision
keeps open rather than the one it rejects.

mediasoup. ISC and C++, driven from Node. Not taken because it adds a second
runtime the tree would otherwise not carry, and every operator inherits that
cost.

Janus. GPL-3.0. This one was blocked rather than judged on merit: entry 1 of
issue #1 was the licence of this repository, and a GPL-3.0 engine pre-empts it.
That entry has since been answered, and the answer is AGPL-3.0, landed in
`LICENSE` on `3d33d03`. So the block is gone and the argument was never made. It
is recorded here as blocked rather than as rejected, because those are different
statements and a later reader is owed the true one.

Jitsi Videobridge. Apache-2.0 and Kotlin. Not taken because it pulls in a JVM,
which is the same second-runtime cost as mediasoup in a different shape.

Galene. MIT and Go. It appears in the survey command below and issue #2 argued no
case against it, so nothing here rejects it on merit either. It is smaller than
LiveKit and does not model rooms, participants and tracks with a server-side API
of the shape the orchestration layer needs, which is the reason LiveKit was
preferred. That is a preference stated here for the first time, not a finding
carried over from the issue.

## The survey, with the command that produced it

Run on 2026-08-06 against the branch that carries this file:

    for r in livekit/livekit pion/webrtc versatica/mediasoup jitsi/jitsi-videobridge meetecho/janus-gateway jech/galene; do gh api "repos/$r" --jq '"\(.full_name) \(.license.spdx_id) \(.language)"'; done
    livekit/livekit Apache-2.0 Go
    pion/webrtc MIT Go
    versatica/mediasoup ISC C++
    jitsi/jitsi-videobridge Apache-2.0 Kotlin
    meetecho/janus-gateway GPL-3.0 C
    jech/galene MIT Go

The command reads the licence and language GitHub reports for each repository at
the moment it runs. It does not read a pinned version, so a licence change
upstream moves the output. Re-run it rather than trusting the paste.

## Residual risk

We will not own the byte path. A latency regression inside the unit is not ours
to fix on our own schedule.

That sentence is the risk and it is not softened by what follows. Two things
hold it, and neither removes it. The media plane port, issue #3, makes a
replacement a bounded project rather than a rewrite. The bench, issue #4,
measures the unit as a black box, so a regression arrives as a red number from a
run rather than as a report from a user. Both of those shorten the response to a
regression. Neither makes the regression ours to fix at its source.

## When this is revisited

The trigger is a number the bench produces, not a judgement about the upstream
project.

Forwarding delay through the media plane, ingress packet to egress packet against
one clock, has a budget line of p99 at or below 5 ms in issue #6. This decision
is revisited when a bench run on the reference profile reports p99 forwarding
delay above 5 ms, and a re-run on the same profile and the same commit of the
unit reports it again. One run is noise. Two consecutive runs on an unchanged
system are the signal, and issue #4 requires the rig to be repeatable to within
10 ms at p95 before its numbers are used for anything, which is what makes the
second run mean something.

Revisiting means opening the question of driving Pion directly. It does not mean
raising the budget line.

Neither the bench nor the budget exists yet. Until they do, this decision has no
live revisit trigger, and that is a gap rather than a state of safety.
