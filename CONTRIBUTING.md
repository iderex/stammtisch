# Contributing

This repository refuses some things outright and asks for others without any way
of refusing them. The difference matters more than the rules do, so every rule
below says which of the two it is. Where nothing refuses a violation, the rule is
marked `NOT ENFORCED` and carries the issue that owes the mechanism. A rule with
that mark is still a rule. It is just one that a person has to catch.

## Getting a change in

Every change starts as an issue and lands as a pull request.

    git clone https://github.com/iderex/stammtisch.git
    cd stammtisch
    git switch -c topic/short-name

`NOT ENFORCED`, issue #77. Nothing today reads a pull request for an issue
reference or refuses one that names none.

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

Five workflows. What each one refuses is in its own file under
`.github/workflows/`, and reading the file is the only way to know what it
covers today:

    ls .github/workflows/
    gh pr checks <number>

There is no local gate on this board. Nothing here builds, because there is no
code yet, and there is no single verb that runs the whole set on your machine.
Two of the five have local equivalents you can run yourself, and they are the
sign-off loop above and the Unicode scan below. The other three run on GitHub or
not at all.

`NOT ENFORCED`, issue #25. None of the five is a required status check, so a red
check does not by itself block a merge. The `types` list in the ruleset output
above is where you can see that for yourself: it carries no
`required_status_checks` entry. Wait for the checks and merge on green anyway.

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

## What a change carries

One topic per pull request, and a body that says what was wrong, what the change
does about it, and how you know it works.

Where the body asserts a fact, it carries the command that produced it, run
against the branch as it will be reviewed rather than against your working tree.
A number without its command is a number a reader has to take on trust, and this
repository has enough of those already in the issues it inherited.

Before you push, look at what you actually touched:

    git diff --name-only origin/main...HEAD

`NOT ENFORCED`, issue #77. Nothing reads a pull request body, compares changed
paths against a declared scope, or judges whether a commit message says anything.
The check that would do the mechanical half of this is owed there, and it will
never do the other half, which is whether the reasoning is any good.

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
