package orchestration_test

import (
	"os"
	"testing"
)

// TestProofADisplayIsRequired is a deliberate fault. It exists to show that the
// Unit check refuses a test that needs a display server, and it is never
// merged. It fails on any runner where DISPLAY is empty, which is every runner
// the Unit job describes.
func TestProofADisplayIsRequired(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Fatal("this test needs a display server and there is none")
	}
}
