# Security policy

This software carries other people's conversations. A vulnerability in it is a
vulnerability in something a community treats as private, so a report gets
priority over every other kind of work.

There is nothing to attack yet. This repository holds decisions, a gate and a
binary that prints one line, and no part of it terminates a connection. The
policy exists now anyway, because the alternative is that the first person to
find something has nowhere to send it and opens a public issue, which is the
worst available outcome for everyone running the software.

## Reporting a vulnerability

Report it privately, through GitHub's private vulnerability reporting:
[Report a vulnerability](https://github.com/iderex/stammtisch/security/advisories/new),
which is also the button on the Security tab of this repository. It is enabled,
and you can check that rather than take this sentence for it:

    gh api repos/iderex/stammtisch/private-vulnerability-reporting
    {"enabled":true}

Please do not open a public issue for something exploitable. A public issue is
how it gets used before it gets fixed.

What helps, in rough order of how much it helps: the version or commit, what an
attacker gets, the smallest sequence that reproduces it, and whether you have
told anybody else. A report with none of that is still worth sending.

If a language model helped you find it, check it by hand before sending. A
plausible-looking report that turns out to describe code that does not exist
costs the same time as a real one.

## What to expect

These are numbers this project can meet at its current size, which is one
person working on it in their own time. They are an intent applied
consistently and not a contractual service level, and a modest promise kept is
worth more here than a fast one broken.

- An acknowledgement that the report arrived, within five working days.
- An assessment of whether it is exploitable and how badly, within fifteen
  working days of the acknowledgement, or an explanation of what is taking
  longer.
- A fix released as soon as it is ready. Security fixes are not batched behind
  feature work and are not held for a release date.
- Coordinated disclosure. An advisory is published once a fix is available, and
  it names the reporter unless the reporter asks otherwise.
- Ninety days from the acknowledgement, an advisory is published whether or not
  a fix exists, describing the problem and any mitigation an operator can apply.
  A vulnerability that cannot be fixed quickly is still one operators are
  entitled to know about.

If a report gets no acknowledgement inside five working days, assume it did not
arrive and send it again.

## What is in scope

The software in this repository, and the defaults an operator gets when they
deploy it. A default that is unsafe is a vulnerability in this software, not a
mistake by the person who accepted it.

Also in scope: this repository's own supply chain, meaning the workflows, the
actions they pin and the dependencies the build resolves.

Out of scope: an operator's own configuration once they have changed it away
from the defaults, their infrastructure, and their choice of where to run it.
Findings there belong to that operator, and this project cannot fix them.

Also out of scope: a report that a check listed in `CONTRIBUTING.md` as not
enforced is in fact not enforced. Those are already written down as gaps, each
with the issue that owes the mechanism. Send them as issues rather than as
advisories.

## Supported versions

None yet. There has been no release, so there is no version line to patch and
nothing to state a support window for. When the first release is cut, this
section says which lines get security fixes and for how long. Issue
[#91](https://github.com/iderex/stammtisch/issues/91) is where the version
policy is decided and
[#92](https://github.com/iderex/stammtisch/issues/92) is where the first release
is cut.

## How an advisory is published

A draft advisory is opened on this repository from the report, before the fix
lands, so that the fix and the description of what it fixes are written
together rather than reconstructed afterwards.

The maintainer publishes it. There is one maintainer, so there is no second
person to hold the timing to, and `GOVERNANCE.md` says the same thing about
every other decision here.

An advisory carries what the problem was, which versions were affected, what an
operator should do, and what an operator who cannot upgrade immediately can do
instead. A CVE is requested through GitHub where the problem affects anybody
other than this project.

## What this policy does not do

Nothing in this repository refuses a violation of it. No check reads a report,
measures a response time or notices a missed one. It is held by a person or not
at all, and it is written down so that a missed promise is visible rather than
deniable.
