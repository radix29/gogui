package driver

// Widget is the part shared by every widget peer.
//
// A widget has at most one parent: a Container, a Box, a ScrollView or a
// Window. Adding it to another parent first removes it from the old one.
type Widget interface {
	// SetFrame places the widget. r is in the coordinate space of the
	// parent's content area (top-left origin). The frames of all widgets
	// are computed by the shared layout engine (internal/layout).
	SetFrame(r Rect)

	// FittingSize returns the size the widget needs to show its content
	// (NSView.fittingSize), given at most max. Components of max may be
	// +Inf (see Unbounded). Widgets that usually stretch, such as text
	// fields and sliders, return their minimum comfortable size; the layout
	// engine decides about stretching.
	FittingSize(max Size) Size

	SetEnabled(enabled bool)
	SetHidden(hidden bool)
	SetControlSize(cs ControlSize)

	// SetTooltip sets the tooltip text. An empty string removes it.
	SetTooltip(text string)

	// Focus makes the widget the window's focused control (first
	// responder). It does nothing for widgets that cannot take focus.
	Focus()

	// Destroy removes the widget from its parent and releases its native
	// resources and handle. It does not destroy children; the public layer
	// owns the widget tree and destroys children first. No method may be
	// called after Destroy.
	Destroy()
}

// Container is a plain view that holds child widgets. It draws nothing.
type Container interface {
	Widget

	// SetChildren replaces the ordered list of children. The order is the
	// paint order and the keyboard focus order. Children that are no longer
	// in the list are removed but not destroyed.
	SetChildren(children []Widget)
}
