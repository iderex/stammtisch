#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026  iderex
#
# Refuses a module whose manifest names a dependency the lockfile does not pin.
#
# The dependency review workflow reads what a change did to the graph. It cannot
# say that the graph underneath an unchanged manifest is the one the tree
# declares, because it never looks at the lockfile. This script is the other
# direction: for every module `go.mod` requires, `go.sum` has to carry the pin
# for that exact version, or the tree is refused.
#
# What it does not do is judge the rest of the lockfile. `go.sum` legitimately
# carries entries for modules nothing requires directly, so a check reading that
# direction would refuse correct trees. `go mod tidy -diff` covers that
# direction and runs beside this script in the workflow.
#
# A `replace` directive is refused rather than reasoned about. A replacement can
# move the obligation to another module or remove it entirely by pointing at a
# directory, and a check that guessed which would be wrong quietly. Here it is a
# refusal asking to be handled, so a replacement cannot arrive unnoticed in a
# tree whose argument is that an operator can trust what they run.
#
# It proves itself before it judges anything. The self-test builds fixture
# modules in a temporary directory, runs the same check over them, and refuses
# unless every faulty one was caught and the correct ones were not. A run
# reporting a clean tree has therefore also shown that the check can still say
# no.
#
# Usage: sh .github/check-dependency-lock.sh [directory]

set -eu

root=${1:-.}

command -v go >/dev/null 2>&1 || {
	printf 'go is not on PATH, and this check cannot read a manifest without it\n' >&2
	exit 2
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# True when go.sum pins that module version's go.mod.
#
# The comparison is on whole fields and never on a substring, and that is the
# whole reason it is awk and not grep. `example.com/a` is a prefix of
# `example.com/ab`, so a pattern written as a substring accepts the longer
# module's pin and reports the shorter one as locked. The proof block spends a
# fixture on exactly that, because it is the mistake somebody makes while
# writing this function rather than an obvious one.
pins_gomod() {
	[ -f "$3" ] || return 1
	awk -v want_path="$1" -v want_file="$2/go.mod" '
		$1 == want_path && $2 == want_file { found = 1 }
		END { exit !found }
	' "$3"
}

# Prints one line per violation and returns 1 if there was any.
check_module() {
	check_dir=$1
	check_mod="$check_dir/go.mod"
	check_sum="$check_dir/go.sum"

	if [ ! -f "$check_mod" ]; then
		printf 'no go.mod in %s\n' "$check_dir"
		return 1
	fi

	# The manifest is read through the toolchain's own parse rather than
	# through a pattern over the file, so the block form, the single-line
	# form and an `// indirect` comment all arrive here as the same two
	# fields. A go.mod the toolchain refuses is a refusal here and not an
	# empty list, which is the direction that would otherwise fail open.
	if ! go mod edit -json "$check_mod" >"$work/mod.json" 2>"$work/mod.err"; then
		printf 'go.mod could not be parsed: %s\n' "$check_mod"
		sed 's/^/    /' "$work/mod.err"
		return 1
	fi

	: >"$work/violations"

	if grep -q '"Replace": \[' "$work/mod.json"; then
		printf 'replace directive present, and this check does not reason about one: %s\n' \
			"$check_mod" >>"$work/violations"
	fi

	awk '
		/"Require": \[/    { inside = 1; next }
		inside && /^\t\]/  { inside = 0; next }
		inside && /"Path": / {
			path = $0
			sub(/^[^:]*: "/, "", path)
			sub(/",?$/, "", path)
		}
		inside && /"Version": / {
			version = $0
			sub(/^[^:]*: "/, "", version)
			sub(/",?$/, "", version)
			print path, version
		}
	' "$work/mod.json" >"$work/requires"

	while IFS=' ' read -r req_path req_version; do
		[ -n "$req_path" ] || continue
		if pins_gomod "$req_path" "$req_version" "$check_sum"; then
			continue
		fi
		if [ -f "$check_sum" ]; then
			printf 'required but not pinned in go.sum: %s %s\n' \
				"$req_path" "$req_version" >>"$work/violations"
		else
			printf 'required but there is no go.sum at all: %s %s\n' \
				"$req_path" "$req_version" >>"$work/violations"
		fi
	done <"$work/requires"

	if [ -s "$work/violations" ]; then
		cat "$work/violations"
		return 1
	fi
	return 0
}

# --- the proof, run before the tree is judged --------------------------------

# One fixture module per case. Each carries a go.mod and, where the case has
# one, a go.sum. The hashes are not real and do not need to be: this check reads
# which lines exist and never verifies a hash. Verifying one is `go mod verify`,
# which is a separate step in the workflow and is the toolchain's own.
fixture() {
	mkdir -p "$work/fx/$1"
	cat >"$work/fx/$1/go.mod"
}

pin() {
	printf '%s %s/go.mod h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n' "$1" "$2"
}

content() {
	printf '%s %s h1:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=\n' "$1" "$2"
}

# Correct: one require, both lines present.
fixture pinned <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.0.0
EOF
{ content example.com/a v1.0.0; pin example.com/a v1.0.0; } >"$work/fx/pinned/go.sum"

# Correct: the block form with an indirect comment, both requires pinned. What
# this fixture is about is the parse, not the pins.
fixture blockform <<'EOF'
module example.com/fixture

go 1.26

require (
	example.com/a v1.0.0
	example.com/b v1.2.3 // indirect
)
EOF
{ pin example.com/a v1.0.0; pin example.com/b v1.2.3; } >"$work/fx/blockform/go.sum"

# Correct: nothing required and no go.sum. This is the shape of a module with no
# third-party dependencies, and a check that refused it would refuse this tree.
fixture nodeps <<'EOF'
module example.com/fixture

go 1.26
EOF

# Faulty: required, and there is no lockfile at all.
fixture nosum <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.0.0
EOF

# Faulty: a lockfile that pins something else entirely.
fixture unpinned <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.0.0
EOF
pin example.com/other v1.0.0 >"$work/fx/unpinned/go.sum"

# Faulty: the go.mod pin deleted and the content hash left behind. This is the
# one-line deletion, and it is the shape a bad merge leaves rather than an
# obvious emptying of the file.
fixture deletedline <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.0.0
EOF
content example.com/a v1.0.0 >"$work/fx/deletedline/go.sum"

# Faulty: the version moved in the manifest and the lockfile still carries the
# old one. A check matching on the module path alone passes this.
fixture staleversion <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.1.0
EOF
pin example.com/a v1.0.0 >"$work/fx/staleversion/go.sum"

# Faulty: the near-miss. `example.com/a` is a prefix of `example.com/ab` and the
# lockfile pins only the longer one. A substring comparison reports the shorter
# module as locked.
fixture prefix <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.0.0
EOF
pin example.com/ab v1.0.0 >"$work/fx/prefix/go.sum"

# Faulty: a replace directive, which this check refuses rather than reasons
# about.
fixture replaced <<'EOF'
module example.com/fixture

go 1.26

require example.com/a v1.0.0

replace example.com/a => example.com/b v1.0.0
EOF
{ content example.com/a v1.0.0; pin example.com/a v1.0.0; } >"$work/fx/replaced/go.sum"

proof_failures=0

expect_refused() {
	if check_module "$work/fx/$1" >"$work/proof.log" 2>&1; then
		printf 'PROOF FAILED: %s was accepted, and it is %s\n' "$1" "$2"
		proof_failures=1
	else
		printf 'proved refusable: %s\n' "$2"
	fi
}

expect_accepted() {
	if check_module "$work/fx/$1" >"$work/proof.log" 2>&1; then
		printf 'proved accepted: %s\n' "$2"
	else
		printf 'PROOF FAILED: %s was refused, and it is %s\n' "$1" "$2"
		sed 's/^/    /' "$work/proof.log"
		proof_failures=1
	fi
}

expect_accepted pinned "a require with its go.mod pin present"
expect_accepted blockform "the block form with an indirect comment, both pinned"
expect_accepted nodeps "a module with no requires and no go.sum"
expect_refused nosum "a require in a module with no go.sum at all"
expect_refused unpinned "a require whose go.sum pins a different module"
expect_refused deletedline "a require whose go.mod pin line was deleted"
expect_refused staleversion "a require whose version moved past the pin"
expect_refused prefix "a require whose pin belongs to a longer module path"
expect_refused replaced "a manifest carrying a replace directive"

if [ "$proof_failures" -ne 0 ]; then
	printf 'the check did not prove it can refuse, so the tree is not judged\n'
	exit 1
fi

# --- the tree ----------------------------------------------------------------

printf '\njudging %s\n' "$root"
if check_module "$root"; then
	printf 'every module required by go.mod is pinned in go.sum\n'
else
	exit 1
fi
