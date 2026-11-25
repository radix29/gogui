// Package win32 is the Windows shell of the aqua driver (PLAN.md §5, S2.3):
// an implementation of the internal/shell interfaces on Win32 and DWM, with
// Direct2D and DirectWrite to come in S2.4 and S2.5. All raw calls go
// through internal/shell/win32/w32.
//
// S2.3b adds the shell core:
//
//   - Run locks the calling goroutine to its OS thread, sets Per-Monitor-V2
//     DPI awareness, creates the shell's hidden listener window and runs
//     the GetMessageW loop until Quit. The listener is a hidden top-level
//     tool window rather than a message-only window (HWND_MESSAGE), because
//     message-only windows get no broadcast messages, and the appearance
//     needs WM_SETTINGCHANGE and WM_DWMCOLORIZATIONCOLORCHANGED even when
//     no surface is open.
//   - Post pushes onto a mainthread.Queue whose wake is one PostMessageW to
//     the listener per drain.
//   - One window procedure for every window of the package, created once
//     with ffi.RegisterCallback("win32.wndproc", …). It finds its target
//     through the ffi.Handle in GWLP_USERDATA (set in WM_NCCREATE from
//     lpCreateParams, which carries the handle, never a Go pointer), so
//     windows whose handle is unknown or stale go to DefWindowProcW. Panics
//     in targets are recovered there, because a panic must not unwind
//     through the native frames, and are re-raised as a *PanicError by the
//     message loop once the message has returned.
//   - Appearance reads dark mode, the accent, high contrast, client-area
//     animations, transparency, the scroll-bar setting and the caret blink
//     (GetCaretBlinkTime, SPI_GETCARETTIMEOUT); changes are
//     reported once per actual change.
//   - The text system (text.go, S2.5d) is DirectWrite's; the interim
//     fixed-width one, the interim GDI paint and the interim frame clock
//     went in S2.5d, S2.4d_3 and S2.4f.
//
// S2.3c adds surfaces (surface.go): top-level windows of the one class,
// created hidden and sized from logical points at the window's DPI, with
// the size limits in WM_GETMINMAXINFO, Resized from WM_SIZE and
// WM_DPICHANGED, CloseRequested from WM_CLOSE, Closed from WM_DESTROY,
// KeyChanged from WM_ACTIVATE, the cursor in WM_SETCURSOR, and Paint from
// the frame (or WM_PAINT). Quit closes every surface.
//
// S2.3d adds the custom chrome (chrome.go, ncinput.go): WM_NCCALCSIZE
// removes the native caption, so the drawable area is the whole window
// except the side and bottom resize frame; a 1 px DWM frame extension keeps
// the shadow, and the dark frame and rounded corners follow the appearance.
// WM_NCHITTEST answers the resize borders (the top one inside the drawable
// area), then the chrome regions, without calling a handler.
// WM_GETDPISCALEDSIZE keeps the logical size across a DPI change. Pointer
// events over the chrome regions arrive as non-client messages and are
// delivered as PointerEvents; a press on a traffic light captures the
// mouse and its default action is suppressed.
//
// S2.3e adds input (input.go, wheel.go, keyboard.go): client mouse
// messages for all five buttons with the capture rules of
// shell.PointerEvent, click counts from the OS double-click settings
// counted by the shell, event times from GetMessageTime mapped onto the
// clock; wheel notches in lines and precise deltas in points with
// synthesized gesture phases; key messages as KeyEvents (a VK table and
// the active layout's unshifted characters), WM_CHAR as TextEvents, and a
// handled key-down dropping its text and skipping DefWindowProcW.
//
// S2.4c_2 adds the rendering pipeline (gpu.go, present.go): the shell's
// D3D11, DXGI, D2D and DComp device objects, created with the first
// surface and released after the last; per surface a composition swap
// chain on a DComp target and visual with a D2D target bitmap over the back
// buffer, resized with the window and kept at its DPI; WM_PAINT clears the
// back buffer to the window background and presents; a lost device
// recreates the devices and every chain and repaints.
//
// S2.4d_2 adds the Direct2D render.Canvas (canvas.go): a Go-side state
// stack over SetTransform, axis-aligned clips and geometry-masked layers,
// fills and strokes with per-frame brush and stroke-style caches, contract
// violations ignored before they reach D2D, and an optional recorder for
// the debug checks.
//
// S2.4d_3 makes every surface paint through it: WM_PAINT clears the back
// buffer, runs SurfaceHandler.Paint on a fresh canvas in the same
// BeginDraw/EndDraw, releases the frame's resources and presents. With
// GOGUI_DEBUG=1 (Shell.debug) a recorder checks every Paint.
//
// S2.4e completes the canvas: group opacity as layers and box shadows on a
// second mask context (shadow.go), backdrop blur from a copy of the target
// (blur.go), and the shell's own images, drawn from a bitmap made on first
// draw and released with the device objects (image.go).
//
// S2.4f paces the frames (clock.go): the message loop waits in
// MsgWaitForMultipleObjectsEx on a high-resolution timer (half a refresh
// period between frames, targets on DWM's vblank grid) or the visible
// surfaces' frame-latency waitables, with input still flowing; Now is the
// performance counter. WM_SIZE and WM_DPICHANGED resize the buffers,
// paint and present before they return, also inside the modal size loop,
// where frames come from a posted message and WM_TIMER.
//
// S2.5f draws text: the canvas draws the layouts of the shell's text
// system with DrawTextLayout, from a native layout rebuilt once per layout
// and frame, in grayscale antialiasing (canvas.go).
//
// S5.5b adds popups (popup.go): owned WS_POPUP windows of a second class
// (CS_DROPSHADOW, the same window procedure) that are never activated,
// each with its own transparent-cleared composition chain painted after
// the surfaces, placed with shellkit.PlacePopup in the work area of the
// anchor's monitor at its DPI, and closed with their parent.
//
// S5.5c adds the grab (grab.go): a grab popup holds the mouse capture and
// gets the parent's keys, eats and is dismissed by an outside press, and
// is dismissed by the parent's deactivation, move, resize, a mode cancel
// or a lost capture; a popup without grab is transparent to the mouse.
//
// S5.5d adds the backdrop (backdrop.go): the small rounded corners, the
// dark-mode tint and, from Windows 11 22H2 on, outside high contrast and
// with transparency effects on, the Acrylic system backdrop behind the
// transparent chain, which Popup.Translucent reports; re-applied on every
// appearance change.
//
// Everything runs on the UI thread, the goroutine that called Run, except
// Shell.Post.
package win32
