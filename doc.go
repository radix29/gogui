// Package gogui is a cross-platform desktop GUI toolkit with a macOS look
// and feel. Apps import only this package: the same code builds on macOS,
// Windows and Linux with CGO_ENABLED=0 and without build tags.
//
// # Running an app
//
// A program calls Run once, from main. Run starts the UI event loop on the
// main OS thread, calls the ready callback with the App and blocks until
// App.Quit. The ready callback builds the UI: it creates windows with
// App.NewWindow, fills them with widgets and shows them. Run returns nil
// after Quit, or an error when the platform can't start (for example, GTK 4
// missing on Linux); it never calls os.Exit.
//
// As on macOS, closing the last window does not quit the app. An app with
// one main window quits from its OnClose handler:
//
//	win.OnClose(app.Quit)
//
// # Windows and widgets
//
// A Window is created hidden and holds one content widget, set with
// Window.SetContent, which it lays out to fill its content area. Widgets are
// made by the New… functions (NewLabel, NewButton, NewTextField …) and by
// VStack, HStack and Form. A widget is in one place at a time: putting it
// into another stack, form, box, scroll view or window moves it there.
// Stack.Remove and Stack.Clear take widgets out without destroying them.
//
// Setters return their receiver, so calls chain:
// NewButton("OK").SetDefault(true). Getters have no Get prefix. The On…
// methods subscribe a handler and return an Unsubscribe. Handlers report
// what the user and the OS change; setters never call them.
//
// The setters every widget shares (SetEnabled, SetHidden, SetTooltip,
// SetControlSize and Focus) are defined once on an embedded generic type,
// so go doc shows them returning T: T is always the widget's own type
// (*Button for a Button).
//
// # Lifetime
//
// A widget lives until it is destroyed, and taking it out of a window or
// container does not destroy it: content replaced with Window.SetContent,
// Box.SetContent or ScrollView.SetDocument, or removed from a stack, can
// be shown again. Widget.Destroy frees a widget for good: it takes it out
// of its parent or window and destroys it with everything in it, children
// first. An app that swaps panels destroys the ones it no longer needs.
// Closing a window destroys its widget tree, and when Run returns it
// destroys every widget still alive.
//
// A destroyed widget can't be used again: its setters, its On… methods
// and its actions panic, and so does placing it anywhere or placing a
// widget in it. Its getters still return the last values. A handler that
// destroys its own widget ends the event: the handlers after it are not
// called. A closed window drops its content and handlers.
//
// # Layout
//
// Geometry is in logical points with a top-left origin: Y grows downwards,
// and the platform layer scales points to pixels for the display. VStack and
// HStack place their children in a column or a row, a given spacing apart;
// Stack.SetPadding adds room around them, and a Spacer takes the leftover
// space. Each widget either keeps its natural size or expands along an axis,
// as its type's documentation says (a TextField expands horizontally, a
// Button doesn't); a stack expands when one of its children does. Form lays
// out labelled rows as macOS settings do. A window lays its content out again
// whenever it is resized or a widget in it changes size, and it never gets
// smaller than its content's minimum size.
//
// # Threading
//
// Every callback (the ready callback, event handlers, menu actions, an
// alert's done) runs on the UI thread, and widgets, windows and the App may
// be used only there. Other goroutines hand work to the UI thread with Do,
// which queues a function and returns at once, or DoSync, which waits for
// it (re-raising a panic in the function as a *PanicError). With the
// environment variable GOGUI_DEBUG=1 set, using the toolkit off
// the UI thread panics with the name of the method, and on Windows and Linux
// every frame whose work takes longer than a 120 Hz frame (8.3 ms) is logged
// with its split into the frame function (animations), layout, paint and
// submit.
//
// # Appearance
//
// The semantic colors (ColorLabel, ColorControlAccent, ColorSystemBlue …)
// mirror NSColor: they resolve against the current appearance every time
// they are drawn, so an app that uses them follows light and dark mode, the
// accent color and increased contrast without doing anything. RGB8 and
// RGBA make concrete colors. The zero Font is the system font at the
// control size's default size: SF Pro on macOS and Inter, bundled with the
// toolkit, on Windows and Linux. App.SetAppearance forces light or dark,
// App.SetAccentColor overrides the accent, and App.OnAppearanceChange
// reports a switch between light and dark.
//
// Where the OS settings come from: on macOS, the system appearance; on
// Windows, the Settings app's app mode, the accent color, a high-contrast
// theme (as increased contrast), "Animation effects" (as Reduce Motion) and
// "Transparency effects" (as Reduce Transparency); on Linux, the XDG settings
// portal (color-scheme, accent-color, contrast, reduced-motion) and the
// GtkSettings as a fallback.
//
// # Menus and shortcuts
//
// App.SetMenuBar installs a menu bar built with MenuBar, Menu, MenuItem and
// the standard AppMenu, EditMenu and WindowMenu. On macOS it is the menu bar
// at the top of the screen. On Windows and Linux every titled window draws it
// in its title bar, next to the window buttons, as a strip of menu titles:
// Alt or F10 focuses the strip for the keyboard, and the shortcuts work while
// every menu is closed. An app that never calls SetMenuBar shows no strip
// there.
//
// Shortcuts are written with KeyPrimary, which means ⌘ on macOS and Ctrl on
// Windows and Linux, so Shortcut(KeyPrimary, 'S') is ⌘S on a Mac and Ctrl+S
// elsewhere. The Shortcut… functions return the standard ones.
//
// # Alerts
//
// Alert.Show shows a modal alert and calls its done function with the
// chosen button: as a sheet of a window, or app-modal without one. Return
// presses the default (first) button and Escape the button Alert.Cancel
// names, by default the one titled "Cancel".
//
// # Platforms
//
// Three drivers implement the API; Run picks the one for the OS:
//
//   - aqua, on Windows (Win32 with Direct2D and DirectComposition, Windows 10
//     1809 or later) and Linux (a GTK 4.14 or later window, X11 or Wayland).
//     It draws every control, the title bar with its window buttons and the
//     menus itself, from the macOS Sonoma theme, so the two look and behave
//     the same. It is not reachable by screen readers yet: the accessibility
//     bridge comes after v0.1.
//   - AppKit, on macOS 13 or later, with the native controls. It is not
//     built yet.
//   - headless, which macOS uses until the AppKit driver exists. It shows
//     nothing: Run prints one notice to stderr, the widgets keep their state,
//     and the user can't pick a menu item or answer an alert, so an app runs
//     until it calls Quit itself. Most of the package's tests and the
//     examples in this documentation run on it, on every OS.
//
// Where an identifier behaves differently on a driver, its documentation
// says so.
package gogui
