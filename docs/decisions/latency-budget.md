# The latency budget

Status: decided, 2026-08-07. Raised by issue #6.

## The decision

The budget is a set of per-feature thresholds rather than one end-to-end number,
because a single figure tells nobody which segment regressed. Each line below is
a segment that can be measured on its own, carries one figure, says whether that
figure was measured or pinned from a standard, and names the command that
measures it.

Every line in this record is pinned. Not one of them has been measured on this
project's software, and the section after the table says why that is the honest
state rather than an oversight.

## What measured and pinned mean here

Measured means a bench run against the thing the line is about produced the
figure, and the run is named.

Pinned means the figure comes from a standard, from frame arithmetic, or from a
design decision already recorded, and the bench has not contradicted it because
the bench has not been pointed at anything that could. A pinned figure is a
target somebody chose. It is not evidence.

The distinction is the whole point of the record. A budget that does not say
which of its numbers were observed reads, six months from now, as though all of
them were.

## The lines

### Mouth-to-ear on the reference path

Same region, wired, 48 kHz Opus at 20 ms frames. p95 at or below 150 ms, with a
working goal of p50 at or below 90 ms.

Pinned. The 150 ms is the ITU-T G.114 bound for one-way transmission time in
speech, the figure below which most applications need no special consideration.
The 90 ms working goal is this project's own choice and rests on no standard.

    go run ./bench/cmd/mouth-to-ear --out mouth-to-ear.json

That command runs today and measures the rig's own fixture. It measures this
project's software on the day an implementation of `bench.System` reaches a real
audio path, which does not exist in this tree.

### Capture to encoder output

p95 at or below 30 ms.

Pinned, from frame arithmetic: one 20 ms Opus frame plus a device buffer of the
same order. Nothing in this repository captures audio, so there is no reference
run and no measured device buffer behind it.

    the command does not exist

There is no client and no capture path to attach a probe to. #63 is where input
handling lands and #65 is where the client is measured against the lines it
owns; the command belongs to those and not to this record.

### Forwarding delay through the media plane

Ingress packet to egress packet against one clock. p99 at or below 5 ms.

Pinned. `docs/decisions/media-engine.md` already treats this line as the revisit
trigger for the choice of selective forwarding unit, so the figure predates this
record and is unchanged by it.

    the command does not exist

#46 is where forwarding delay is measured against this line. The adapter it
would measure is #40 and is not in the tree.

### Jitter buffer target delay

p95 at or below 40 ms with no loss, and at or below 80 ms at 2 percent random
loss.

Pinned, from the same frame arithmetic: two 20 ms frames of nominal depth, and
double that once the buffer has to cover a loss pattern.

    the command does not exist

The loss and bandwidth characterisation is #48. The rig applies no network
shaping and verifies none, which its own report says in the object it carries the
profile in, so the loss half of this line has no instrument today either.

### Decode to playback

p95 at or below 25 ms.

Pinned, from one frame plus an output buffer.

    the command does not exist

Same reason as capture to encoder output: no client, no playback path.

### Switching from one voice channel to another

From the click to the first decoded frame of audio from the new channel. p95 at
or below 300 ms.

Pinned. This is the line the product is most about and it is the one with the
least evidence behind it. `docs/decisions/nearest-projects-survey.md` records
that Mumble's design makes this interval a message round trip, and that if the
design taken here cannot get near it, the reading is that the incumbent design
was better on this feature rather than that 300 ms was always the target. That
sentence is the reason this line is not raised when it is missed.

    the command does not exist

#60 is where switching lands and #65 is where it is measured.

### Joining a room from cold

From the click to the first decoded frame. p95 at or below 800 ms.

Pinned.

    the command does not exist

This line also has a measurement problem the bench has already produced, and it
is in the section below.

### Participant list staleness

For a channel the viewer has not joined, from the event to the list being
correct. p95 at or below 2 s.

Pinned. `docs/decisions/presence-model.md` designs the 500 ms coalescing interval
to sit inside this bound with margin for the queue and the network, and says in
the same place that until this record exists the 2 s is a number that record is
designed against rather than a threshold anything is held to. This record is
that record, and the line is still pinned rather than measured.

    the command does not exist

#35 is the presence fan-out projection and #94 is the load evidence.

### Per-person volume

From the control moving to the change being audible. p95 at or below 100 ms.

Pinned. `docs/decisions/per-person-volume.md` puts the gain at the port boundary
precisely so this needs no round trip, which is what makes 100 ms achievable at
all.

    the command does not exist

#43 settles the gain at the port boundary and #62 is the client control.

### Idle cost of an always-on room

One thousand rooms with no members held for one hour, under 200 MB resident and
under 2 percent of one core.

Pinned. `docs/decisions/channel-and-room-model.md` names the same shape.

    the command does not exist

#47 is where the idle cost is measured. This is the one line in the budget that
is not a latency at all, and it is here because it is the cost of the feature the
latency lines are about.

## Whether the bench moved any figure

No figure in this record moved from its provisional value, and the reason is that
the bench has not yet measured anything a figure is about.

The rig is real and proved. It tracks a known synthetic delay of 50 ms and of
200 ms to the sample, it reports a distribution rather than an average, and two
passes over an unchanged system agree on p95 inside 10 ms. Every one of those
figures is a figure about the instrument. The only implementation of
`bench.System` in the tree is `DelayLine`, a fixture whose whole behaviour is a
delay, and `bench/README.md` says so in its own words.

So an instrument that has been pointed only at a known delay cannot contradict a
target taken from G.114 or from frame arithmetic. It has nothing to contradict it
with. That is why no figure moved, and it is a statement about what has been run
rather than a claim that the provisional figures were right.

What the bench did produce is a constraint on the measurement rather than on the
product, and it belongs in the budget because two lines here sit against it. The
rig's trial period is 500 ms, which puts its ceiling at 460 ms:

    go run ./bench/cmd/mouth-to-ear --delay 50ms --repeat=false
    "trial_period_ms": 500,
    "max_measurable_delay_ms": 460,

A delay at or above that ceiling is not reported as large. It is reported wrongly,
because the arriving sweep has crossed into the next trial's window. A 600 ms
delay comes back as 100 ms:

    go run ./bench/cmd/mouth-to-ear --delay 600ms --repeat=false --out /dev/null
    pass 1: 299/300 chirps detected, 1 missed, p50 100.000 ms, p95 100.000 ms, p99 100.000 ms
    mouth-to-ear: pass 1 detected 299 chirps, below the 300 required
    exit status 1

The trial period is compiled in and no flag changes it:

    go run ./bench/cmd/mouth-to-ear --help
    (the flag list carries -delay, -jitter, -min-samples, -noise, -out, -raw,
    -repeat, -seed, -shaping and -trials, and no trial period)

The cold-join line is 800 ms and the participant list staleness line is 2 s. Both
are above the ceiling, so the rig as committed cannot measure either one, and a
run against a system at those delays would return a number that looks plausible
and is wrong. That is the sharpest thing the bench has said so far, and it is
about the rig. Widening the trial period, or making it a flag, is the change that
retires it, and it belongs to the issue that first needs one of those two lines
measured rather than to this record.

## What happens when a figure is missed

The figure stays and the work is done, or the line is removed from the budget
because the feature it belongs to was dropped. Raising a figure because the
implementation missed it is the one answer this record refuses, and the reason is
in `docs/decisions/nearest-projects-survey.md`: an existing project already
reaches the switch line by design, so a number this project cannot hit is
evidence about this project's design rather than evidence that the number was
wrong.

A missed figure that has been measured twice on the same profile and the same
commit is a defect with an issue, and that issue carries the two runs. One run is
noise; `docs/decisions/media-engine.md` uses the same two-run rule for its own
revisit trigger and for the same reason.

Changing a figure in this record is a decision somebody argues for in its own
issue, with the measurement in front of them, and the record says what it was
before and why it moved. It is not something that happens as part of the change
that missed it.

## What this record does not do

It measures nothing. Every command above that runs today runs against the rig's
own fixture, and every other command is named as not existing. A reader looking
for evidence that this project is fast will not find it here, and should not
infer it from the record's existence.

It pins no figure that a check refuses. Nothing in the tree reads this document,
no test compares a run against a line in it, and there is no route by which a
regression past one of these numbers reds a gate. The lines are targets in a
document, which is the state the three rules in `CONTRIBUTING.md` call an
explanation of a rule rather than a rule.

It does not cover the segments the incumbent's own numbers are quoted against.
The figures in circulation for the commercial products come from comparison
articles that publish no method, and this record deliberately takes nothing from
them. #5 is where the incumbent and the nearest open projects are measured on the
bench, on the same terms as this project's own software.
