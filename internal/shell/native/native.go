package native

import "errors"

// ErrNotImplemented is returned by New on an OS without a shell (macOS,
// which has its own driver, and the BSDs). It is declared on every OS,
// so callers compile everywhere; on Windows and Linux New never returns it.
var ErrNotImplemented = errors.New("gogui: no native shell for this OS (macOS has its own driver)")
