package repocheck

import (
	"fmt"
	"os"
	"testing"
)

// TestMain runs the tests from the repository root, so their paths are the
// ones the documents use.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintln(os.Stderr, "repocheck:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
