//go:build linux

package native

import (
	"testing"

	"github.com/radix29/gogui/internal/shell/gtk4"
)

func TestNewGTK4(t *testing.T) {
	sh, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := sh.(*gtk4.Shell); !ok {
		t.Fatalf("New() = %T; want *gtk4.Shell", sh)
	}
	// A second call returns a fresh shell, not a shared one.
	sh2, _ := New()
	if sh2 == sh {
		t.Fatal("New() returned the same shell twice")
	}
}
