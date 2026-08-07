# Governance

This document describes what exists. A governance document describing a
committee that has not been assembled is worse than one saying a single person
decides, because the first is a promise nobody can hold anyone to.

## Who decides

One maintainer, [@iderex](https://github.com/iderex), who owns this repository
and has the final say on every change in it. There is no steering group, no
technical committee and no vote.

That is not a permanent arrangement so much as an accurate one. If the project
gains regular contributors, this file changes to say what the arrangement then
is, and the change lands as a pull request like anything else.

## How a decision is proposed

On the issue tracker, before the code that depends on it exists.

An issue that proposes a decision says what the options are, what each one
costs, and what evidence would settle it. Where the evidence is a number, it
carries the command that produced it. An issue that names no options and no
cost is a preference rather than a proposal, and it is answered as one.

Decisions that shape the architecture are recorded under `docs/decisions/` once
they are made, so that somebody arriving later reads the reasoning rather than
the outcome.

Some decisions are the maintainer's alone and are parked in one place rather
than assumed one at a time. That is
[issue #1](https://github.com/iderex/stammtisch/issues/1), which states the
options for each and no recommendation. Work blocked on one of those entries
says which entry it waits on and stops there rather than guessing.

## How a change lands

Every change starts as an issue and lands as a pull request. Direct pushes to
`main` are refused by an active ruleset with no bypass actors, which applies to
the maintainer as well. `CONTRIBUTING.md` carries the detail and, for each rule,
whether anything actually refuses a violation.

## How a disagreement is resolved

Argue it on the issue, in writing, where the argument stays readable after
everyone involved has forgotten it. A rule that survives an argument is worth
more than one nobody has tested.

If the argument does not converge, the maintainer decides and says why in the
issue. Nothing here can appeal that. Anyone who wants a different answer badly
enough has the licence to fork, which is the honest description of what recourse
exists in a single-maintainer project and is why this section is short.

## Behaviour

`CODE_OF_CONDUCT.md`, with a contact that reaches a person.

The same person receives conduct reports and decides technical questions, which
is a real weakness of a project this size rather than an oversight. It is
written here so that somebody deciding whether to report something knows it
before they write, rather than afterwards.

## Security reports

Not here. `SECURITY.md` has the private route, and an exploitable vulnerability
does not belong in a public issue.
