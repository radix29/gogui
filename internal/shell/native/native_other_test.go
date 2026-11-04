//go:build !windows && !linux

package native

import (
	"errors"
	"testing"
)

func TestNewNotImplemented(t *testing.T) {
	sh, err := New()
	if sh != nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("New() = %v, %v; want nil, ErrNotImplemented", sh, err)
	}
}
