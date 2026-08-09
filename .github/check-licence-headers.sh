#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026  iderex
#
# Refuses a tracked source file whose first two lines are not the licence
# header for the arm that covers it, and refuses a tracked file it cannot
# classify as source or as text.
#
# The classifier half is the one that matters over time. A header rule that only
# looks at the file types the tree happens to hold today stops covering the
# tree on the day somebody adds a language, and nothing says so. Here an
# extension this script does not name is a refusal asking to be classified,
# so a second toolchain arriving cannot arrive quietly.
#
# The arm half is the same shape one level up. This repository carries two
# licences, and which one a file is under is a property of where it sits. A
# check that admitted either identifier anywhere would let the server's arm
# spread into the surface third parties write against, and let the permissive
# arm spread out of it, which is the direction that cannot be taken back once
# somebody has built on it. So the expected identifier is decided per path and
# the other one is refused by name.
#
# It proves itself before it judges anything. The self-test builds fixtures in
# a temporary directory, runs the same scan over them, and refuses unless every
# fault is caught and the correct files are not. A run reporting a clean tree
# has therefore also shown that the scan can still say no.
#
# Usage: sh .github/check-licence-headers.sh

set -eu

SPDX_DEFAULT='SPDX-License-Identifier: AGPL-3.0-or-later'
SPDX_APACHE='SPDX-License-Identifier: Apache-2.0'
COPY='Copyright (C) 2026  iderex'

# The paths under the second arm, as a list this script reads rather than as a
# sentence in a document that nothing compares against the tree. Each entry is a
# prefix and everything under it takes Apache-2.0; every path not matched by an
# entry takes the repository's own arm.
#
# Adding a surface is adding an entry here. The proof block below derives a
# fixture pair from every entry, so an entry that is added and then never
# exercised is not a thing this list can hold: an arm that cannot be shown to
# bite in both directions reds the run before the tree is read.
APACHE_PATHS='botapi/'

# The identifier a file at this path has to carry.
spdx_for() {
	for spdx_arm in $APACHE_PATHS; do
		case "$1" in
		"$spdx_arm"*)
			printf '%s' "$SPDX_APACHE"
			return 0
			;;
		esac
	done
	printf '%s' "$SPDX_DEFAULT"
}

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
#
# The extension patterns judge a whole path and need nothing done to them. The
# bare names are different: scan() is given what `git ls-files` prints, so
# `bench/.gitignore` arrives with its directory attached and a pattern written
# as a file name matches only in the repository root. They are matched against
# the last path component for that reason.
#
# That widens them to the same file under any directory and to nothing else.
# The anchoring is the point and the proof block below spends most of its
# faulty fixtures on it: `*.gitignore` would have been one character shorter to
# write and would also accept `backup.gitignore`, which is a file this list
# never named.
classify() {
	case "$1" in
	*.go | *.sh) printf 'source' ;;
	*.md | *.yml | *.yaml | *.json | *.mod | *.sum | *.txt) printf 'text' ;;
	*)
		case "${1##*/}" in
		LICENSE | NOTICE | DCO | .gitattributes | .gitignore) printf 'text' ;;
		*) return 1 ;;
		esac
		;;
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
		scan_spdx=$(spdx_for "$scan_file")
		scan_first=1
		# A shebang keeps the first line and the header follows it.
		case $(line_at "$scan_file" 1) in
		'#!'*) scan_first=2 ;;
		esac

		if [ "$(line_at "$scan_file" "$scan_first")" != "$scan_prefix $scan_spdx" ]; then
			printf 'line %s is not "%s %s", which is the arm this path is under: %s\n' \
				"$scan_first" "$scan_prefix" "$scan_spdx" "$scan_file"
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
mkdir "$work/sub" "$work/notes"

# The fixtures are scanned from inside the temporary directory rather than by
# absolute path, so each one reaches scan() in the shape `git ls-files` prints:
# some bare, some under a directory. A classifier that only ever sees an
# absolute path cannot be shown to mishandle a relative one, and mishandling a
# relative one is what #119 was.
proof_scan() (
	cd "$work" && scan "$@"
)

printf '%s\n' "// $SPDX_DEFAULT" "// $COPY" '' 'package fixture' >"$work/good.go"
printf '%s\n' '#!/bin/sh' "# $SPDX_DEFAULT" "# $COPY" >"$work/good.sh"
# Named files, at the root and under a directory. Both directions are load
# bearing: a repair written as `*/.gitignore` fixes the subdirectory case and
# drops the root one, and nothing else in this block would notice.
printf '%s\n' '*.json' >"$work/.gitignore"
printf '%s\n' '*.json' >"$work/sub/.gitignore"
printf '%s\n' '* text=auto' >"$work/sub/.gitattributes"
printf '%s\n' 'a licence' >"$work/sub/LICENSE"
printf '%s\n' 'a notice' >"$work/sub/NOTICE"
printf '%s\n' 'a certificate' >"$work/sub/DCO"

printf '%s\n' 'package fixture' >"$work/absent.go"
# The one-character-class mistake: the identifier of the licence without its
# "or later" arm, which is a different licence and reads identical at a glance.
printf '%s\n' '// SPDX-License-Identifier: AGPL-3.0' "// $COPY" >"$work/nearmiss.go"
printf '%s\n' "// $COPY" "// $SPDX_DEFAULT" >"$work/swapped.go"
printf '%s\n' '#!/bin/sh' "# $COPY" >"$work/shebang-then-nothing.sh"
printf '%s\n' 'fn main() {}' >"$work/unknown.rs"
# An unnamed extension has to stay refused under a directory too, or the
# classifier stops covering the tree exactly where the tree grows.
printf '%s\n' 'fn main() {}' >"$work/sub/unknown.rs"
# The two mistakes a basename match invites, both one character from correct.
# `*.gitignore` accepts the first; `LICENSE*` accepts the second. Neither name
# is in the list, and a classifier that takes them has stopped asking.
printf '%s\n' '*.json' >"$work/sub/backup.gitignore"
printf '%s\n' 'an exception' >"$work/notes/LICENSE-EXCEPTIONS"

# The permissive identifier where nothing grants it. This is the mistake a
# person makes by copying a header out of the surface they were just reading,
# and it relicenses a file of the server by two words.
printf '%s\n' "// $SPDX_APACHE" "// $COPY" >"$work/wrong-arm-outside.go"

clean_fixtures='good.go good.sh .gitignore sub/.gitignore sub/.gitattributes sub/LICENSE sub/NOTICE sub/DCO'
faulty_fixtures='absent.go nearmiss.go swapped.go shebang-then-nothing.sh unknown.rs sub/unknown.rs sub/backup.gitignore notes/LICENSE-EXCEPTIONS wrong-arm-outside.go'

# A pair per entry in the arm list, derived from the list rather than written
# beside it, so an arm added without a proof is not a state this script has. The
# two files differ in one identifier and in nothing else, which is the whole
# near miss: a file under the surface carrying the server's arm is what happens
# when somebody adds a file the ordinary way.
for arm in $APACHE_PATHS; do
	mkdir -p "$work/$arm"
	printf '%s\n' "// $SPDX_APACHE" "// $COPY" '' 'package fixture' >"$work/${arm}right-arm.go"
	printf '%s\n' "// $SPDX_DEFAULT" "// $COPY" '' 'package fixture' >"$work/${arm}wrong-arm.go"
	clean_fixtures="$clean_fixtures ${arm}right-arm.go"
	faulty_fixtures="$faulty_fixtures ${arm}wrong-arm.go"
done

proof_failures=0
clean_count=0
faulty_count=0

# Refuses nothing it should not.
for good in $clean_fixtures; do
	clean_count=$((clean_count + 1))
	if out=$(proof_scan "$good"); then
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
	if out=$(proof_scan "$bad"); then
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
