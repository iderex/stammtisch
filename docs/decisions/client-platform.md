# The client platform

Status: decided, 2026-08-08. Raised by issue #58.

## The decision

The first client is a browser application. It is written in TypeScript, it
drives the media plane through the LiveKit JavaScript client SDK, and it is
built to static files that the server binary carries and serves. No native
client is built for the first release.

The second sentence is load-bearing and is not a detail of packaging. What an
operator installs stays one file, which is what
`docs/decisions/server-language.md` promised and what the audience this project
is for expects. A browser client that arrives as a directory of assets the
operator has to put behind a second web server would take that promise back.

## What the means had to carry

Issue #58 names four things. A fifth decides the question and it is not on that
list, so it is written here rather than assumed.

Audio capture and playback with control over buffering. The budget's capture
line and playback line belong to the client, and this is the one requirement a
browser answers worst. It is answered under the budget section below rather than
here, because the honest answer is a cost and not a capability.

A participant list that updates without a visible repaint. Presence arrives as
events over the signalling connection and the list is rendered from them.
`docs/decisions/presence-model.md` already bounds what arrives at one message per
client per interval, so this is a rendering question rather than a transport one,
and every browser rendering engine of the last decade updates a list in place
without repainting the page around it.

A switch that feels instant. The budget line is p95 at or below 300 ms from the
click to the first decoded frame from the new channel, and the way it is reached
is by not tearing anything down. The signalling connection persists and the
media session is not renegotiated. A browser gives no advantage here and takes
none away: the peer connection object survives a track subscription change, which
is the operation a switch is made of.

A distribution story for people who are not going to install a toolchain. This is
where the browser wins by a distance. The operator sends a link. There is no
build per platform, no code signing, no notarisation, no store review, no
installer, and no update mechanism to write, because the next load is the update.
Every one of those is a permanent cost a native client pays for the life of the
project, and none of them is work this board has anybody to do.

The fifth is echo cancellation, noise suppression and automatic gain control.
Without them a laptop in a room with its speakers on feeds itself back and the
conversation is unusable, so they are not a refinement. They are also years of
signal-processing work that nobody here is going to write, and the good
implementations of all three live inside libwebrtc. A browser ships that
implementation and applies it to a capture stream on request. A native client
either links libwebrtc, which is a large C++ dependency and the single heaviest
thing this tree could take on, or ships without them.

Pion, which `docs/decisions/media-engine.md` names as what LiveKit is built on,
is deliberately not in this business: it carries the transport and leaves capture,
playback and the processing above to the caller. So a native client in Go, which
would otherwise be the cheapest native option because it stays in the server's
ecosystem, is the option that has the least to offer here.

## The alternatives and why each was not taken

A native client. It owns the audio path outright, which is the only way the
capture and playback budget lines become fully ours to hit rather than partly the
browser's to allow. It was not taken because the two costs above are both real
and both permanent: the audio processing has to come from somewhere, and the
per-platform build, signing and distribution work never ends. Neither is a
one-time cost that a first release absorbs.

A framework wrapping a browser engine, meaning Electron or Tauri. Electron ships
Chromium, so it inherits the same media stack as the browser option and adds a
native window, native distribution, and control over the flags the engine starts
with. That last part is not nothing, and it is the shape this decision is most
likely to move to. It was not taken now because it pays the whole per-platform
build and distribution cost immediately in order to buy a lever nothing here has
yet shown it needs, and because the code it runs is the same code the browser
option produces. Tauri was not taken because it uses the operating system's own
webview rather than a bundled engine, so the media stack differs per platform,
which is the one property a voice client cannot be relaxed about.

The order matters and it is the same order `docs/decisions/media-engine.md`
takes. Ship the option that reaches people, keep the option that owns the path,
and do not pay for the second before a measurement asks for it.

## Which budget lines this makes harder, and what is done about each

The figures are from `docs/decisions/latency-budget.md`, where every one of them
is marked pinned rather than measured. Nothing below has been measured in this
tree either, and each claim about what a browser exposes is a claim about an
external platform rather than a reading of anything here.

Capture to encoder output, p95 at or below 30 ms. This is the line the choice
makes hardest. A browser does not expose the capture buffer size, so the figure
is whatever the engine and the operating system settle on and the client's only
levers are the constraints it asks for on the capture track. What is done about
it: the line is measured from the client rather than assumed, which is #65, and
if it is missed the record of that measurement says so rather than the figure
moving. `docs/decisions/latency-budget.md` refuses raising a figure by name.

Decode to playback, p95 at or below 25 ms. The same shape and a second lever.
The receiver's jitter buffer target is settable from script on at least one
engine and the browsers differ in whether they honour it, so this is a line where
the client asks and the engine decides. What is done about it: #63 owns input and
device handling and #65 owns the measurement, and the first of the two that has a
figure writes it into the budget record with the engine and version it was taken
on, because a browser figure without its engine is not a figure.

Switching between channels, p95 at or below 300 ms. Not made harder by this
choice. The work is in not renegotiating, which is #60, and the property is
asserted on the signalling exchange rather than on a stopwatch.

Per-person volume, p95 at or below 100 ms. Not made harder, and arguably made
easier. `docs/decisions/per-person-volume.md` puts the gain in the client
precisely because a selective forwarding unit delivers each speaker as a separate
stream, and a browser applies a gain to one of those streams locally with no
round trip and no decode the server has to do.

Idle cost, mouth-to-ear, forwarding delay and the jitter buffer lines are the
server's and the media plane's. This choice does not reach them.

## Is this force, and is it held to its smallest surface

It is partly force and the force is named. `docs/decisions/media-engine.md`
adopted LiveKit, and the client SDKs LiveKit publishes are what a client of it
is built against rather than a protocol somebody reimplements. What those SDKs
are, and under which licence, at the moment this command was run:

    for r in livekit/client-sdk-js livekit/client-sdk-swift livekit/client-sdk-android livekit/client-sdk-flutter livekit/rust-sdks; do gh api "repos/$r" --jq '"\(.full_name) \(.license.spdx_id) \(.language)"'; done
    livekit/client-sdk-js Apache-2.0 TypeScript
    livekit/client-sdk-swift Apache-2.0 Swift
    livekit/client-sdk-android Apache-2.0 Kotlin
    livekit/client-sdk-flutter Apache-2.0 Dart
    livekit/rust-sdks Apache-2.0 Assembly

That command reads what GitHub reports at the moment it runs, so re-run it rather
than trusting the paste. The language GitHub attributes to the Rust repository is
what its detector reports and not a description of the project.

The force is real and it is narrow. It rules out writing a client in a language
with no SDK, and it says nothing about which of the five to prefer. The
JavaScript one was chosen for the reasons above and not because it was first in
the list.

Held to its smallest surface means two things here. The client is a client: it
holds no permission decision, no orchestration state and no rule that the server
does not also enforce, because a hostile client that ignores every client-side
rule is an attacker this project already assumes, which is written into
`docs/decisions/per-person-volume.md` and is #82's to model. And the toolchain
stays at build time, which is the next section.

## Does it add a runtime or a dependency the tree does not already carry

It adds a build-time toolchain the tree does not carry today, which is Node and a
bundler, and it adds a dependency graph in a second ecosystem. That is a real
cost and it is paid knowingly.

What it does not add is a runtime on the operator's machine. The output is static
files, they are compiled into the server binary, and the operator still installs
one file. This is the same test `docs/decisions/media-engine.md` applied when it
turned down mediasoup and Jitsi Videobridge, and the answer here is different
because the second runtime in those cases sat on the operator's box and this one
sits on a contributor's.

Two consequences follow and neither is optional.

The lockfile and restore work in #18 is written against Go's `go.sum` and covers
one ecosystem. A second ecosystem arriving means a second lockfile, a second
restore mode and a second proof that deleting a line reds the check. That is not
a footnote to this record; it is work #18 does not currently describe.

The licence header check refuses a tracked file whose extension it cannot
classify, which is what keeps it covering the tree. TypeScript, JSON and whatever
configuration the bundler wants are extensions it does not classify today, so the
first change that lands client files reds `Licence headers` until they are
classified. That is the check behaving correctly and it is named here so it
arrives as an expected step rather than as a surprise inside somebody's change.

## Is it testable by the harness this board plans

Partly, and the part that is not is named.

What is testable headless. Every browser engine this client would run on can be
driven headless by a browser automation tool, and the two engines that matter can
be started with a synthetic audio device and a file played into it in place of a
microphone. That is what makes the interaction tests, the participant list, the
switch and the per-person volume control runnable in a job with no display, no
device and no elevation, which is the constraint #16 exists to enforce. It is
also what makes #59 a real issue rather than an aspiration.

What is not testable that way. The audio path from a real microphone through the
operating system to the engine, and from the engine to a real speaker, is
exactly what a synthetic device replaces, so the capture and playback budget
lines cannot be measured by the headless suite. They are measured on the bench
against a real device instead, which is #65 and is gated on hardware in the same
way #45 is.

The accessibility floor in #64 is the one place this choice makes a testing story
better rather than worse. The mechanically checkable half of an accessibility
standard has established checkers for a browser interface and none for an
arbitrary native toolkit, so #64's second done-when line, an automated check that
fails on a violation, is reachable here and would have been a research project on
a native client.

None of the tests above are `go test` invocations, so this decision does add a
second runner and a second place a check reads a result from. The server-language
record claimed one runner for the suites it named, and this is the first thing on
the board that argues with that claim. It is written down rather than absorbed.

## What this decides for other issues

The signalling transport in #30 has to be reachable from a page. That rules out a
raw TCP or QUIC stream the client opens itself and leaves the transports a
browser can open, which is what #30 chooses between rather than deciding whether
a browser matters. Before this record the question was open and #30 could not
answer its own first done-when line without it.

The client logic package that #76's fifth invariant rule is written against gets
a home and a language, so the rule that refuses a direct device call can finally
name an API. It belongs to the change that first creates that package, which is
what `.github/workflows/invariants.yml` already says.

#59, #60, #62, #63, #64 and #65 are all constrained by this record and none of
them re-decides it. #58 said that in advance and it is repeated here so a reader
arriving at one of those issues does not reopen the argument inside it.

## The one command, which this record does not carry

Issue #58's fourth done-when line asks that a window open and connect to a
running server from a clean checkout with one command, and that the command be in
this record. There is no such command and this record does not invent one.

    go run .
    stammtisch: nothing is implemented yet

Nothing listens. The signalling transport is #30 and the authentication that
gates it is #28, and until both exist there is nothing for a client to connect
to. Writing a command here that a reader would run and watch fail is the defect
this board guards against, so the line stays unmet and #58 stays open on it.

## Residual risk

The two budget lines the client owns are the two this choice has the least
control over, and neither has been measured. If capture to encoder output or
decode to playback is missed once #65 measures it, the levers available are the
constraints asked of the engine and nothing more, and the next move is the
Electron option above rather than a change to the figure.

Every claim here about what a browser exposes is a claim about an external
platform, taken from its documented interfaces and not from a run in this tree.
None of it is measured, the engines differ, and they move. The first measurement
that contradicts one of these claims is worth more than the paragraph it
contradicts.

The second ecosystem is the cost most likely to be underestimated. It is one
lockfile, one restore mode, one set of licence classifications and one more
supply chain to scan, and this record names it rather than discovering it inside
the change that lands the first client file.

## When this is revisited

Two triggers, and both are numbers rather than judgements.

A capture or playback figure from #65 that misses its budget line on the engines
the project supports, confirmed by a second run on the same machine and the same
engine version. One run is noise. That trigger opens the Electron option, which
is the same client code inside an engine whose start-up flags are ours.

An accessibility finding from #64's manual walk that cannot be repaired inside a
browser interface. That one is not expected and is written down because the
opposite argument, that the browser inherits accessibility for free, is the kind
of claim that goes unchecked until somebody who needs it finds out.
