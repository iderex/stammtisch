// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The file that is allowed to decide a permission, and the file holding this
// guard. Everything else in the package goes through Allow.
const (
	permissionModelFile = "permission.go"
	permissionGuardFile = "permission_guard_test.go"
)

// deciderCalls are the calls that read a grant directly. A file making one of
// these is answering a permission question itself, whatever it does with the
// answer afterwards.
//
// Only calls are refused, never declarations. A test that implements Grantor
// declares a method called Granted and that is the correct way to supply grants;
// what it may not do is call one.
var deciderCalls = map[string]string{
	"Granted": "read a grant and judged it here. Ask orchestration.Allow instead",
	"has":     "tested a PermissionSet for membership here. Ask orchestration.Allow instead",
}

// kindIdentifiers are the names that let a caller tell a bot from a person. A
// file naming one of these is a file that can write the special case #29 exists
// to prevent, which is a bot skipping a check a person takes.
var kindIdentifiers = map[string]string{
	"principalKind": "branched on what kind of principal this is",
	"kindPerson":    "branched on what kind of principal this is",
	"kindBot":       "branched on what kind of principal this is",
}

// TestOnlyOnePlaceDecidesAPermission is the check #29 asks for in the words
// "a check refuses a permission decision made anywhere else".
//
// It refuses the vocabulary rather than the intent, because intent is not
// readable from a tree. What it can say is that no file but permission.go
// reaches a grant set or names the principal kind, and those are the two things
// a second decision site has to do. A caller naming a permission is not caught
// and is not meant to be: passing SeeChannel to Allow is how the model is used.
//
// It fails closed. A run that parsed nothing, or that could not find the model
// file, is a failure rather than a pass.
func TestOnlyOnePlaceDecidesAPermission(t *testing.T) {
	files := goFilesUnder(t, ".")
	if len(files) < 2 {
		t.Fatalf("found %d Go files under this package, which is not the tree this test means to read", len(files))
	}

	sawModel := false
	inspected := 0
	fset := token.NewFileSet()

	for _, path := range files {
		base := path[strings.LastIndex(path, "/")+1:]
		if base == permissionModelFile {
			sawModel = true
			continue
		}
		if base == permissionGuardFile {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		inspected++

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if why, banned := deciderCalls[sel.Sel.Name]; banned {
					t.Errorf("%s:%d %s", path, fset.Position(sel.Pos()).Line, why)
				}
			case *ast.Ident:
				if why, banned := kindIdentifiers[node.Name]; banned {
					t.Errorf("%s:%d %s", path, fset.Position(node.Pos()).Line, why)
				}
			}
			return true
		})
	}

	if !sawModel {
		t.Fatalf("%s is not among the %d files read, so this guard was watching a package that does not hold the model",
			permissionModelFile, len(files))
	}
	if inspected == 0 {
		t.Fatal("no file other than the model and this guard was inspected, so nothing was actually checked")
	}
}

// TestPermissionsMatchesTheDeclaredConstants reads the constants of type
// Permission out of the model file and compares them against what Permissions()
// returns.
//
// Without it, Permissions() is a hand-written list beside a hand-written set of
// constants, and the failure mode is silent in the worst direction: a
// permission left out of the list is a permission the generated matrix suite in
// #37 never produces a case for, so the suite goes on passing while a row of it
// has quietly stopped existing.
func TestPermissionsMatchesTheDeclaredConstants(t *testing.T) {
	declared := permissionConstantsInSource(t)
	if len(declared) < 2 {
		t.Fatalf("read %d Permission constants out of %s, which is not the file this test means to read",
			len(declared), permissionModelFile)
	}

	listed := map[string]bool{}
	for _, p := range permissionNamesInSource(t, declared) {
		listed[p] = true
	}

	returned := map[string]bool{}
	for _, p := range permissionsReturnedInSource(t) {
		returned[p] = true
	}

	for name := range listed {
		if !returned[name] {
			t.Errorf("%s declares the permission %s and Permissions() does not return it, "+
				"so nothing generated from that list will ever test it", permissionModelFile, name)
		}
	}
	for name := range returned {
		if !listed[name] {
			t.Errorf("Permissions() returns %s and it is not a Permission constant in %s", name, permissionModelFile)
		}
	}
}

// TestEveryDeclaredPermissionIsNamedInAllow reads the switch in Allow and
// requires every declared permission to appear in one of its case lists.
//
// The default arm already refuses an unnamed permission, so the model fails
// closed without this test. What it adds is the noise: failing closed and
// failing silently is how a permission that was added on purpose ends up
// refused for everybody with nothing saying why.
func TestEveryDeclaredPermissionIsNamedInAllow(t *testing.T) {
	declared := permissionConstantsInSource(t)
	named := permissionsNamedInAllowSwitch(t)
	if len(named) == 0 {
		t.Fatalf("found no case naming a permission in Allow, so this test read the wrong function")
	}

	for _, name := range permissionNamesInSource(t, declared) {
		if !named[name] {
			t.Errorf("Allow does not name %s in any case of its switch, so it reaches the default arm "+
				"and is refused for every principal with nothing in the code saying that was intended", name)
		}
	}
}

// --- reading the model out of its own source --------------------------------

// permissionConstantsInSource returns the const specs whose type is Permission,
// including the ones that inherit it through iota.
func permissionConstantsInSource(t *testing.T) []*ast.ValueSpec {
	t.Helper()
	file := parseModel(t)

	var found []*ast.ValueSpec
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		isPermission := false
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, named := value.Type.(*ast.Ident); named {
				isPermission = ident.Name == "Permission"
			}
			if isPermission {
				found = append(found, value)
			}
		}
	}
	return found
}

// permissionNamesInSource flattens the constant names out of those specs.
func permissionNamesInSource(t *testing.T, specs []*ast.ValueSpec) []string {
	t.Helper()
	var names []string
	for _, spec := range specs {
		for _, ident := range spec.Names {
			if ident.Name == "_" {
				continue
			}
			names = append(names, ident.Name)
		}
	}
	return names
}

// permissionsReturnedInSource reads the identifiers in the slice literal
// Permissions() returns.
func permissionsReturnedInSource(t *testing.T) []string {
	t.Helper()
	fn := funcInModel(t, "Permissions")

	var names []string
	ast.Inspect(fn, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			if ident, isIdent := elt.(*ast.Ident); isIdent {
				names = append(names, ident.Name)
			}
		}
		return true
	})
	return names
}

// permissionsNamedInAllowSwitch reads the case lists of the switch on the
// permission inside Allow.
func permissionsNamedInAllowSwitch(t *testing.T) map[string]bool {
	t.Helper()
	fn := funcInModel(t, "Allow")

	named := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		clause, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expr := range clause.List {
			if ident, isIdent := expr.(*ast.Ident); isIdent {
				named[ident.Name] = true
			}
		}
		return true
	})
	return named
}

func funcInModel(t *testing.T, name string) *ast.FuncDecl {
	t.Helper()
	file := parseModel(t)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("%s declares no function called %s, so this test read the wrong file", permissionModelFile, name)
	return nil
}

func parseModel(t *testing.T) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), permissionModelFile, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", permissionModelFile, err)
	}
	return file
}
