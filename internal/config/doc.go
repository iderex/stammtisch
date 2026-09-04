// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Package config is the one configuration an operator writes and the one place
// it is validated.
//
// An operator meets this before they meet anything else, and a service that
// starts on a bad configuration and misbehaves quietly is one they will debug
// at the wrong layer for an afternoon. So validation is total and it fails
// closed: every declared key is reached, an unknown key is refused rather than
// ignored, and a value that is not accepted stops the process with a message
// naming the key, the value and what was expected. Nothing is substituted
// silently, and a defaulted value is reported at startup so it appears in the
// output an operator pastes when they ask for help.
//
// # The format, and why it is not somebody else's
//
// One key and one value a line, separated by `=`, with `#` starting a comment
// and blank lines ignored. That is parsed here in about forty lines of the
// standard library and it adds no dependency, no language and no runtime to a
// tree that already carries three direct requirements.
//
// The alternatives were weighed rather than assumed. TOML and YAML are what an
// operator expects and both are a module this tree does not carry, for a key
// set of five whose values are two durations, a number and two words. JSON is
// in the standard library and takes no comment, and the first paragraph of the
// issue this package answers says the file is the thing people paste into a
// forum post, which is a document that has to be able to explain itself. The
// cost of the choice is real and is stated rather than hidden: this format is
// this program's own, so nothing else can read it and an operator learns it
// here. The day a key needs a list or a nested table, the trade is worth
// re-taking rather than extending this parser.
//
// # The keys are a table and the table is what the suite judges
//
// Every key is one entry in settings below, carrying what it is for, what a
// valid value looks like, its default where it has one, and a value that has to
// be refused. The last field is why the totality claim is a proof rather than a
// promise: TestEverySettingIsReachedByValidation walks the table, feeds each
// entry's faulty value in on its own and requires a refusal naming that key, so
// a key added without one reds the suite rather than arriving unvalidated.
//
// # Why the defaults report is not a log line
//
// The log surface refuses free text on purpose, and a key's name is not
// anything it can carry: no Field constructor takes a string and Identifier
// admits local@host and nothing else. That is an argument against laundering a
// key name through it rather than a gap in it.
//
// The deciding reason is one step earlier, though. Where the log writes is
// itself one of the keys here, so at the moment the defaults report exists
// there is no log to write it to. The report is produced here as lines and
// written by the entry point, which is the only place that knows where a
// process's own startup output goes before any of this has been read.
//
// # Secrets
//
// No key here carries one, and the format has no shape for writing one down. A
// key that needs a secret takes the path of a file holding it and is named for
// that, so what is in this file is a reference and never the secret. The
// convention is enforced on the example configuration by the
// `Configuration secrets` job, which refuses a literal in it that looks like a
// secret and proves it can before it reads the file.
//
// What refuses a secret written into an operator's own configuration is
// nothing, because that file is not in this tree and no route reaches it. That
// is the residual and it is not narrowed by anything here.
package config
