# Confidentiality in transit, on the signalling leg and on the media leg

Status: decided, 2026-08-09. Raised by issue #148.

## The decision

A conversation carried by this software is confidential on the wire, on both
legs, and a deployment in which it is not has to be refused rather than
tolerated. The two legs get separate answers because they are different
problems, and collapsing them is how a proxy in front of the signalling
connection ends up read as an answer about media.

### The signalling connection

The connection between a participant and the operator's infrastructure carries
credentials, room membership, who is speaking and who is present. It is
confidential, and exactly two arrangements satisfy that:

- this process terminates TLS itself
- something in front of it terminates TLS and forwards to it over a link the
  operator controls, which is the reverse proxy a self-hosted service usually
  sits behind

Which of the two applies is an argument to the function that produces a
signalling connection, and the value that refuses is the zero value:

    grep -n 'TLSHere Transit = iota' internal/transport/confidentiality.go

A caller that says nothing therefore gets the refusal. That is the whole shape
of the decision: the arrangement this project exists against is the one nobody
has to choose, so it is the one that cannot be reached by leaving something out.

A refused request is answered 403 and no connection is made for it. It is not a
redirect to the same address under another scheme, because the client is opening
a socket rather than following a link, and it is not a silent drop, because a
client author debugging a failed connection is owed the reason.

### The media path

Media between a participant and the selective forwarding unit is not this
project's byte path. `docs/decisions/media-engine.md` records that, and it is
the reason the answer here is a requirement on the deployment rather than a
guard in this tree.

The requirement is that the media path is protected by DTLS-SRTP. This is not a
choice being made here. The WebRTC security architecture, RFC 8827, requires
DTLS-SRTP and defines no unprotected arm for media, and a browser has no mode in
which it sends RTP in the clear. So for the leg between a participant and the
unit, the standard already decides it and an operator cannot turn it off from
this side.

What is not decided by the standard is everything around that leg. The
connection between this process and the unit's own control interface carries
room admission and the credentials minted for it, and that connection is
ordinary client-to-server traffic which has to be TLS. Nothing in this tree
carries it yet, because there is no adapter. That is issue #40, and issue #41 is
the admission and credential minting that would travel over it. This record is
where those two find the requirement; neither is enforced by anything today.

## What is enforced today, and how far it reaches

The refusal is in the package that turns an HTTP request into a signalling
connection, and it is proved by standing up the refused arrangement:

    go test ./internal/transport -run TestARequestThatDidNotArriveOverTLSIsRefused -count=1

It reaches no further than that package, and the reason is worth stating plainly
rather than leaving to be discovered. Nothing in this tree serves that handler.
The entry point prints one sentence and exits:

    grep -n 'nothing is implemented yet' main.go

    git grep -l 'transport\.Handler(' -- '*.go'
    internal/transport/confidentiality_test.go
    internal/transport/websocket_test.go

The only callers are the suite. So what exists today is a requirement that binds
the first process to serve this handler, and not a running service that refuses
anything. A reader who takes this record as a report about a service carrying
conversations would be taking it for more than it is.

There is no refusal at startup, because there is no startup. Issue #69 is the
startup self-check, and it is where a deployment that does not meet this record
is made to say so at the moment an operator would see it.

## What the declaration cannot check

The value that admits a forwarded request is the operator's word and not a
measurement. Nothing in this process can tell a request handed over by a proxy
on the same host from one that crossed a network in the clear before it arrived.
The forwarded arrangement is therefore trusted, and it is trusted deliberately,
because the alternative is refusing the deployment shape almost every
self-hosted service actually has.

What the value does buy is that the trust is written at the call site, in a name
that says what is being claimed, rather than being the absence of a setting
nobody looked at. The failure this record is about is silent, and a declaration
that has to be typed is not silent.

Two issues carry the parts that would narrow it. Issue #66 is the validated
configuration, which is where this value stops being a call-site argument and
becomes a key an operator sets with the same default. Issue #69 is where a
deployment can be made to report which of the two arrangements it is in, so an
operator sees it in the log they paste when they ask for help.

## Why the proxy answer is not enough on its own

An operator putting a reverse proxy in front is the likely deployment and a
reasonable one. It is not an answer until it is a requirement, because the
failure mode is silent in both directions.

A service that starts happily on a clear connection produces exactly the outcome
this project is a reaction to, and produces it with nobody seeing a warning. The
person running it has no signal, and the people in the conversation have less
than that, because they cannot see the deployment at all. That asymmetry is what
makes this different from a misconfiguration an operator eventually notices.

And a proxy in front of the signalling connection says nothing about media. A
selective forwarding unit's media does not pass through an ordinary reverse
proxy, which is the point issue #88 makes about the ports a bundle has to state,
so an operator who has terminated TLS at their proxy has done nothing at all to
the second leg. Answering the two legs in one sentence is the mistake this
record is split in two to avoid.

## Residual risk

The forwarded arrangement is unverifiable from here, as above, and an operator
who declares it wrongly gets a service that behaves exactly like one that is
right.

The media leg has no control in this tree at all. Its confidentiality rests on
the standard and on the unit being deployed the way this record requires, and
neither has been checked by anything, because there is nothing here to check
them with. That is a gap held open by issues #40 and #41 rather than one this
record closes.

Nothing here covers confidentiality at rest, on either side. The credential
store, the durable domain and whatever an operator's backup holds are a separate
question, and issues #27 and #70 are where it lives.

Nothing here covers metadata. TLS on the signalling connection hides what is
said about a conversation from somebody on the path, and it does not hide that a
connection exists, how long it lasted or how much it carried. An observer who
can see the traffic can still see that somebody is in a call.

## When this is revisited

Two conditions, and both are events rather than dates.

The first process that serves the signalling handler is where the refusal
becomes a property of a running service rather than of a package. That change
carries the startup half in issue #69 and the configuration half in issue #66,
and it is where this record is read again to check that the two arrangements
above are still the only two.

A deployment shape neither arrangement describes reopens it. The pair here is
exhaustive over what an operator can do today, and a transport that is not
HTTP-over-TLS, or a unit reached over something other than its control
interface, would be a third case rather than a variation on these two.
