# bench

The measurement rigs. Development only: nothing here is built into the shipped
binary, and nothing the server imports may import it.

## What the mouth-to-ear rig measures

One-way delay from a signal entering a sending endpoint to the same signal
leaving a receiving endpoint, for a system it treats as a black box. That is the
only shape of instrument that measures this project's software and somebody
else's on the same terms, so the black box is the point rather than a
convenience.

A known sweep goes into the sending port at a known sample index. The receiving
port is cross-correlated against the same sweep, and the offset of the
correlation peak is the delay. Both ports are driven by one clock, which is what
makes the two indices directly comparable and why there is no clock skew
estimate anywhere in the rig.

It reports a distribution and not an average. The figure a person in a
conversation notices is the tail, and an average hides it.

## Running it

    go run ./bench/cmd/mouth-to-ear --delay 50ms --out report.json

The command exits non-zero when a pass detects fewer chirps than `--min-samples`
requires, or when two passes over one system disagree on p95 by more than 10 ms.
A pass that measured nothing and a pass that measured zero delay are different
results and the report shows them differently: percentiles over an empty sample
are `null` rather than `0`.

## What it does not do, today

There is no implementation of `bench.System` that reaches a real audio path.
The only system in the tree is `DelayLine`, a fixture whose whole behaviour is a
delay, and it exists so the rig can be pointed at a delay somebody already knows
and asked whether it reports that delay back. So every figure this rig has
produced so far is a figure about the instrument. None of it is a measurement of
this project's software, and none of it is a measurement of anybody else's.

The rig applies no network shaping and checks none. Shaping is the operating
system's own facility, applied from outside to the container the system under
test runs in, which is a thing the rig has no handle on. `--shaping` records the
name it is given and `--shaping-command` records the command that was applied,
verbatim and unrun. The report says in the same breath that both were declared
rather than verified.

Give the command whenever the profile is not `none`. A profile name is a label
somebody chose, and two runs under one label are comparable only if whoever ran
them meant the same thing by it; the command is the part a later reader can run,
read against the label, or find wrong. Where a named profile arrives without
one, the report says so in its origin line rather than leaving a field that
reads the same as an unshaped run.

Delays at or above `max_measurable_delay_ms`, which the report carries, are not
reported as large. They are not reported correctly at all, because past that
point the arriving sweep has crossed into the next trial's window.
