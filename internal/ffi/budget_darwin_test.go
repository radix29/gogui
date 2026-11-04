package ffi

// purego/objc creates a callback (the block dispose callback) in its init.
// The appkit driver will import it, so the darwin test binary does too, and
// TestCallbackBudget's child measures the slots a darwin driver really has
// (PLAN.md §11.1 S0.10a).
import _ "github.com/ebitengine/purego/objc"
