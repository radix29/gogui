//go:build darwin

package main

import (
	"errors"
	"runtime"
	"time"

	"github.com/ebitengine/purego/objc"
)

// The application: activation policy, a minimal main menu, the app
// delegate, the event loop and the wake from other threads.

const (
	activationPolicyRegular     = 0  // NSApplicationActivationPolicyRegular
	eventTypeApplicationDefined = 15 // NSEventTypeApplicationDefined
)

// appPeer is the Go side of the GGAppDelegate.
type appPeer struct {
	// launched runs once, from applicationDidFinishLaunching:, on the
	// main thread inside [NSApp run].
	launched func()
	// keepRunning makes applicationShouldTerminateAfterLastWindowClosed:
	// answer NO (the self-test closes the window without quitting).
	keepRunning bool
}

func (a *appPeer) action(self objc.ID, cmd objc.SEL, arg objc.ID) {
	if cmd == sel("applicationDidFinishLaunching:") {
		activate()
		if a.launched != nil {
			a.launched()
		}
	}
}

func (a *appPeer) ask(self objc.ID, cmd objc.SEL, arg objc.ID) bool {
	return cmd == sel("applicationShouldTerminateAfterLastWindowClosed:") && !a.keepRunning
}

func nsApp() objc.ID {
	return objc.ID(class("NSApplication")).Send(sel("sharedApplication"))
}

func isMainThread() bool {
	return objc.Send[bool](objc.ID(class("NSThread")), sel("isMainThread"))
}

// setupApp makes the process a regular app (Dock icon, menu bar), installs
// the main menu and sets the app delegate. NSApp holds the delegate
// weakly; the caller keeps it alive.
func setupApp(delegate objc.ID) error {
	app := nsApp()
	if !objc.Send[bool](app, sel("setActivationPolicy:"), activationPolicyRegular) {
		return errors.New("setActivationPolicy:NSApplicationActivationPolicyRegular returned NO")
	}
	menu := newMainMenu()
	app.Send(sel("setMainMenu:"), menu)
	release(menu) // NSApp retains it
	app.Send(sel("setDelegate:"), delegate)
	return nil
}

// newMainMenu returns a main menu (+1) with an app menu holding
// "Quit <name>" (⌘Q). The system shows the process name in the menu bar.
func newMainMenu() objc.ID {
	name := goString(objc.ID(class("NSProcessInfo")).Send(sel("processInfo")).Send(sel("processName")))
	menu := alloc("NSMenu", "init")
	appItem := alloc("NSMenuItem", "init")
	appMenu := alloc("NSMenu", "initWithTitle:", nsString(name))
	quit := alloc("NSMenuItem", "initWithTitle:action:keyEquivalent:",
		nsString("Quit "+name), sel("terminate:"), nsString("q"))
	appMenu.Send(sel("addItem:"), quit)
	appItem.Send(sel("setSubmenu:"), appMenu)
	menu.Send(sel("addItem:"), appItem)
	// The parents retain their children.
	release(quit)
	release(appMenu)
	release(appItem)
	return menu
}

// activate brings the app to the front. activateIgnoringOtherApps: is
// deprecated since macOS 14, so activate is used where it exists.
func activate() {
	app := nsApp()
	if objc.Send[bool](app, sel("respondsToSelector:"), sel("activate")) {
		app.Send(sel("activate"))
	} else {
		app.Send(sel("activateIgnoringOtherApps:"), true)
	}
}

// runApp runs [NSApp run] on the main thread. It returns after stopApp, or
// never if the app terminates (terminate: exits the process).
func runApp() { nsApp().Send(sel("run")) }

// stopApp makes [NSApp run] return. stop: only takes effect after the next
// event has been processed, so an application-defined dummy event is
// posted at the front of the queue.
func stopApp() {
	app := nsApp()
	app.Send(sel("stop:"), objc.ID(0))
	ev := objc.ID(class("NSEvent")).Send(
		sel("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
		uint(eventTypeApplicationDefined), NSPoint{}, uint(0), float64(0), 0, objc.ID(0), int16(0), 0, 0)
	if ev == 0 {
		panic("otherEventWithType: returned nil")
	}
	app.Send(sel("postEvent:atStart:"), ev, true)
}

// performOnMain sends [obj performSelectorOnMainThread:selector
// withObject:nil waitUntilDone:NO], which wakes [NSApp run]. It may be
// called from any thread that has an autorelease pool in place.
func performOnMain(obj objc.ID, selector string) {
	obj.Send(sel("performSelectorOnMainThread:withObject:waitUntilDone:"), sel(selector), objc.ID(0), false)
}

// onWorkerThread runs f on a new goroutine locked to its own OS thread,
// inside an autorelease pool that is created and drained on that thread.
func onWorkerThread(f func()) {
	go func() {
		runtime.LockOSThread()
		withPool(f)
	}()
}

// startTicker sends tick: to target on the main thread every interval,
// from a worker thread, forever. Each send has its own pool.
func startTicker(target objc.ID, every time.Duration) {
	go func() {
		runtime.LockOSThread()
		for range time.Tick(every) {
			withPool(func() { performOnMain(target, "tick:") })
		}
	}()
}
