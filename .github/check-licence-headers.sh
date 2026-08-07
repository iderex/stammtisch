#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026  iderex
#
# Refuses a tracked source file whose first two lines are not the licence
# header, and refuses a tracked file it cannot classify as source or as text.
#
# The second half is the one that matters over time. A header rule that only
# looks at the file types the tree happens to hold today stops covering the
# tree on the day somebody adds a language, and nothing says so. Here an
# extension this script does not name is a refusal asking to be classified,
# so a second toolchain arriving cannot arrive quietly.
#
# It proves itself before it judges anything. The self-test builds fixtures in
# a temporary directory, runs the same scan over them, and refuses unless every
# fault is caught and the correct files are not. A run reporting a clean tree
# has therefore also shown that the scan can still say no.
#
# Usage: sh .github/check-licence-headers.sh

set -eu

SPDX='SPDX-License-Identifier: AGPL-3.0-or-later'
COPY='Copyright (C) 2026  iderex'

# The comment prefix per source language. An extension absent from here is not
# a source file as far as this script is concerned, which is what classify()
# then decides.
prefix_for() {
	case "$1" in
	*.go) printf '//' ;;
	*.sh) printf '#' ;;
	*) return 1 ;;
	esac
}

# source: carries the header. text: does not. Anything else is a refusal.
classify() {
	case "$1" in
	*.go | *.sh) printf 'source' ;;
	*.md | *.yml | *.yaml | *.json | *.mod | *.sum | *.txt) printf 'text' ;;
	LICENSE | NOTICE | DCO | .gitattributes | .gitignore) printf 'text' ;;
	*) return 1 ;;
	esac
}

# Reads one line of a file with any carriage return removed, so a checkout that
# rewrote the line endings is not reported as a missing header.
line_at() {
	sed -n "$2p" "$1" | tr -d '\r'
}

# Prints one line per violation and returns 1 if there was any.
scan() {
	scan_bad=0
	for scan_file in "$@"; do
		if scan_class=$(classify "$scan_file"); then
			:
		else
			printf 'unclassified file, name it in classify(): %s\n' "$scan_file"
			scan_bad=1
			continue
		fi
		[ "$scan_class" = source ] || continue

		scan_prefix=$(prefix_for "$scan_file")
		scan_first=1
		# A shebang keeps the first line and the header follows it.
		case $(line_at "$scan_file" 1) in
		'#!'*) scan_first=2 ;;
		esac

		if [ "$(line_at "$scan_file" "$scan_first")" != "$scan_prefix $SPDX" ]; then
			printf 'no licence identifier on line %s: %s\n' "$scan_first" "$scan_file"
			scan_bad=1
			continue
		fi
		if [ "$(line_at "$scan_file" "$((scan_first + 1))")" != "$scan_prefix $COPY" ]; then
			printf 'no copyright line under the identifier: %s\n' "$scan_file"
			scan_bad=1
		fi
	done
	return "$scan_bad"
}

# --- the proof, run before the tree is judged --------------------------------

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

printf '%s\n' "// $SPDX" "// $COPY" '' 'package fixture' >"$work/good.go"
printf '%s\n' '#!/bin/sh' "# $SPDX" "# $COPY" >"$work/good.sh"

printf '%s\n' 'package fixture' >"$work/absent.go"
# The one-character-class mistake: the identifier of the licence without its
# "or later" arm, which is a different licence and reads identical at a glance.
printf '%s\n' '// SPDX-License-Identifier: AGPL-3.0' "// $COPY" >"$work/nearmiss.go"
printf '%s\n' "// $COPY" "// $SPDX" >"$work/swapped.go"
printf '%s\n' '#!/bin/sh' "# $COPY" >"$work/shebang-then-nothing.sh"
printf '%s\n' 'fn main() {}' >"$work/unknown.rs"

clean_fixtures='good.go good.sh'
faulty_fixtures='absent.go nearmiss.go swapped.go shebang-then-nothing.sh unknown.rs'

proof_failures=0
clean_count=0
faulty_count=0

# Refuses nothing it should not.
for good in $clean_fixtures; do
	clean_count=$((clean_count + 1))
	if out=$(scan "$work/$good"); then
		[ -z "$out" ] || {
			printf 'PROOF FAILED: %s produced output: %s\n' "$good" "$out"
			proof_failures=$((proof_failures + 1))
		}
	else
		printf 'PROOF FAILED: %s was refused and it carries the header\n' "$good"
		proof_failures=$((proof_failures + 1))
	fi
done

# Refuses every fault, and names the file it refused.
for bad in $faulty_fixtures; do
	faulty_count=$((faulty_count + 1))
	if out=$(scan "$work/$bad"); then
		printf 'PROOF FAILED: %s passed and it should not\n' "$bad"
		proof_failures=$((proof_failures + 1))
	else
		case "$out" in
		*"$bad"*) ;;
		*)
			printf 'PROOF FAILED: %s was refused without being named: %s\n' "$bad" "$out"
			proof_failures=$((proof_failures + 1))
			;;
		esac
	fi
done

if [ "$proof_failures" -ne 0 ]; then
	printf '\n%s proof(s) failed, so the scan does not behave as this script asserts\nand its verdict on the tree is not reported.\n' "$proof_failures"
	exit 1
fi

printf 'proof: %s deliberate fault(s) refused and named, %s correct file(s) passed\n' \
	"$faulty_count" "$clean_count"

# --- the tree ----------------------------------------------------------------

git ls-files >"$work/tracked"
tracked_count=$(wc -l <"$work/tracked" | tr -d ' ')
printf 'scanning %s tracked file(s)\n' "$tracked_count"

tree_bad=0
while IFS= read -r tracked; do
	[ -n "$tracked" ] || continue
	if out=$(scan "$tracked"); then
		[ -z "$out" ] || {
			printf '%s\n' "$out"
			tree_bad=1
		}
	else
		printf '%s\n' "$out"
		tree_bad=1
	fi
done <"$work/tracked"

if [ "$tree_bad" -ne 0 ]; then
	printf '\nThe header is two lines at the top of the file, the second one after a\nshebang where there is one. CONTRIBUTING.md carries the exact text.\n'
	exit 1
fi

printf 'every tracked source file carries the licence header\n'
