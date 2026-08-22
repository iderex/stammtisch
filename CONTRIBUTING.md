# Contributing

This repository refuses some things outright and asks for others without any way
of refusing them. The difference matters more than the rules do, so every rule
below says which of the two it is. Where nothing refuses a violation, the rule is
marked `NOT ENFORCED` and carries the issue that owes the mechanism, or says that
no mechanism is owed because there is nothing in a tree for a check to read. A
rule with that mark is still a rule. It is just one that a person has to catch.

A rule can be half of each. When a mechanism lands it
usually covers one clause of a sentence and not the rest, so the mark splits
rather than disappearing: the enforced half names the check, and the half that is
still nobody's mechanism stays written down as such. A paragraph that lost its
mark entirely when a check landed would be claiming cover the check does not
give.

## Getting a change in

Every change starts as an issue and lands as a pull request.

    git clone https://github.com/iderex/stammtisch.git
    cd stammtisch
    git switch -c topic/short-name

REFUSED BY A CHECK for the second half of that sentence. The
`Deterministic PR-hygiene checks` job in `.github/workflows/pr-hygiene.yml` reads
the commit messages and the body for `#<number>` references, resolves each one
against this repository, and refuses a change where none of them resolves to an
issue. A number that resolves to a pull request is reported and does not count.
The job fails closed: a reference it cannot read for any reason other than a 404
reds the check rather than passing as absent.

`NOT ENFORCED` for the first half, and no issue owes a mechanism for it. Nothing
reads whether the issue says what is wrong, what the evidence is and what done
means, and nothing can tell an issue opened before the work from one opened to
justify a branch that already exists. Both are judgements about meaning, and the
review is where a bad one is caught.

Direct pushes to `main` are refused, by an active ruleset with no bypass actors.
That one you can read for yourself:

    gh api repos/iderex/stammtisch/rulesets --jq '.[] | "\(.id) \(.name) \(.target) \(.enforcement)"'
    gh api repos/iderex/stammtisch/rulesets/20482339 --jq '{enforcement, bypass: .bypass_actors, types: [.rules[].type]}'

The `pull_request` rule in that output is what makes a push to `main` fail. The
empty `bypass` list is what makes it apply to everybody, including whoever owns
the repository.

Read that output rather than trusting this paragraph. A ruleset is settings and
settings move, and this file cannot tell you when they have.

## Sign your work

Every commit needs a `Signed-off-by` trailer matching its author. That is the
Developer Certificate of Origin, and `git` writes the trailer for you:

    git commit -s

REFUSED BY A CHECK. The `DCO sign-off` job in `.github/workflows/dco.yml` walks
every non-merge commit in the pull request and reds the check on the first one
without a matching trailer. It compares the trailer against the commit's own
author name and email, so a trailer naming somebody else does not pass.

You can run the same comparison before you push:

    for sha in $(git rev-list --no-merges origin/main..HEAD); do git show -s --format='%B' "$sha" | grep -qxF "Signed-off-by: $(git show -s --format='%an <%ae>' "$sha")" && echo "ok    $sha" || echo "FAIL  $sha"; done

If a commit already exists without the trailer, add it to the whole branch with
`git rebase --signoff origin/main` rather than by hand.

The file the certificate itself lives in is not in the tree yet. Issue #21 owes
it. Until it lands, the trailer is a reference to the standard text of the
Developer Certificate of Origin 1.1 and this sentence is the whole disclosure.

## What runs on a pull request

What each workflow refuses is in its own file under `.github/workflows/`, and
reading the file is the only way to know what it covers today. How many there
are is what the listing prints rather than a number written here, which is a
number that goes wrong the first time one lands:

    ls .github/workflows/
    gh pr checks <number>

There is no local gate on this board and no single verb that runs the whole set
on your machine. Some of them have local equivalents you can run yourself: the
sign-off loop above, the Unicode scan below, and the licence header scan, which
is the same script the workflow runs. The rest run on GitHub or not at all.

REFUSED BY THE RULESET, for most of them. It carries a
`required_status_checks` rule and the contexts on it have to be green before the
forge will merge, with no bypass actors, so this applies to whoever owns the
repository as well. Read the list rather than trusting a count here, because a
name goes on it as each check starts reporting and this paragraph cannot tell
you when one has:

    gh api repos/iderex/stammtisch/rulesets/20482339 --jq '[.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[] | .context] | sort | .[]'

Each entry also pins the app that may satisfy it, so a check run of the right
name from another app does not count. `CodeQL` is why: the job and the
code-scanning result share one name, and only the job is required.

    gh api repos/iderex/stammtisch/rulesets/20482339 --jq '[.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[] | .integration_id] | unique'

`NOT ENFORCED` for the rest, and the rest is not nothing. A workflow that runs
on a schedule and never on a pull request cannot be required, because a name
that never reports would block every merge, so those stay advisory by
construction. Which ones those are is readable in the `on:` block of each file
and nowhere else. Compare the two lists yourself rather than assuming they
match:

    ls .github/workflows/
    gh pr checks <number>

A branch does not have to be up to date with the default branch before it
merges. `strict_required_status_checks_policy` is false, so a green run on a
head cut from an older base still counts.

## Unicode

Tracked text may not carry bidirectional or invisible Unicode control characters.
Those let source render differently from how it executes, which is the Trojan
Source attack, CVE-2021-42574. Accented letters, em dashes and every other benign
non-ASCII character are fine and are not in the set.

REFUSED BY A CHECK. The `Reject Trojan Source Unicode` job in
`.github/workflows/unicode-guard.yml` scans the tracked tree on every push and
every pull request, and it fails closed: a scanner error reds the check rather
than passing as clean. Run the same scan yourself:

    git grep -nIP '(*UTF)[\x{202A}-\x{202E}\x{2066}-\x{2069}\x{200E}\x{200F}\x{061C}\x{200B}-\x{200D}\x{2060}]' -- . ; echo "exit=$?"

Exit 1 is a clean tree. Exit 0 means a match was found and names it. Anything
else is the scanner failing, which the workflow treats as a failure and so should
you.

## Licence headers

Every tracked source file starts with the two lines that say what it is under:

    // SPDX-License-Identifier: AGPL-3.0-or-later
    // Copyright (C) 2026  Nils Lehnen

In a shell script the prefix is `#` and the two lines come after the shebang.

The wording is not invented here. The appendix of `LICENSE` asks for the notice
at the start of each source file, and for each file to carry at least the
copyright line and a pointer to where the full notice is found. The identifier
is that pointer. It carries the "or later" arm because the appendix, as this
repository filled it in, offers that arm; if that was not intended, it is one
string in the script and one line per source file, and #79 is where to say so.

REFUSED BY A CHECK. The `Licence headers` job in
`.github/workflows/licence-header.yml` runs `.github/check-licence-headers.sh`.
It refuses a source file without the header, and it also refuses a tracked file
whose extension it cannot classify. The second half is why the rule keeps
covering the tree: a header check that only knows the file types the tree holds
today stops covering it on the day somebody adds a language, and says nothing.
Here an unknown extension is a red check asking to be classified.

Run the same script yourself. It reports what it proved before it reports on
the tree:

    sh .github/check-licence-headers.sh

That proof is not a branch somebody has to keep alive. The script builds faulty
fixtures in a temporary directory, scans them, and refuses to report on the tree
at all unless each one was refused and named. So a run saying the tree is clean
is a run in which the scan was shown able to say otherwise.

## What a change carries

One topic per pull request, and a body that says what was wrong, what the change
does about it, and how you know it works.

Where the body asserts a fact, it carries the command that produced it, run
against the branch as it will be reviewed rather than against your working tree.
A number without its command is a number a reader has to take on trust, and this
repository has enough of those already in the issues it inherited.

Before you push, look at what you actually touched:

    git diff --name-only origin/main...HEAD

REFUSED BY A CHECK for the paths. The `Deterministic PR-hygiene checks` job
reads the body, takes the `Scope:` line out of every issue the change names, and
refuses a change touching a path outside every one of them. `Scope:` is at
column zero and the rest of the line is one path; a comma separates two.

`NOT ENFORCED` for three halves of it, and they are different from each other.

Where no referenced issue carries a `Scope:` line, the comparison is not made at
all. The job says so in the log in those words and passes, so a change that
declares no scope anywhere gets no path check rather than a refusal. That is the
default and it fails open.

Whether the work belongs to the issue it names, as opposed to merely landing in
the same paths, is not read. A second unrelated topic inside one declared scope
passes.

Whether a commit message or a body says anything is not read. The job takes one
thing out of a message, the issue reference, and judges nothing else: not whether
the message says what changed, not whether the body says how you know it works,
and not whether a number in it carries the command that produced it.

## Signatures

`NOT ENFORCED`, issue #78. The ruleset carries no `required_signatures` rule, so
an unsigned commit merges exactly like a signed one and the output above is where
you can confirm that. Signing your commits is worth doing and nothing here will
notice if you do not.

## Comments and review

Everything about a change goes in its body. If the body is wrong, incomplete or
out of date, edit the body rather than adding a comment underneath it. That
includes the reason a change is sent back.

`NOT ENFORCED`, and no issue owes a mechanism for it, because there is nothing in
a tree for a check to read. This one is held by people or not at all.

## Style

English, in every tracked file and every issue and pull request body on this
board.

No attribution to a tool, no generated-by marker, and no note about what produced
a change, in anything tracked. Write about the change.

`NOT ENFORCED`. Nothing scans tracked text for that class of marker today.

## If a rule is wrong

Open an issue and argue with it there. A rule that survives an argument is worth
more than one nobody has tested, and this file has no rule in it that is older
than the repository.
