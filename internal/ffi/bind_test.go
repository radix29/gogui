package ffi

import (
	"reflect"
	"strings"
	"testing"
)

// TestCheckBindFunc checks the rule Bind and BindAddr apply before purego
// sees a func type (S0.10b): a Go func argument, at any position, is
// rejected with a message naming it and pointing to RegisterCallback; a
// func-typed result and every other argument kind are left to purego.
func TestCheckBindFunc(t *testing.T) {
	type cmp func(a, b uintptr) int32
	tests := []struct {
		name string
		fn   any
		arg  string // "" if allowed, else the argument the message names
	}{
		{"no args", (func())(nil), ""},
		{"scalars", (func(uintptr, int32, float64) uintptr)(nil), ""},
		{"func result", (func(uintptr) func(uintptr) uintptr)(nil), ""},
		{"named func result", (func() cmp)(nil), ""},
		{"func pointer arg", (func(*func()) uintptr)(nil), ""},
		{"func arg first", (func(func(), uintptr))(nil), "argument 0"},
		{"func arg last", (func(uintptr, uintptr, uintptr, func(uintptr) uintptr))(nil), "argument 3"},
		{"named func arg", (func(uintptr, uintptr, uintptr, cmp))(nil), "argument 3"},
		{"g_signal_connect_data shape", (func(uintptr, uintptr, func(uintptr, uintptr), uintptr, uintptr, uint32) uint64)(nil), "argument 2"},
		{"two func args", (func(func(), func()))(nil), "argument 0"},
	}
	for _, tt := range tests {
		msg := checkBindFunc(reflect.TypeOf(tt.fn))
		if tt.arg == "" {
			if msg != "" {
				t.Errorf("%s: rejected: %s", tt.name, msg)
			}
			continue
		}
		if !strings.HasPrefix(msg, tt.arg+" is a Go func") ||
			!strings.Contains(msg, "RegisterCallback") || !strings.Contains(msg, "uintptr") {
			t.Errorf("%s: message %q, want one naming %s and pointing to RegisterCallback", tt.name, msg, tt.arg)
		}
	}
}
