# .github

The gate. Workflows and the scripts they call belong here; nothing the server or
the bench imports does, and no rule that a workflow does not run.

What each check refuses is written at the top of its own workflow file, so this
directory deliberately holds no list of them. `CONTRIBUTING.md` sends a reader
to the files, and `docs/reference-gate-parity.md` records which of them are
carried over from the reference gate and which deviate.
