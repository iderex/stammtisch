# The accessibility floor

Status: decided, 2026-08-08. Raised by issue #64.

## The decision

The floor is WCAG 2.2 at level AA, for every part of the client a person uses to
join a conversation, hear it, and be heard in it.

It is a floor and not a target. A change that would take the client below it is
refused rather than logged, and a change that goes above it needs no permission.

## Why a named level rather than an aspiration

"Accessible" is a word every project applies to itself. A level of a named
standard is a thing a person can hold the client to without arguing about what
the word meant, and it is the form the audience for this project already works
in: an operator in a German public body procures against BITV 2.0, which takes
its requirements from EN 301 549, which takes the web ones from WCAG. Naming
WCAG 2.2 AA is naming the thing all three end at.

AA rather than AAA. AAA carries requirements that a real-time voice interface
cannot meet without becoming a different product, sign language interpretation
of live audio being the clearest of them, and a floor nobody can stand on is a
floor nobody checks. AA is the level the procurement rules above actually
require, and it is the one this client will be measured against by somebody who
did not read this file.

2.2 rather than 2.1 because it is the current recommendation and its additions
are exactly the ones a control-dense interface trips over: a focus indicator
that is not obscured, a target big enough to hit, and no dragging-only
interaction.

## What the floor means for the parts this product is actually about

Four things carry most of the risk here, and they are named so a reviewer does
not have to derive them from a standard.

Every control reachable and operable by keyboard alone, with the focus visible
at all times and the order matching the visual order. Push to talk is the one
that will be got wrong: a key that is held is not a control a screen reader user
can discover, so the same function has a toggle, and the toggle is the one the
interface documents rather than a fallback nobody mentions.

Speaking state conveyed without relying on colour. A ring that changes hue
around a name tells a person with a colour vision deficiency nothing, and it
tells a screen reader user nothing at all. The state carries a text equivalent
that assistive technology announces, and the announcement is rate limited,
because a room of forty people announcing every start and stop of speech is a
denial of service on the person listening to it.

The participant list readable as a list rather than as a picture of one.
Presence is the feature this product sells alongside switching, and a list that
updates without a repaint is worth nothing if the update is invisible to the
thing reading the page aloud.

Contrast at the level the standard states, including the states people forget:
focus, hover, disabled, and the speaking indicator against every background it
sits on.

## What is mechanically checkable and what is not

Roughly a third of WCAG's criteria can be decided by a machine looking at a
rendered page, and the rest need a person. That split is not this project's
finding and it is not a reason to check less; it is the reason the floor is held
in two places.

The mechanical half is an automated check in the workflow, over the rendered
client, failing on a violation. It runs on the same headless browser the client
tests use, so it needs no display and no device.

The half a machine cannot decide is walked by hand against a written list, and
the record of that walk says who walked it, when, what they found and what was
done. A walk with no name on it is a claim, not a record.

Neither half exists yet. The client does not exist, which is the last done-when
line of #58, and both halves belong to the change that builds it.

## The one thing this floor does not cover

The audio itself. Nothing in WCAG says anything useful about whether a
conversation is intelligible to somebody with a hearing impairment, and the
things that would help there are a different kind of work: per-person volume,
which is decided in `docs/decisions/per-person-volume.md` and is a comfort
control rather than an accessibility one, and captioning, which needs speech
recognition and is nowhere on this board.

Saying so is the point. A floor that quietly implies it covers the hearing case
is worse than one that names what it leaves out, because somebody will read the
first as a promise.

## Residual risk

Everything above is a statement about a client that does not exist. It
constrains the change that builds one and it has been measured against nothing,
so the first walk is where any of it is tested.

The rate limiting on speaking announcements is the part most likely to be wrong
in both directions. Too slow and the announcement is about a person who has
stopped talking; too fast and it is unusable. There is no figure here because
there is nothing to measure, and picking one now would be a number nobody
derived.

## When this is revisited

A finding from the manual walk that cannot be repaired inside the interface as
built, or a WCAG version that supersedes 2.2 and changes a criterion this
document names. A new version on its own is not a trigger; a changed criterion
is.
