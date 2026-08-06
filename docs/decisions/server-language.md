# The server language and toolchain

Status: decided, 2026-08-06. Raised by issue #13.

## The decision

The server is written in Go. The minimum toolchain is Go 1.26.

## What the means had to carry, and how Go answers each

A single static binary an operator can drop on a box. The audience already runs
a handful of self-hosted services and does not want another runtime to keep
patched. `go build` produces one file with no shared-library dependency by
default, which is the shape that audience expects.

A real concurrency story. The orchestration layer is thousands of long-lived
connections and a fan-out problem, and the presence model in
`docs/decisions/presence-model.md` already turns one person moving into a bounded
message per client per interval. That is a scheduler and channel problem, and it
is what the language is built around.

A test story strong enough to unit test the whole signalling layer with a fake
clock and no network. The standard library ships the test runner, `testing.T`,
subtests, race detection under `go test -race`, and `net/http/httptest`. The fake
clock is ours to write and does not need a framework.

First-class access to the chosen media plane. `docs/decisions/media-engine.md`
names LiveKit, which is Apache-2.0 and Go, built on Pion, which is MIT and Go. A
server-side binding is a package import rather than a foreign function interface
or a subprocess.

A supply chain that can be locked, scanned and reproduced. The module graph is
declared in `go.mod` and, from the first dependency onwards, pinned by hash in a
`go.sum` the toolchain writes and checks on every build. There are no
dependencies yet, so that file does not exist in the tree today. Roughly half the gate this board plans is
about exactly that, and issues #18 and #19 are where it is proved rather than
claimed.

## Is this force, and is it held to its smallest surface

It is partly force and the force is named. The engine decision picked a Go unit
built on a Go library, and a server in another language would reach it through a
network API or a binding rather than an import. That argues for Go on the server
and it argues for nothing else.

It does not decide the client, which is a separate check on issue #58. It does
not decide the language of any bot library, which follows the bot API contract
and not this record. Anywhere the force does not reach, the question is asked
again.

## Does it add a runtime or a dependency the tree does not already carry

It adds one toolchain and no runtime. The tree today carries no code at all, so
this is the first language in it either way, and the honest comparison is against
the alternatives rather than against nothing.

The cost is paid knowingly and it is small. A Go binary carries its runtime
inside itself, so the operator installs nothing, and the toolchain is a build-time
dependency for contributors rather than a deployment one. What an operator gets is
one file. That is the whole point.

The alternatives were not free in the same place. A JVM or a Node runtime is a
second thing on the operator's box to install, patch and keep at a compatible
version, which is what `docs/decisions/media-engine.md` already refused when it
turned down mediasoup and Jitsi Videobridge for the same reason. Rust would
produce the same single binary and would carry the same supply-chain story, and it
was not taken because it would put the server in a different ecosystem from the
media plane it drives, which converts every engine call into a binding and makes
the door the engine decision deliberately left open harder to walk through, not
easier.

## Is it testable by the harness this board plans

Yes, and by the same one. The suites this board plans are the unit suite over the
orchestration layer, the property tests over the protocol decoder, and the media
integration harness gated on hardware. All three are `go test` invocations against
packages in this module, so there is one runner, one coverage format and one place
a check reads a result from.

Nothing here needs a parallel apparatus. The one place a second means is likely to
be forced is the bench, which measures other systems as black boxes and belongs to
issue #4, and that is a separate check on a separate artefact.

## The minimum toolchain, and why that number

Go 1.26, declared in `go.mod`.

The floor is the current release line rather than the oldest one that would
compile, because nothing in the tree has to run on an older toolchain and a lower
floor is a promise to keep working on compilers nobody here runs. Raising or
lowering it is a change to this record and not a detail of a build file.

    go version
    go version go1.26.5 windows/amd64

## The one command

From a clean checkout:

    go build .

It produces the binary in the checkout directory. Ignoring that artefact is part
of laying out the repository, which is issue #14, and this record deliberately
adds no ignore file of its own.

## Residual risk

Go was checked against what the server has to do and it was checked with the
engine decision already made, so the media plane's language is doing real work in
this argument. If that decision is revisited under the trigger
`docs/decisions/media-engine.md` names, the strongest reason here weakens, and
this record should be read again rather than assumed to survive.

Nothing about the concurrency or the supply chain claims above has been measured
in this tree. They are properties of the language and its toolchain, and the
figures that matter are the ones issues #47, #89 and #94 will produce against a
server that does something.
