//go:build darwin

package main

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/radix29/gogui/internal/ffi"
)

// The spike's ObjC classes. Their methods share a fixed set of
// trampolines, one per IMP signature, created once with
// ffi.RegisterCallback (PLAN.md §7.2). A trampoline finds its Go peer
// through the handle ivar of self.

// actionPeer is the Go side of a GG object (a GGTarget, a window delegate,
// the app delegate). Every void (id, SEL, id) message to the object
// arrives in action; cmd tells the messages apart.
type actionPeer interface {
	action(self objc.ID, cmd objc.SEL, arg objc.ID)
}

// askPeer is a peer that also answers BOOL (id, SEL, id) messages.
type askPeer interface {
	actionPeer
	ask(self objc.ID, cmd objc.SEL, arg objc.ID) bool
}

// peers maps the handle ivar of GG objects to their Go peers. It is used
// on the main thread.
var peers ffi.Handles[actionPeer]

const (
	targetClass         = "GGTarget"
	flippedClass        = "GGFlippedView"
	windowDelegateClass = "GGWindowDelegate"
	appDelegateClass    = "GGAppDelegate"
)

// boolEnc is the type encoding of BOOL: signed char on amd64, bool on
// arm64. A Go bool result fills the return register on both.
var boolEnc = map[string]string{"amd64": "c", "arm64": "B"}[runtime.GOARCH]

var trampolines struct {
	once     sync.Once
	voidIDID uintptr // void (id, SEL, id): actions, tick:, delegate notifications
	boolID   uintptr // BOOL (id, SEL): isFlipped
	boolIDID uintptr // BOOL (id, SEL, id): applicationShouldTerminateAfterLastWindowClosed:
	count    int     // trampolines registered by the spike
}

// registerTrampolines creates the spike's trampolines, once per process.
func registerTrampolines() {
	trampolines.once.Do(func() {
		trampolines.voidIDID = ffi.RegisterCallback("appkit.void(id,SEL,id)", impVoidIDID)
		trampolines.boolID = ffi.RegisterCallback("appkit.BOOL(id,SEL)", impFlipped)
		trampolines.boolIDID = ffi.RegisterCallback("appkit.BOOL(id,SEL,id)", impBoolIDID)
		trampolines.count = 3
	})
}

// impVoidIDID dispatches a void (id, SEL, id) message to the peer whose
// handle is in self, inside an autorelease pool. A zero or stale handle
// (the peer is gone) is ignored.
func impVoidIDID(self objc.ID, cmd objc.SEL, arg objc.ID) {
	p, ok := peers.Get(getHandle(self))
	if !ok {
		return
	}
	withPool(func() { p.action(self, cmd, arg) })
}

// impBoolIDID dispatches a BOOL (id, SEL, id) message to the peer whose
// handle is in self, inside an autorelease pool. Without a peer that
// answers, the result is NO.
func impBoolIDID(self objc.ID, cmd objc.SEL, arg objc.ID) bool {
	p, ok := peers.Get(getHandle(self))
	if !ok {
		return false
	}
	q, ok := p.(askPeer)
	if !ok {
		return false
	}
	var r bool
	withPool(func() { r = q.ask(self, cmd, arg) })
	return r
}

// impFlipped is -[GGFlippedView isFlipped]. It needs no peer.
func impFlipped(self objc.ID, cmd objc.SEL) bool { return true }

// defineClasses registers the spike's four classes.
func defineClasses() error {
	registerTrampolines()
	for _, d := range classDefs() {
		if _, err := defineClass(d); err != nil {
			return err
		}
	}
	return nil
}

func classDefs() []classDef {
	return []classDef{
		{name: targetClass, super: "NSObject", methods: []methodDef{
			{sel: "buttonClicked:", imp: trampolines.voidIDID, types: "v@:@"},
			{sel: "tick:", imp: trampolines.voidIDID, types: "v@:@"},
		}},
		{name: flippedClass, super: "NSView", methods: []methodDef{
			{sel: "isFlipped", imp: trampolines.boolID, types: boolEnc + "@:"},
		}},
		{name: windowDelegateClass, super: "NSObject", methods: []methodDef{
			{sel: "windowDidResize:", imp: trampolines.voidIDID, types: "v@:@"},
			{sel: "windowWillClose:", imp: trampolines.voidIDID, types: "v@:@"},
		}},
		{name: appDelegateClass, super: "NSObject", methods: []methodDef{
			{sel: "applicationDidFinishLaunching:", imp: trampolines.voidIDID, types: "v@:@"},
			{sel: "applicationShouldTerminateAfterLastWindowClosed:", imp: trampolines.boolIDID, types: boolEnc + "@:@"},
		}},
	}
}

// newPeerObject returns [[cls alloc] init] with a fresh handle to p in its
// ivar. Targets and delegates are held weakly by AppKit, so the caller
// keeps the object (and frees the handle) itself.
func newPeerObject(cls string, p actionPeer) objc.ID {
	obj := alloc(cls, "init")
	setHandle(obj, peers.New(p))
	return obj
}

// freePeerObject frees obj's handle and releases obj.
func freePeerObject(obj objc.ID) {
	peers.Free(getHandle(obj))
	setHandle(obj, 0)
	release(obj)
}
