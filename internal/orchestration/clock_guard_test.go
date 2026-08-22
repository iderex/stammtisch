// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// bannedTimeCalls are the ways a package reads or waits on the operating system
// clock. Types out of the same package are fine and are used all over this one:
// time.Time and time.Duration say nothing about when it is.
var bannedTimeCalls = map[string]string{
	"Now":       "read the injected Clock instead",
	"Since":     "subtract two readings of the injected Clock instead",
	"Until":     "subtract two readings of the injected Clock instead",
	"Sleep":     "a suite that sleeps cannot test a timeout at all",
	"After":     "a real timer cannot be advanced by a test",
	"AfterFunc": "a real timer cannot be advanced by a test",
	"Tick":      "a real ticker cannot be advanced by a test",
	"NewTimer":  "a real timer cannot be advanced by a test",
	"NewTicker": "a real ticker cannot be advanced by a test",
}

// bannedImports are the packages that make a value unpredictable to a test.
var bannedImports = map[string]string{
	"math/rand":    "identifier generation is injected, so a test can fix the sequence",
	"math/rand/v2": "identifier generation is injected, so a test can fix the sequence",
	"crypto/rand":  "identifier generation is injected, so a test can fix the sequence",
}

// TestNothingInOrchestrationReadsTheClockDirectly is the check issue #24 asks
// for. Time and identifiers are injected here from the first line of code, and
// the reason is that this layer is made of grace periods and reconnect windows:
// every one of them is untestable in a suite that waits for real time to pass.
//
// It covers the tests as well as the code under them. A test that reads the
// operating system clock is the same defect one step out, and it is the more
// likely one, because reaching for time.Now in a test never looks like a
// design decision.
func TestNothingInOrchestrationReadsTheClockDirectly(t *testing.T) {
	files := goFilesUnder(t, ".")
	// A guard that passes on an empty file list is not a guard.
	if len(files) < 2 {
		t.Fatalf("found %d Go files under this package, which is not the tree this test means to read", len(files))
	}

	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}

		timeName := ""
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: import path %s: %v", path, spec.Path.Value, err)
			}
			if why, banned := bannedImports[imported]; banned {
				t.Errorf("%s imports %s: %s", path, imported, why)
			}
			if imported == "time" {
				timeName = "time"
				if spec.Name != nil {
					timeName = spec.Name.Name
				}
			}
		}
		if timeName == "" || timeName == "_" {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != timeName {
				return true
			}
			if why, banned := bannedTimeCalls[sel.Sel.Name]; banned {
				t.Errorf("%s:%d calls %s.%s: %s",
					path, fset.Position(sel.Pos()).Line, timeName, sel.Sel.Name, why)
			}
			return true
		})
	}
}

// goFilesUnder returns every Go file at or below root, tests included.
func goFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		found = append(found, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}
