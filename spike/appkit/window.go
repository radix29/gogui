//go:build darwin

package main

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
)

// The spike's window: a GGFlippedView content view with two labels at the
// top-left and a push button pinned to the top-right. Frames are computed
// in Go (no Auto Layout) and recomputed in windowDidResize:.

const (
	windowWidth  = 480
	windowHeight = 320
	padding      = 20
	spacing      = 8

	styleTitled         = 1 << 0 // NSWindowStyleMaskTitled
	styleClosable       = 1 << 1
	styleMiniaturizable = 1 << 2
	styleResizable      = 1 << 3
	backingBuffered     = 2 // NSBackingStoreBuffered
)

// ui is the window and its Go state. All of it is used on the main thread.
type ui struct {
	window, content         objc.ID
	clicksLabel, ticksLabel objc.ID
	button                  objc.ID
	target                  objc.ID // GGTarget: buttonClicked: and tick:
	delegate                objc.ID // GGWindowDelegate
	clicks, ticks, resizes  int
	closed                  bool
}

// targetPeer receives the button's action and the ticker's tick:.
type targetPeer struct{ u *ui }

func (p targetPeer) action(self objc.ID, cmd objc.SEL, arg objc.ID) {
	switch cmd {
	case sel("buttonClicked:"):
		p.u.clicks++
		setText(p.u.clicksLabel, fmt.Sprintf("Clicks: %d", p.u.clicks))
	case sel("tick:"):
		p.u.ticks++
		setText(p.u.ticksLabel, fmt.Sprintf("Ticks: %d", p.u.ticks))
	}
}

// windowPeer is the Go side of the GGWindowDelegate.
type windowPeer struct{ u *ui }

func (p windowPeer) action(self objc.ID, cmd objc.SEL, arg objc.ID) {
	switch cmd {
	case sel("windowDidResize:"):
		p.u.resizes++
		p.u.layout()
	case sel("windowWillClose:"):
		p.u.closed = true
	}
}

// newUI builds the window and orders it front. The window, its target and
// its delegate live for the rest of the process.
func newUI() *ui {
	u := &ui{}
	u.target = newPeerObject(targetClass, targetPeer{u})

	u.window = alloc("NSWindow", "initWithContentRect:styleMask:backing:defer:",
		rect(0, 0, windowWidth, windowHeight),
		uint(styleTitled|styleClosable|styleMiniaturizable|styleResizable),
		uint(backingBuffered), false)
	// The spike owns the window; without this, close would release it.
	u.window.Send(sel("setReleasedWhenClosed:"), false)
	u.window.Send(sel("setTitle:"), nsString("gogui spike A (S0.8c)"))

	u.content = alloc(flippedClass, "initWithFrame:", rect(0, 0, windowWidth, windowHeight))
	u.window.Send(sel("setContentView:"), u.content)
	release(u.content) // the window retains it

	u.clicksLabel = newLabel("Clicks: 0")
	u.ticksLabel = newLabel("Ticks: 0")
	u.button = objc.ID(class("NSButton")).Send(sel("buttonWithTitle:target:action:"),
		nsString("Click me"), u.target, sel("buttonClicked:"))
	for _, v := range []objc.ID{u.clicksLabel, u.ticksLabel, u.button} {
		u.content.Send(sel("addSubview:"), v) // retains the autoreleased views
	}

	u.delegate = newPeerObject(windowDelegateClass, windowPeer{u})
	u.window.Send(sel("setDelegate:"), u.delegate)
	u.layout()
	u.window.Send(sel("center"))
	u.window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	return u
}

// newLabel returns an autoreleased, non-editable NSTextField label.
func newLabel(s string) objc.ID {
	return objc.ID(class("NSTextField")).Send(sel("labelWithString:"), nsString(s))
}

func setText(field objc.ID, s string) { field.Send(sel("setStringValue:"), nsString(s)) }

func text(field objc.ID) string { return goString(field.Send(sel("stringValue"))) }

// buttonFrame returns the button's frame for content bounds b: padding
// from the top and right edges, at its fitting size. The content view is
// flipped, so y is measured from the top.
func (u *ui) buttonFrame(b NSRect) NSRect {
	bs := objc.Send[NSSize](u.button, sel("fittingSize"))
	return rect(b.Size.Width-padding-bs.Width, padding, bs.Width, bs.Height)
}

// layout places the labels top-left, stacked and filling the width left of
// the button, and the button top-right.
func (u *ui) layout() {
	b := objc.Send[NSRect](u.content, sel("bounds"))
	bf := u.buttonFrame(b)
	u.button.Send(sel("setFrame:"), bf)
	lh := objc.Send[NSSize](u.clicksLabel, sel("fittingSize")).Height
	lw := max(0, bf.Origin.X-spacing-padding)
	u.clicksLabel.Send(sel("setFrame:"), rect(padding, padding, lw, lh))
	u.ticksLabel.Send(sel("setFrame:"), rect(padding, padding+lh+spacing, lw, lh))
}
