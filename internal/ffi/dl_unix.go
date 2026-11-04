//go:build darwin || linux

package ffi

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/ebitengine/purego"
)

// Lib is a shared library opened with Dlopen. Its methods are safe for
// concurrent use except Close, which must not race with the others.
type Lib struct {
	path string
	h    uintptr
}

// Dlopen loads the shared library at path (a file path or, on Linux, a
// soname such as "libgtk-4.so.1") with RTLD_NOW|RTLD_GLOBAL, so missing
// dependencies fail here and later libraries can resolve against its
// symbols. The error names the library and carries dlerror's message.
func Dlopen(path string) (*Lib, error) {
	if path == "" {
		return nil, errors.New("ffi: dlopen with an empty path")
	}
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("ffi: dlopen %s: %w", path, err)
	}
	return &Lib{path: path, h: h}, nil
}

// Path returns the path the library was opened with.
func (l *Lib) Path() string { return l.path }

// Sym returns the address of the symbol name in the library (or in the
// libraries it depends on). The error names the symbol and the library.
func (l *Lib) Sym(name string) (uintptr, error) {
	if l.h == 0 {
		return 0, fmt.Errorf("ffi: symbol %s in %s: library is closed", name, l.path)
	}
	p, err := purego.Dlsym(l.h, name)
	if err != nil {
		return 0, fmt.Errorf("ffi: symbol %s not found in %s: %w", name, l.path, err)
	}
	if p == 0 {
		return 0, fmt.Errorf("ffi: symbol %s in %s resolves to NULL", name, l.path)
	}
	return p, nil
}

// MustSym is Sym that panics on error. It is meant for symbols a binding
// can't work without, looked up once at start-up.
func (l *Lib) MustSym(name string) uintptr {
	p, err := l.Sym(name)
	if err != nil {
		panic(err)
	}
	return p
}

// Bind looks up the C function name and sets *fptr (a pointer to a Go
// func variable) to a function that calls it, through
// purego.RegisterFunc. A missing symbol or a func type purego rejects is
// returned as an error naming the library and the symbol instead of a
// panic. So is a func type with a Go func argument (see checkBindFunc):
// pass a RegisterCallback pointer as a uintptr instead. Nothing checks
// that the Go signature matches the C one.
func (l *Lib) Bind(fptr any, name string) (err error) {
	v := reflect.ValueOf(fptr)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Func {
		return fmt.Errorf("ffi: bind %s in %s: fptr is %T, not a non-nil pointer to a func variable",
			name, l.path, fptr)
	}
	if msg := checkBindFunc(v.Elem().Type()); msg != "" {
		return fmt.Errorf("ffi: bind %s in %s as %s: %s", name, l.path, v.Elem().Type(), msg)
	}
	sym, err := l.Sym(name)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ffi: bind %s in %s as %s: %v", name, l.path, v.Elem().Type(), r)
		}
	}()
	purego.RegisterFunc(fptr, sym)
	return nil
}

// BindAddr sets *fptr (a pointer to a Go func variable) to a function that
// calls the C function at addr, through purego.RegisterFunc. It is Bind for
// a function pointer that has no symbol name: a vfunc slot read from a
// GObject class struct, or a parent class's implementation to chain up to.
// An addr of 0, a bad fptr, a func type with a Go func argument (see
// checkBindFunc) and a func type purego rejects are returned as errors
// instead of panics, and *fptr is left unchanged. Nothing checks that the
// Go signature matches the C one or that addr points to code; a wrong addr
// crashes the process when the func is called.
func BindAddr(fptr any, addr uintptr) (err error) {
	where := "0x" + strconv.FormatUint(uint64(addr), 16)
	v := reflect.ValueOf(fptr)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Func {
		return fmt.Errorf("ffi: bind address %s: fptr is %T, not a non-nil pointer to a func variable",
			where, fptr)
	}
	if msg := checkBindFunc(v.Elem().Type()); msg != "" {
		return fmt.Errorf("ffi: bind address %s as %s: %s", where, v.Elem().Type(), msg)
	}
	if addr == 0 {
		return fmt.Errorf("ffi: bind address 0 as %s: NULL function pointer", v.Elem().Type())
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ffi: bind address %s as %s: %v", where, v.Elem().Type(), r)
		}
	}()
	purego.RegisterFunc(fptr, addr)
	return nil
}

// Close releases the library handle (dlclose). Functions bound from it
// must not be called afterwards. Closing a closed Lib does nothing.
func (l *Lib) Close() error {
	if l.h == 0 {
		return nil
	}
	h := l.h
	l.h = 0
	if err := purego.Dlclose(h); err != nil {
		return fmt.Errorf("ffi: dlclose %s: %w", l.path, err)
	}
	return nil
}
