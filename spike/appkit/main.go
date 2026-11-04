//go:build darwin

// Command appkit is spike A of PLAN.md §11.1 (S0.8): AppKit through
// purego, without cgo. It opens a window with a click counter and a
// "ticks" label that a worker thread updates once a second through
// performSelectorOnMainThread:. -selftest checks classes, ivars and
// struct passing (S0.8b), then the window and the event loop (S0.8c), and
// exits 0 or 1. S1.1 deletes the spike.
//
//	CGO_ENABLED=0 go run ./spike/appkit
//	CGO_ENABLED=0 go run ./spike/appkit -selftest
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"
)

// AppKit must run on the main thread. Locking in init keeps the main
// goroutine on it.
func init() { runtime.LockOSThread() }

func main() {
	selftest := flag.Bool("selftest", false, "run the self-test checks and exit 0 (all passed) or 1")
	flag.Parse()
	if *selftest {
		if !runSelfTest(os.Stdout) {
			os.Exit(1)
		}
		return
	}
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, "appkit:", err)
		os.Exit(1)
	}
	// Closing the window terminates the app, and with it the process,
	// from inside run.
	runApp()
}

// setup loads the runtime, defines the classes, sets up the app and opens
// the window. The ticker starts once the app has finished launching.
func setup() (err error) {
	if err := loadRuntime(); err != nil {
		return err
	}
	withPool(func() {
		nsApp()
		if err = defineClasses(); err != nil {
			return
		}
		a := &appPeer{}
		// The delegate object lives for the process.
		if err = setupApp(newPeerObject(appDelegateClass, a)); err != nil {
			return
		}
		u := newUI()
		a.launched = func() { startTicker(u.target, time.Second) }
	})
	return err
}
