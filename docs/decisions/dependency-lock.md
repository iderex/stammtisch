# Locking the dependency graph

Status: decided, 2026-08-08. Raised by issue #18.

## What the lockfile is here

`go.sum`. It carries a cryptographic hash per module version, and the toolchain
refuses to build against a module whose contents do not match the hash it holds.
There is no second file and no separate lock format to adopt: in this module
system the manifest and the lockfile are `go.mod` and `go.sum`, and the restore
that fails when the lockfile would change is the toolchain's default rather than
a mode somebody has to select.

## There is nothing in this tree to lock yet

This is the part a reader has to have before the rest of the record means
anything. The graph is empty:

    go list -m all
    github.com/iderex/stammtisch

    git ls-files go.sum | wc -l
    0

One module, which is this one, and no lockfile, because a module with no
third-party dependencies has nothing to write into one. `go mod tidy` does not
create an empty `go.sum` and this record does not create one by hand either: a
file committed to satisfy a sentence would be a file the next `go mod tidy`
argues with.

So the four legs below refuse nothing in this tree today. They are written now
because the first dependency will arrive inside somebody else's change, and a
lock gate that lands after the dependency it was meant to lock has already
missed the only moment it mattered.

## What refuses what

Four legs, in `.github/workflows/dependency-lock.yml`, stopping at the first
failure.

`sh .github/check-dependency-lock.sh` refuses a module version named in `go.mod`
that `go.sum` does not pin. It also refuses a `replace` directive outright,
because a replacement can move the obligation to another module or remove it
entirely by pointing at a directory, and a check that guessed which would be
wrong quietly.

`go mod tidy -diff` refuses a manifest or a lockfile that is not what resolving
the graph would produce. The flag is what makes the leg safe in a gate: plain
`go mod tidy` rewrites both files and exits zero, so a job running it would
report a tidy tree it had just tidied.

`go mod verify` refuses a module in the cache whose contents no longer hash to
the pin. That is the tamper leg rather than the drift leg, and it is the one that
catches a dependency whose bytes changed under an unchanged version.

`go build -mod=readonly ./...` refuses a build that would have to edit `go.mod`.
`-mod=readonly` is already the toolchain's default and is written out anyway,
because a default can be moved by a `GOFLAGS` export in a later step or in a
runner image, and a gate resting on a default it does not state can be turned off
from outside the file that declares it.

## What is proven and what is not

Only the first leg is proven here, and it is proven the way the licence header
check is: the script builds faulty fixture modules in a temporary directory,
runs the same code over them, and refuses to report on the tree at all unless
every faulty one was caught and every correct one was not.

    sh .github/check-dependency-lock.sh
    proved accepted: a require with its go.mod pin present
    proved accepted: the block form with an indirect comment, both pinned
    proved accepted: a module with no requires and no go.sum
    proved refusable: a require in a module with no go.sum at all
    proved refusable: a require whose go.sum pins a different module
    proved refusable: a require whose go.mod pin line was deleted
    proved refusable: a require whose version moved past the pin
    proved refusable: a require whose pin belongs to a longer module path
    proved refusable: a manifest carrying a replace directive

    judging .
    every module required by go.mod is pinned in go.sum

The fourth refusal is the deleted line the issue asks to be shown, and it is the
shape a bad merge leaves rather than an obvious emptying of the file: the
`/go.mod` pin removed and the content hash left behind.

The last one but two is the near-miss the effort went into. `example.com/a` is a
prefix of `example.com/ab`, so a comparison written as a substring reports the
shorter module as locked when only the longer one is pinned. That is the mistake
somebody makes while writing the comparison rather than one a reviewer would
spot, which is why the fixture exists and why the comparison is on whole fields.

The other three legs are the toolchain's own refusals and nothing in this
repository proves they bite. They are not fixtures anybody here wrote and they
are not tested here. That is stated rather than glossed, because a workflow with
four steps reads as four things this repository stands behind, and it is one.

## The update procedure

Locking that has no update path becomes never updating, which trades a graph that
drifts for a graph that rots. The procedure is three commands and a reading:

    go get <module>@<version>
    go mod tidy
    sh .github/check-dependency-lock.sh

`go get` moves the version and writes the new pins. `go mod tidy` removes what is
no longer reachable, which is the step people skip and the reason lockfiles
accumulate entries nobody can account for. The script is run before pushing
rather than after, so a bad update is caught on the machine that made it.

The update lands as its own change, on its own issue, and never inside a change
that is about something else. A dependency bump hidden in a feature branch is a
supply chain change nobody reviewed as one.

`dependency-review.yml` is what reads the update for known vulnerabilities, and
it fails on low severity and above. That job and these legs are opposite halves:
it judges what changed and cannot see drift, and these judge drift and know
nothing about vulnerability.

## Means

Shell, and it is the same shell the licence header check is written in.

The check reads two files and compares fields. It needs no state, no library and
no build step, it runs identically on a workstation and on the runner, and the
one tool it does need, `go`, is the tool whose files it is reading. Writing it in
Go would mean a package that exists to be run by a workflow rather than to be
part of the server, and a second thing to build before the gate can judge
anything. What Go would buy is a test file, and the script already carries its
proof in the shape this tree uses for the same job.

Where the reasoning goes the other way, it says so: the manifest is parsed by
`go mod edit -json` rather than by a pattern over `go.mod`, because the block
form, the single-line form and an `// indirect` comment are three shapes of one
thing and a pattern would have to know all three. A manifest the toolchain
refuses to parse is a refusal here and not an empty list, which is the direction
that would otherwise fail open.

## Residual risk

The gate is not required. No status check on this repository is required, which
`CONTRIBUTING.md` states and issue #25 is where that is asked for, so a red
Dependency lock does not by itself stop a merge.

The first leg reads the manifest and the lockfile and never the module cache, so
it says nothing about what was actually downloaded. `go mod verify` covers that
and is unproven here.

Nothing checks that a dependency was chosen deliberately. All four legs are about
a graph matching what the tree declares, and a tree can declare a dependency
nobody should have taken. That is a review question and there is no mechanism for
it here.

The `replace` refusal will one day be in somebody's way, and when it is, the
repair is to teach the script the shape rather than to remove the refusal. A
replacement pointing at a directory has no pin to check and one pointing at
another module moves the obligation to that module, and both are readable rules.
They are not written now because writing a rule for a shape the tree has never
carried is guessing at the case that will actually arrive.
