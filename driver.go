package gogui

import "os"

// newDriver creates the platform driver: newPlatformDriver, which lives in
// the driver-selection files driver_darwin.go and driver_other.go (the
// only build-tagged files in this package). It returns an error if the
// platform has no backend that can start; Run returns that error. The
// tests replace it.
var newDriver = newPlatformDriver

// debugChecks turns on the UI-thread checks (GOGUI_DEBUG=1). The tests set
// it.
var debugChecks = os.Getenv("GOGUI_DEBUG") == "1"
