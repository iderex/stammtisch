# The repository layout

Status: decided, 2026-08-06. Raised by issue #14.

Layout becomes expensive after about the tenth file. This tree is being laid out
while it has few, so the seams the plan depends on are visible from the
directory names rather than from somebody's memory of a decision record.

## The directories

Each top-level directory carries its own README saying what belongs in it and
what does not. Those READMEs are the authority for their own directory. This
document is the authority for the boundaries between them, and for which of them
the shipped artefact is built from.

`internal/` holds the server. Nothing outside this module can import it, which
is what the name means to the Go toolchain, so anything under it can be changed
without breaking somebody else's build.

`internal/orchestration/` holds signalling, sessions, permissions, presence and
the state machines. It is importable without dragging in a media dependency, and
that is enforced rather than asked for. See the next section.

`internal/media/` holds the media plane port and, beside it, its
implementations. The port is the interface `docs/decisions/media-plane-port.md`
specifies. The two implementations do not exist yet: the fake is issue #36 and
the binding to the chosen unit is issue #40, and each gets its own directory
under this one when it lands. Keeping them apart from everything that uses them
is what lets a reviewer see at a glance whether the orchestration layer reached
around the port.

`internal/logging/` holds the one surface a log line is written through. It is a
leaf: it imports nothing else in this module, and
`TestTheLogSurfaceDependsOnNoOtherPackageInThisModule` refuses a change to that.
Every package under `internal/` has to be able to log, so a surface that
imported the domain or the store would be one neither could import back, and the
first package that needed a line would write its own instead.

`internal/store/` holds the persistence port and, beside it, its
implementations, which is the arrangement `internal/media/` uses one directory
along. The port and the in-memory implementation are the package itself and
carry no driver, so the orchestration suite can hold a real store without a
database, a file or a build tag. The durable implementation is
`internal/store/sqlite/` and is the only package in this tree that imports a
database driver. Which store, and the means check behind it, is issue #27.

`botapi/` holds the bot API surface. It sits outside `internal/` because it is a
public contract that third parties write against, and it is its own package so
its diff is readable on its own. The contract itself is issue #50 and is not in
this tree yet.

`bench/` holds the measurement rigs. Development only.

`docs/` holds the decision records and the operator documentation.

`.github/` holds the workflows and the scripts they call. Its README says what
belongs there and deliberately lists none of the checks: what each one refuses
is written at the top of its own workflow file, and a list here would be a
second place for the same answer to drift from.

## What the artefact is built from

The shipped artefact is built from the module root package, `internal/` and
`botapi/`.

`bench/`, `docs/` and `.github/` are development only. Nothing under `internal/`
or `botapi/` may import `bench/`, because that would put a measurement rig in an
operator's binary.

The entry point is still `main.go` at the module root. It belongs under a
`cmd/` directory and this change does not move it, because moving a file is not
the same job as deciding where it goes and the move has its own issue. Until
then the root package is part of the artefact and is named as such above.

## The seams that are enforced

The orchestration layer must not import the media plane. That is the property
that keeps the unit suite fast and headless, and it is the first thing that
erodes, because reaching straight for the port is always the shortest path to
whatever is being written that day.

`TestOrchestrationDoesNotReachTheMediaPlane` in
`internal/orchestration/import_graph_test.go` refuses it. The test asks the
toolchain for the dependency graph rather than reading imports out of the source,
so an import that arrives through a third package is caught the same as a direct
one, and it asks for the graph of the test binary as well as of the package, so
a media dependency that only a test carries is caught too.

Run it yourself:

    go list -deps -test ./internal/orchestration/... | grep media
    (no output)

The test fails closed. If the toolchain cannot be run, or if the graph comes
back without the orchestration package in it, the test fails rather than passing
on an empty answer.

The same layer must not import the durable store or the driver under it, and
`TestOrchestrationDoesNotReachADatabaseDriver` in
`internal/orchestration/store_seam_test.go` refuses that the same way, off the
same graph. It asserts the port is present before it asserts the two absences,
so an absence cannot be read out of a graph that carries no store at all:

    go list -deps -test ./internal/orchestration/... | grep 'internal/store'
    github.com/iderex/stammtisch/internal/store

    go list -deps -test ./internal/orchestration/... | grep 'modernc.org/sqlite'
    (no output)

The port is importable there and the implementation is not. That is what lets
the orchestration suite run against a real store on a runner with nothing
installed.

## What this does not settle

Where the entry point lives, which is the paragraph above.

How the client is laid out. The client platform is issue #58 and the client may
not be in this repository at all, so nothing here reserves a directory for it.

Whether `botapi/` stays one package. A contract with several versions in flight
may want a directory per version, and that is a question for issue #55 rather
than for this record.
