package orchestration_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePath      = "github.com/iderex/stammtisch"
	mediaPrefix     = modulePath + "/internal/media"
	benchPrefix     = modulePath + "/bench"
	selfPackagePath = modulePath + "/internal/orchestration"
)

// TestOrchestrationDoesNotReachTheMediaPlane holds the seam docs/layout.md
// describes. The orchestration layer stays importable without a media
// dependency, which is what lets the unit suite run on a machine with no audio
// device and nothing to forward media to.
//
// It asks the toolchain for the dependency graph rather than reading imports
// out of the source, so an import arriving through a third package is caught
// exactly like a direct one. It asks for the graph of the test binaries too,
// because a suite that only runs when a media package is present is the same
// problem one step out.
func TestOrchestrationDoesNotReachTheMediaPlane(t *testing.T) {
	deps := dependencyGraph(t, true, selfPackagePath+"/...")
	// A guard that passes on an empty answer is not a guard. If the package
	// this test lives in is missing, the graph is not the one it thinks it
	// read.
	requirePresent(t, deps, selfPackagePath)
	requireAbsent(t, deps, mediaPrefix,
		"the orchestration layer must stay runnable with no media plane in the import graph")
}

// TestTheShippedPackagesDoNotReachTheBench holds the other half of the layout:
// the measurement rigs are development only and must not end up in an
// operator's binary.
//
// This one asks for the graph without the test binaries, because a rig imported
// by a test is not shipped and refusing it would be refusing the wrong thing.
func TestTheShippedPackagesDoNotReachTheBench(t *testing.T) {
	deps := dependencyGraph(t, false, modulePath, modulePath+"/internal/...", modulePath+"/botapi/...")
	requirePresent(t, deps, modulePath)
	requirePresent(t, deps, modulePath+"/botapi")
	requireAbsent(t, deps, benchPrefix, "a measurement rig must not reach an operator's binary")
}

func requirePresent(t *testing.T, deps map[string]bool, pkg string) {
	t.Helper()
	if !deps[pkg] {
		t.Fatalf("the dependency graph does not contain %s, so it is not the graph this test asked for", pkg)
	}
}

func requireAbsent(t *testing.T, deps map[string]bool, prefix, why string) {
	t.Helper()
	for dep := range deps {
		if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
			t.Errorf("the graph contains %s: %s", dep, why)
		}
	}
}

// dependencyGraph returns every package the given patterns depend on. It fails
// the test rather than returning nothing when the toolchain cannot be asked.
func dependencyGraph(t *testing.T, withTests bool, patterns ...string) map[string]bool {
	t.Helper()
	args := []string{"list", "-deps"}
	if withTests {
		args = append(args, "-test")
	}
	args = append(args, patterns...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	deps := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			deps[line] = true
		}
	}
	if len(deps) == 0 {
		t.Fatalf("go %s returned nothing", strings.Join(args, " "))
	}
	return deps
}
