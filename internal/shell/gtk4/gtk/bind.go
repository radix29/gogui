package gtk

// The binding tables have no build tag, so their consistency test
// (bind_test.go) runs on every host; only Load (load_linux.go) opens the
// libraries and fills them in.

// The bound C functions, one struct per C prefix. A field is set by Load
// and is nil before it (or after a failed Load). The field name is the C
// name without its prefix: Gtk.WindowNew is gtk_window_new,
// GObject.SignalConnectData is g_signal_connect_data.

// Gtk holds the gtk_* functions (libgtk-4).
var Gtk struct {
	GetMajorVersion func() uint32
	GetMinorVersion func() uint32
	GetMicroVersion func() uint32
	InitCheck       func() int32 // gboolean

	WidgetGetType         func() uintptr
	WidgetMeasure         func(w uintptr, orientation, forSize int32, minimum, natural, minBaseline, natBaseline uintptr)
	WidgetClassSetCSSName func(klass uintptr, name string)
	WidgetSizeAllocate    func(w uintptr, alloc *Allocation, baseline int32)
	WidgetGetMapped       func(w uintptr) int32 // gboolean
	WidgetGetWidth        func(w uintptr) int32
	WidgetGetHeight       func(w uintptr) int32
	// WidgetComputeBounds writes w's border box in target's coordinates.
	WidgetComputeBounds  func(w, target uintptr, out *GrapheneRect) int32 // gboolean
	WidgetAddController  func(w, controller uintptr)
	WidgetQueueDraw      func(w uintptr)
	WidgetQueueResize    func(w uintptr)
	WidgetSetSizeRequest func(w uintptr, width, height int32)
	WidgetPaintableNew   func(w uintptr) uintptr // GdkPaintable *, transfer full
	// WidgetAddTickCallback's callback is a GtkTickCallback,
	// gboolean (GtkWidget *, GdkFrameClock *, gpointer user_data); notify
	// is a GDestroyNotify for user_data. It returns the callback's id.
	WidgetAddTickCallback    func(w, callback, data, notify uintptr) uint32
	WidgetRemoveTickCallback func(w uintptr, id uint32)
	WidgetRealize            func(w uintptr)
	WidgetSetVisible         func(w uintptr, visible int32)
	// WidgetSetCursorFromName takes a CSS cursor name ("default", "text").
	WidgetSetCursorFromName func(w uintptr, name string)

	SnapshotNew             func() uintptr
	SnapshotFreeToNode      func(s uintptr) uintptr // GskRenderNode *, transfer full
	SnapshotAppendColor     func(s uintptr, color *RGBA, bounds *GrapheneRect)
	SnapshotPushRoundedClip func(s uintptr, clip *RoundedRect)
	SnapshotPop             func(s uintptr)
	// The canvas's calls (S4.1g). Save/Restore keep the
	// transform; the pushes (clip, opacity, stroke) end with Pop.
	SnapshotSave      func(s uintptr)
	SnapshotRestore   func(s uintptr)
	SnapshotTranslate func(s uintptr, p *GraphenePoint)
	SnapshotScale     func(s uintptr, x, y float32)
	// SnapshotTransformMatrix's matrix is a graphene_matrix_t, 16-byte
	// aligned (SIMD), so it lives in g_malloc memory (MatrixSize bytes).
	SnapshotTransformMatrix func(s, matrix uintptr)
	SnapshotPushOpacity     func(s uintptr, opacity float64)
	// SnapshotAppendBorder's outline is the outer edge; the widths
	// (top, right, bottom, left) go inwards.
	SnapshotAppendBorder func(s uintptr, outline *RoundedRect, widths *[4]float32, colors *[4]RGBA)
	// SnapshotAppendLinearGradient needs at least two stops, in
	// non-decreasing offsets within [0, 1]; n is a gsize.
	SnapshotAppendLinearGradient func(s uintptr, bounds *GrapheneRect, start, end *GraphenePoint, stops *ColorStop, n uintptr)
	// SnapshotPushStroke (4.14) clips what is drawn until its Pop to the
	// stroke of path; it takes its own reference to path and copies stroke.
	SnapshotPushStroke func(s, path, stroke uintptr)
	// The path calls (4.14, S4.2c). AppendFill and PushFill take a
	// GskFillRule (FillRule*); AppendStroke copies stroke. All take their
	// own reference to path.
	SnapshotAppendFill   func(s, path uintptr, rule int32, color *RGBA)
	SnapshotPushFill     func(s, path uintptr, rule int32)
	SnapshotAppendStroke func(s, path, stroke uintptr, color *RGBA)
	// The box shadows (S4.2d, since 4.0), CSS box-shadow: an outset
	// shadow outside outline only, an inset one inside it only; blur is
	// the CSS blur radius (σ = blur/2). Both copy outline and color.
	SnapshotAppendOutsetShadow func(s uintptr, outline *RoundedRect, color *RGBA, dx, dy, spread, blur float32)
	SnapshotAppendInsetShadow  func(s uintptr, outline *RoundedRect, color *RGBA, dx, dy, spread, blur float32)
	// SnapshotAppendScaledTexture (4.10, S4.2e) draws texture scaled into
	// bounds with a GskScalingFilter (ScalingFilter*); it takes its own
	// reference to texture and copies bounds.
	SnapshotAppendScaledTexture func(s, texture uintptr, filter int32, bounds *GrapheneRect)
	// The backdrop blur's calls (S4.2f, since 4.0). AppendNode takes its
	// own reference to node; PushBlur blurs what is drawn until its Pop
	// with the CSS blur radius (σ = radius/2).
	SnapshotAppendNode func(s, node uintptr)
	SnapshotPushBlur   func(s uintptr, radius float64)
	// SnapshotAppendLayout (S4.2i, since 4.0) draws layout, a PangoLayout,
	// with its top-left corner at the current origin in color, which it
	// copies; the text node keeps the glyphs and fonts, not the layout.
	SnapshotAppendLayout func(s, layout uintptr, color *RGBA)

	GestureClickNew func() uintptr
	// The caption gestures (S4.1h). GestureSetState takes an
	// EventSequence* value; GestureGetDevice returns a GdkDevice *
	// (transfer none; NULL without a sequence).
	GestureDragNew                     func() uintptr
	GestureSingleSetButton             func(g uintptr, button uint32)
	GestureSingleGetCurrentButton      func(g uintptr) uint32
	GestureSetState                    func(g uintptr, state int32) int32 // gboolean
	GestureGetDevice                   func(g uintptr) uintptr
	EventControllerGetCurrentEventTime func(c uintptr) uint32
	EventControllerReset               func(c uintptr)
	// The pointer controllers (S4.1i). EventControllerGetCurrentEventState
	// returns the GdkModifierType of the event being handled.
	EventControllerMotionNew            func() uintptr
	EventControllerGetCurrentEventState func(c uintptr) uint32
	// SettingsGetDefault returns the default display's GtkSettings
	// (transfer none).
	SettingsGetDefault func() uintptr
	// The scroll controller (S4.1j): EventControllerScrollNew takes
	// GtkEventControllerScrollFlags; GetUnit returns a GdkScrollUnit (4.8+).
	EventControllerScrollNew     func(flags uint32) uintptr
	EventControllerScrollGetUnit func(c uintptr) uint32
	// The keyboard (S4.1k). SetPropagationPhase takes a GtkPropagationPhase;
	// GetCurrentEvent returns the GdkEvent being handled (transfer none).
	EventControllerKeyNew              func() uintptr
	EventControllerSetPropagationPhase func(c uintptr, phase int32)
	EventControllerGetCurrentEvent     func(c uintptr) uintptr
	IMContextSimpleNew                 func() uintptr
	IMContextSetClientWidget           func(im, w uintptr)
	IMContextFilterKeypress            func(im, event uintptr) int32 // gboolean
	IMContextReset                     func(im uintptr)
	IMContextFocusIn                   func(im uintptr)
	IMContextFocusOut                  func(im uintptr)
	// The IME (S5.3h). IMMulticontextNew returns a GtkIMMulticontext
	// (transfer full), which picks the user's input method (IBus, Fcitx,
	// the simple context as GTK's fallback); SetUsePreedit takes a gboolean
	// (TRUE: the client draws the preedit). GetPreeditString writes the
	// preedit's UTF-8 text (the caller GLib.Frees it), its PangoAttrList
	// (the caller Pango.AttrListUnrefs it) and the cursor in characters;
	// none of the pointers may be nil. SetCursorLocation takes the caret in
	// the client widget's coordinates (a GdkRectangle, copied).
	// EventControllerKeySetIMContext gives the key controller an IM context
	// that sees each key before "key-pressed" (0 unsets it).
	IMMulticontextNew              func() uintptr
	IMContextSetUsePreedit         func(im uintptr, usePreedit int32)
	IMContextGetPreeditString      func(im uintptr, str, attrs *uintptr, cursorPos *int32)
	IMContextSetCursorLocation     func(im uintptr, area *Rectangle)
	EventControllerKeySetIMContext func(c, im uintptr)
	WidgetGetDisplay               func(w uintptr) uintptr // GdkDisplay *, transfer none
	// DragCheckThreshold reports whether the pointer moved from the start
	// past the drag threshold (gtk-dnd-drag-threshold), in w's units.
	DragCheckThreshold func(w uintptr, startX, startY, curX, curY int32) int32 // gboolean
	// WidgetComputePoint converts p from w's coordinates to target's.
	WidgetComputePoint func(w, target uintptr, p, out *GraphenePoint) int32 // gboolean

	WindowNew            func() uintptr
	WindowSetTitle       func(win uintptr, title string)
	WindowSetDefaultSize func(win uintptr, width, height int32)
	WindowSetChild       func(win, child uintptr)
	WindowSetTitlebar    func(win, titlebar uintptr)
	WindowSetResizable   func(win uintptr, resizable int32)
	WindowPresent        func(win uintptr)
	WindowMinimize       func(win uintptr)
	WindowMaximize       func(win uintptr)
	WindowUnmaximize     func(win uintptr)
	WindowIsMaximized    func(win uintptr) int32 // gboolean
	WindowIsActive       func(win uintptr) int32 // gboolean
	WindowClose          func(win uintptr)
	WindowDestroy        func(win uintptr)

	// WindowSetModal and WindowSetTransientFor are the modal surface's
	// hints (S5.7f): GTK's grab in the window group while shown, and the
	// window manager's modal dialog of parent (0: none).
	WindowSetModal        func(win uintptr, modal int32)
	WindowSetTransientFor func(win, parent uintptr)
	// WidgetSetSensitive makes w and its children take input or not (a
	// window blocked by a modal surface, S5.7f).
	WidgetSetSensitive func(w uintptr, sensitive int32)

	NativeGetRenderer func(native uintptr) uintptr // GskRenderer *, transfer none
	NativeGetSurface  func(native uintptr) uintptr // GdkSurface *, transfer none; NULL before realize
	// NativeGetSurfaceTransform writes where native's widget coordinates
	// start in its GdkSurface's (the CSD shadow's width).
	NativeGetSurfaceTransform func(native uintptr, x, y *float64)

	// The popovers of the shell's popups (S5.5e, all since 4.0 except
	// CSSProviderLoadFromString, 4.12; inside the 4.14 floor). PopoverNew
	// returns a floating GtkPopover; WidgetSetParent sinks it into the
	// parent (a GGSurface), WidgetUnparent drops that reference, so the
	// popover is unparented before its parent goes. PopoverSetChild takes
	// the popover's only child. SetHasArrow and SetAutohide take a gboolean
	// (autohide: GTK's popup grab, closing on an outside press);
	// SetPointingTo copies the anchor (a GdkRectangle in the parent's
	// coordinates; nil points at the whole parent); SetPosition takes a
	// GtkPositionType (Pos*); SetOffset moves the popover by x, y logical
	// pixels from where its position puts it. Popup shows it and Popdown
	// hides it, emitting "closed"; Present must be called from the
	// parent's size_allocate so GTK places the popover's surface.
	PopoverNew           func() uintptr
	PopoverSetChild      func(p, child uintptr)
	PopoverSetHasArrow   func(p uintptr, hasArrow int32)
	PopoverSetAutohide   func(p uintptr, autohide int32)
	PopoverSetPointingTo func(p uintptr, rect *Rectangle)
	PopoverSetPosition   func(p uintptr, position int32)
	PopoverSetOffset     func(p uintptr, x, y int32)
	PopoverPopup         func(p uintptr)
	PopoverPopdown       func(p uintptr)
	PopoverPresent       func(p uintptr)
	WidgetSetParent      func(w, parent uintptr)
	WidgetUnparent       func(w uintptr)
	// WidgetSetCanTarget takes a gboolean: FALSE lets no pointer event
	// pick w or its children (a tooltip's popover).
	WidgetSetCanTarget func(w uintptr, canTarget int32)
	// WidgetAddCSSClass copies the class name.
	WidgetAddCSSClass func(w uintptr, class string)
	// CSSProviderNew returns a GtkCssProvider (transfer full);
	// CSSProviderLoadFromString (4.12) parses CSS text, which it copies, and
	// reports errors through the provider's "parsing-error" signal only.
	// StyleContextAddProviderForDisplay (not deprecated, unlike the rest
	// of GtkStyleContext) adds a GtkStyleProvider to every widget of
	// display at a priority (StyleProviderPriority*) and takes its own
	// reference to the provider.
	CSSProviderNew                    func() uintptr
	CSSProviderLoadFromString         func(provider uintptr, css string)
	StyleContextAddProviderForDisplay func(display, provider uintptr, priority uint32)

	// The popup placement (S5.5f, GTK 4.0): WidgetSetHalign takes a
	// GtkAlign (AlignStart puts a GTK_POS_BOTTOM popover's left edge on
	// its anchor's instead of centring it); WidgetSetDirection a
	// GtkTextDirection (TextDirLTR keeps that edge the left one in an RTL
	// locale).
	WidgetSetHalign    func(w uintptr, align int32)
	WidgetSetDirection func(w uintptr, dir int32)
}

// Gdk holds the gdk_* functions (inside libgtk-4).
var Gdk struct {
	DisplayGetDefault func() uintptr
	// The frame clock's times are microseconds on g_get_monotonic_time's
	// timeline. GetRefreshInfo writes the refresh interval and the
	// predicted presentation time of the first vblank at or after base;
	// without recent presentation feedback the interval is GDK's default
	// (16667 µs) and the presentation time 0. Both pointers may be nil.
	FrameClockGetFrameTime   func(fc uintptr) int64
	FrameClockGetRefreshInfo func(fc uintptr, base int64, interval, presentation *int64)
	// The presentation feedback (S6.2d): GetFrameCounter is the counter of
	// the clock's current frame; GetTimings the timings of an earlier one
	// (borrowed; NULL once out of GDK's history); a timings' complete flag
	// (gboolean) says its fields are final, its presentation time (0: none)
	// and refresh interval are microseconds.
	FrameClockGetFrameCounter       func(fc uintptr) int64
	FrameClockGetTimings            func(fc uintptr, counter int64) uintptr
	FrameTimingsGetComplete         func(t uintptr) int32
	FrameTimingsGetPresentationTime func(t uintptr) int64
	FrameTimingsGetRefreshInterval  func(t uintptr) int64
	PaintableSnapshot               func(p, s uintptr, width, height float64)
	// ToplevelGetState returns the GdkToplevelState flags of a toplevel
	// GdkSurface (ToplevelState*).
	ToplevelGetState func(toplevel uintptr) uint32
	// ToplevelBeginMove starts the compositor's (or window manager's)
	// interactive move of a toplevel: x, y in surface coordinates, the
	// button held and the press's event time.
	ToplevelBeginMove func(toplevel, device uintptr, button int32, x, y float64, timestamp uint32)
	// The keyboard (S4.1k). DisplayTranslateKey writes the keyval of
	// keycode at the state and layout group; the out pointers may be nil.
	KeyEventGetLayout   func(event uintptr) uint32
	DisplayTranslateKey func(display uintptr, keycode, state uint32, group int32,
		keyval *uint32, effectiveGroup, level *int32, consumed *uint32) int32 // gboolean
	KeyvalToUnicode func(kv uint32) uint32
	// SurfaceGetScale is the surface's fractional scale (GTK 4.12+, so
	// required since the 4.14 floor of S4.2a).
	SurfaceGetScale  func(surface uintptr) float64
	TextureGetWidth  func(t uintptr) int32
	TextureGetHeight func(t uintptr) int32
	TextureDownload  func(t, data, stride uintptr)
	// MemoryTextureNew (S4.2e) returns a GdkTexture (transfer full) of
	// width×height pixels in a GdkMemoryFormat (Memory*) read from bytes,
	// a GBytes it takes its own reference to; stride is a gsize.
	MemoryTextureNew func(width, height, format int32, bytes, stride uintptr) uintptr
	// PopupGetPositionX and Y (S5.5f, GDK 4.0) are where a GdkPopup landed,
	// relative to its parent surface (in its surface coordinates).
	PopupGetPositionX func(popup uintptr) int32
	PopupGetPositionY func(popup uintptr) int32
	// The clipboard (S5.2b, GTK 4.0). DisplayGetClipboard returns the
	// display's GdkClipboard (transfer none); SetText copies text.
	// ReadTextAsync's callback is a GAsyncReadyCallback,
	// void (GObject *source, GAsyncResult *res, gpointer user_data), called
	// once from the main loop; cancellable may be 0. ReadTextFinish returns
	// the text (transfer full, g_free) or 0 and sets *err to a GError the
	// caller frees (GLib.ErrorFree).
	DisplayGetClipboard     func(display uintptr) uintptr
	ClipboardSetText        func(clipboard uintptr, text string)
	ClipboardReadTextAsync  func(clipboard, cancellable, callback, data uintptr)
	ClipboardReadTextFinish func(clipboard, result uintptr, err *uintptr) uintptr
}

// Gsk holds the gsk_* functions (inside libgtk-4). The node getters the
// node-tree tests use are bound in the tests only.
var Gsk struct {
	RenderNodeUnref func(n uintptr)
	// RendererRenderTexture (S4.3f, the readback) renders node into a new
	// texture of viewport's size in pixels, viewport's origin at its top
	// left corner.
	RendererRenderTexture func(r, node uintptr, viewport *GrapheneRect) uintptr // GdkTexture *, transfer full
	// Paths and strokes (4.14, S4.2b). FreeToPath frees the builder and
	// returns its GskPath (transfer full); StrokeNew returns a GskStroke
	// with the given line width and the default caps, joins and miter
	// limit.
	PathBuilderNew            func() uintptr
	PathBuilderAddRect        func(b uintptr, r *GrapheneRect)
	PathBuilderAddRoundedRect func(b uintptr, r *RoundedRect)
	PathBuilderFreeToPath     func(b uintptr) uintptr
	PathUnref                 func(p uintptr)
	StrokeNew                 func(lineWidth float32) uintptr
	StrokeFree                func(s uintptr)
	// The rest of the builder and of GskStroke (S4.2c). SetLineCap and
	// SetLineJoin take a GskLineCap and a GskLineJoin (LineCap*,
	// LineJoin*); SetDash copies its n (a gsize) lengths.
	PathBuilderMoveTo   func(b uintptr, x, y float32)
	PathBuilderLineTo   func(b uintptr, x, y float32)
	PathBuilderQuadTo   func(b uintptr, x1, y1, x2, y2 float32)
	PathBuilderCubicTo  func(b uintptr, x1, y1, x2, y2, x3, y3 float32)
	PathBuilderClose    func(b uintptr)
	StrokeSetLineCap    func(s uintptr, c int32)
	StrokeSetLineJoin   func(s uintptr, j int32)
	StrokeSetMiterLimit func(s uintptr, limit float32)
	StrokeSetDash       func(s uintptr, dash *float32, n uintptr)
	StrokeSetDashOffset func(s uintptr, offset float32)
}

// GObject holds the g_* functions of libgobject-2.0.
var GObject struct {
	TypeRegisterStaticSimple func(parent uintptr, name string, classSize uint32, classInit uintptr,
		instanceSize uint32, instanceInit uintptr, flags uint32) uintptr
	TypeFromName func(name string) uintptr
	TypeQuery    func(t uintptr, q *TypeQuery)
	TypeName     func(t uintptr) uintptr // const gchar *
	TypeParent   func(t uintptr) uintptr
	TypeClassRef func(t uintptr) uintptr

	ObjectNewWithProperties func(t uintptr, n uint32, names, values uintptr) uintptr
	ObjectRef               func(o uintptr) uintptr
	ObjectRefSink           func(o uintptr) uintptr
	ObjectUnref             func(o uintptr)
	// ObjectGetProperty copies a property into v, initialized to its type.
	ObjectGetProperty func(o uintptr, name string, v uintptr)
	// ObjectSetProperty sets a property from v (S4.3d, the linux tests).
	ObjectSetProperty func(o uintptr, name string, v uintptr)
	// ObjectClassFindProperty returns the GParamSpec of a property of the
	// class (G_OBJECT_GET_CLASS), or NULL. g_object_get_property warns on
	// an unknown name, so a property GTK may lack is looked up first (S4.3d).
	ObjectClassFindProperty func(class uintptr, name string) uintptr

	SignalConnectData        func(inst uintptr, signal string, handler, data, destroy uintptr, flags uint32) uintptr // gulong
	SignalHandlerDisconnect  func(inst, id uintptr)
	SignalHandlerIsConnected func(inst, id uintptr) int32 // gboolean
	SignalLookup             func(name string, itype uintptr) uint32
	SignalEmitv              func(instanceAndParams uintptr, id uint32, detail uint32, ret uintptr)

	ValueInit      func(v, t uintptr) uintptr
	ValueSetObject func(v, o uintptr)
	ValueSetInt    func(v uintptr, i int32)
	ValueSetDouble func(v uintptr, d float64)
	ValueGetInt    func(v uintptr) int32
	// ValueSetBoolean and ValueGetBoolean take and return a gboolean of a
	// G_TYPE_BOOLEAN value (S4.3d).
	ValueSetBoolean func(v uintptr, b int32)
	ValueGetBoolean func(v uintptr) int32
	// ValueGetString returns the string of a G_TYPE_STRING value (owned by
	// the value; NULL for none). S4.2g.
	ValueGetString func(v uintptr) uintptr
	ValueUnset     func(v uintptr)
}

// GLib holds the g_* functions of libglib-2.0. IdleAdd is the one call
// another thread may make; GLib documents it as thread-safe.
var GLib struct {
	Malloc0 func(n uintptr) uintptr
	Free    func(p uintptr)
	// BytesNew copies size bytes from data into a new GBytes (S4.2e).
	BytesNew   func(data *byte, size uintptr) uintptr
	BytesUnref func(b uintptr)
	// ErrorFree frees a GError (S4.2j; its message is read through the
	// GError mirror first).
	ErrorFree func(err uintptr)
	// SlistFree frees a GSList's cells, not their data (S5.3h).
	SlistFree func(list uintptr)

	GetMonotonicTime func() int64 // microseconds, CLOCK_MONOTONIC

	IdleAdd                   func(fn, data uintptr) uint32
	TimeoutAdd                func(intervalMS uint32, fn, data uintptr) uint32
	SourceRemove              func(id uint32) int32
	MainContextIteration      func(ctx uintptr, mayBlock int32) int32
	MainContextFindSourceByID func(ctx uintptr, id uint32) uintptr
	MainLoopNew               func(ctx uintptr, isRunning int32) uintptr
	MainLoopRun               func(loop uintptr)
	MainLoopQuit              func(loop uintptr)
	MainLoopUnref             func(loop uintptr)
}

// Pango holds the pango_* functions of libpango-1.0. Sizes, widths and
// positions are in Pango units (PangoScale per logical pixel).
var Pango struct {
	Version func() int32 // major*10000 + minor*100 + micro

	// The text system's calls (S4.2g). FontMapCreateContext returns a new
	// PangoContext (transfer full); FontMapGetFamily (1.46) the family of
	// that name or NULL (transfer none); ContextSetRoundGlyphPositions
	// (1.44) takes a gboolean.
	FontMapCreateContext          func(fm uintptr) uintptr
	FontMapGetFamily              func(fm uintptr, name string) uintptr
	ContextSetRoundGlyphPositions func(ctx uintptr, round int32)
	// FontMapListFamilies writes a g_malloc array of n families (the
	// families are the map's; the caller g_frees the array);
	// FontFamilyGetName's string is the family's. Both since 1.0.
	FontMapListFamilies func(fm uintptr, families *uintptr, n *int32)
	FontFamilyGetName   func(f uintptr) uintptr
	// A PangoFontDescription is a plain struct; the setters copy their
	// strings, FontDescriptionGetFamily's string is owned by the
	// description (NULL when unset). SetWeight takes a PangoWeight (100…
	// 1000), SetStyle a PangoStyle (PangoStyle*), SetAbsoluteSize a size in
	// Pango units of device space (logical pixels × PangoScale).
	FontDescriptionNew             func() uintptr
	FontDescriptionFromString      func(s string) uintptr
	FontDescriptionFree            func(d uintptr)
	FontDescriptionGetFamily       func(d uintptr) uintptr
	FontDescriptionSetFamily       func(d uintptr, family string)
	FontDescriptionSetAbsoluteSize func(d uintptr, size float64)
	FontDescriptionSetWeight       func(d uintptr, weight int32)
	FontDescriptionSetStyle        func(d uintptr, style int32)
	// LayoutNew returns a PangoLayout (a GObject, transfer full) of ctx.
	// SetFontDescription copies the description; SetText copies length
	// bytes of valid UTF-8 without NUL; SetWidth takes -1 for no wrapping,
	// SetWrap a PangoWrapMode (PangoWrap*). GetSize writes the logical
	// extents' width and height (the pointers may be nil); GetBaseline is
	// the first line's baseline from the top.
	LayoutNew                func(ctx uintptr) uintptr
	LayoutSetFontDescription func(l, desc uintptr)
	LayoutSetText            func(l uintptr, text string, length int32)
	LayoutSetWidth           func(l uintptr, width int32)
	LayoutSetWrap            func(l uintptr, wrap int32)
	LayoutGetSize            func(l uintptr, width, height *int32)
	LayoutGetBaseline        func(l uintptr) int32
	// LayoutSetAutoDir takes a gboolean: FALSE lays every paragraph out in
	// the context's base direction (LTR) instead of its first strong one.
	LayoutSetAutoDir func(l uintptr, auto int32)

	// The layout's geometry for hit-testing, carets and selection
	// (S4.2h). LayoutGetIter returns a new iterator at the first line's
	// first character (the caller frees it with LayoutIterFree);
	// LayoutIterNextLine and LayoutIterNextChar return FALSE past the last
	// line or character (NextChar also stops at each line's end, where the
	// run is NULL); LayoutIterGetLineReadonly returns the iterator's
	// PangoLayoutLine and LayoutIterGetRunReadonly its PangoLayoutRun (a
	// PangoGlyphItem, GlyphItem; NULL at a line's end), both the layout's;
	// LayoutIterGetIndex is the iterator's byte index. In layout
	// coordinates: LayoutIterGetLineYrange writes the line's top and
	// bottom, line spacing included; LayoutIterGetBaseline the line's
	// baseline (from the layout's top); LayoutIterGetLineExtents its ink and
	// logical rects (either may be nil); LayoutIterGetCharExtents the
	// logical rect of the iterator's character. LayoutLineGetStartIndex
	// and LayoutLineGetLength (1.50) are the byte range of the line's
	// content: its paragraph delimiter is not part of it.
	// LayoutLineIndexToX writes the x of index relative to the line
	// (trailing a gboolean: the grapheme's trailing edge), the line's
	// right end for an index past its runs. LayoutGetLogAttrsReadonly
	// returns the layout's n PangoLogAttrs (one per character plus one for
	// the end, LogAttrSize bytes each, LogAttr* bits; the layout's memory).
	LayoutGetIter             func(l uintptr) uintptr
	LayoutIterFree            func(it uintptr)
	LayoutIterNextLine        func(it uintptr) int32
	LayoutIterNextChar        func(it uintptr) int32
	LayoutIterGetLineReadonly func(it uintptr) uintptr
	LayoutIterGetRunReadonly  func(it uintptr) uintptr
	LayoutIterGetIndex        func(it uintptr) int32
	LayoutIterGetLineYrange   func(it uintptr, y0, y1 *int32)
	LayoutIterGetBaseline     func(it uintptr) int32
	LayoutIterGetLineExtents  func(it uintptr, ink, logical *PangoRectangle)
	LayoutIterGetCharExtents  func(it uintptr, logical *PangoRectangle)
	LayoutLineGetStartIndex   func(line uintptr) int32
	LayoutLineGetLength       func(line uintptr) int32
	LayoutLineIndexToX        func(line uintptr, index, trailing int32, x *int32)
	LayoutGetLogAttrsReadonly func(l uintptr, n *int32) uintptr

	// The app font (S4.2j). FontMapAddFontFile (1.56, optional: nil on an
	// older Pango) adds the fonts of a file to fm and returns a gboolean;
	// on FALSE it may set *err to a GError the caller frees
	// (GLib.ErrorFree).
	FontMapAddFontFile func(fm uintptr, filename string, err *uintptr) int32

	// The preedit's attributes (S5.3h). AttrListGetAttributes (1.44)
	// returns a GSList of copies of the list's attributes (transfer full:
	// each is freed with AttributeDestroy, the cells with GLib.SlistFree);
	// AttrListUnref drops a PangoAttrList. The attributes are read through
	// the PangoAttribute and PangoAttrClass mirrors.
	AttrListGetAttributes func(list uintptr) uintptr
	AttrListUnref         func(list uintptr)
	AttributeDestroy      func(attr uintptr)
}

// PangoCairo holds the pango_cairo_* functions of libpangocairo-1.0
// (S4.2g). FontMapGetDefault returns the calling thread's default
// PangoCairo font map (transfer none): the one GTK lays out and draws its
// text with on the GTK thread.
var PangoCairo struct {
	FontMapGetDefault func() uintptr
	// ContextSetFontOptions copies options (a cairo_font_options_t *) into
	// the context; its fonts use them for hinting.
	ContextSetFontOptions func(ctx, options uintptr)
}

// Cairo holds the cairo_* functions of libcairo (S4.2g): only the font
// options the text system's context is given. FontOptionsCreate never
// returns NULL (an error object instead); SetHintMetrics takes a
// cairo_hint_metrics_t (HintMetrics*).
var Cairo struct {
	FontOptionsCreate         func() uintptr
	FontOptionsSetHintMetrics func(o uintptr, hm int32)
	FontOptionsDestroy        func(o uintptr)
}

// PangoFc holds the pango_fc_* functions of libpangoft2-1.0 (S4.2j), the
// fontconfig side of the PangoCairo font map on Linux (a PangoFcFontMap).
// FontMapGetConfig (1.38) returns the map's FcConfig, NULL when it uses
// fontconfig's current one; FontMapConfigChanged makes the map reread its
// configuration (after FcConfigAppFontAddFile).
var PangoFc struct {
	FontMapGetConfig     func(fm uintptr) uintptr
	FontMapConfigChanged func(fm uintptr)
}

// Fc holds the Fc* functions of libfontconfig (S4.2j): adding the app
// font on a Pango older than 1.56. ConfigGetCurrent returns the current
// FcConfig (transfer none; it initialises fontconfig if needed);
// ConfigAppFontAddFile adds the fonts of a file to config as application
// fonts and returns an FcBool (FcFalse when the file can't be read).
var Fc struct {
	ConfigGetCurrent     func() uintptr
	ConfigAppFontAddFile func(config uintptr, file string) int32
}

type binding struct {
	fptr any
	name string
}

type library struct {
	soname   string
	bindings []binding
	// optional are the functions newer than the GTK floor (MinGTKMinor,
	// 4.14) or the Pango floor (1.52): Load leaves a field nil when the
	// library lacks it (D7: feature detection). Since S4.2a the only one is
	// pango_font_map_add_font_file (Pango 1.56, S4.2j).
	optional []binding
}

// libraries lists, in the order Load opens them, every library and every
// function bound from it. GTK comes first, so a machine without GTK 4
// gets the error that names it. Graphene is opened only: its structs are
// plain mirrors and gogui calls none of its functions.
func libraries() []library {
	return []library{
		{GTKLib, []binding{
			{&Gtk.GetMajorVersion, "gtk_get_major_version"},
			{&Gtk.GetMinorVersion, "gtk_get_minor_version"},
			{&Gtk.GetMicroVersion, "gtk_get_micro_version"},
			{&Gtk.InitCheck, "gtk_init_check"},
			{&Gtk.WidgetGetType, "gtk_widget_get_type"},
			{&Gtk.WidgetMeasure, "gtk_widget_measure"},
			{&Gtk.WidgetClassSetCSSName, "gtk_widget_class_set_css_name"},
			{&Gtk.WidgetSizeAllocate, "gtk_widget_size_allocate"},
			{&Gtk.WidgetGetMapped, "gtk_widget_get_mapped"},
			{&Gtk.WidgetGetWidth, "gtk_widget_get_width"},
			{&Gtk.WidgetGetHeight, "gtk_widget_get_height"},
			{&Gtk.WidgetComputeBounds, "gtk_widget_compute_bounds"},
			{&Gtk.WidgetAddController, "gtk_widget_add_controller"},
			{&Gtk.WidgetQueueDraw, "gtk_widget_queue_draw"},
			{&Gtk.WidgetQueueResize, "gtk_widget_queue_resize"},
			{&Gtk.WidgetSetSizeRequest, "gtk_widget_set_size_request"},
			{&Gtk.WidgetPaintableNew, "gtk_widget_paintable_new"},
			{&Gtk.WidgetAddTickCallback, "gtk_widget_add_tick_callback"},
			{&Gtk.WidgetRemoveTickCallback, "gtk_widget_remove_tick_callback"},
			{&Gtk.WidgetRealize, "gtk_widget_realize"},
			{&Gtk.WidgetSetVisible, "gtk_widget_set_visible"},
			{&Gtk.WidgetSetCursorFromName, "gtk_widget_set_cursor_from_name"},
			{&Gtk.SnapshotNew, "gtk_snapshot_new"},
			{&Gtk.SnapshotFreeToNode, "gtk_snapshot_free_to_node"},
			{&Gtk.SnapshotAppendColor, "gtk_snapshot_append_color"},
			{&Gtk.SnapshotPushRoundedClip, "gtk_snapshot_push_rounded_clip"},
			{&Gtk.SnapshotPop, "gtk_snapshot_pop"},
			{&Gtk.SnapshotSave, "gtk_snapshot_save"},
			{&Gtk.SnapshotRestore, "gtk_snapshot_restore"},
			{&Gtk.SnapshotTranslate, "gtk_snapshot_translate"},
			{&Gtk.SnapshotScale, "gtk_snapshot_scale"},
			{&Gtk.SnapshotTransformMatrix, "gtk_snapshot_transform_matrix"},
			{&Gtk.SnapshotPushOpacity, "gtk_snapshot_push_opacity"},
			{&Gtk.SnapshotAppendBorder, "gtk_snapshot_append_border"},
			{&Gtk.SnapshotAppendLinearGradient, "gtk_snapshot_append_linear_gradient"},
			{&Gtk.SnapshotPushStroke, "gtk_snapshot_push_stroke"},
			{&Gtk.SnapshotAppendFill, "gtk_snapshot_append_fill"},
			{&Gtk.SnapshotPushFill, "gtk_snapshot_push_fill"},
			{&Gtk.SnapshotAppendStroke, "gtk_snapshot_append_stroke"},
			{&Gtk.SnapshotAppendOutsetShadow, "gtk_snapshot_append_outset_shadow"},
			{&Gtk.SnapshotAppendInsetShadow, "gtk_snapshot_append_inset_shadow"},
			{&Gtk.SnapshotAppendScaledTexture, "gtk_snapshot_append_scaled_texture"},
			{&Gtk.SnapshotAppendNode, "gtk_snapshot_append_node"},
			{&Gtk.SnapshotPushBlur, "gtk_snapshot_push_blur"},
			{&Gtk.SnapshotAppendLayout, "gtk_snapshot_append_layout"},
			{&Gtk.GestureClickNew, "gtk_gesture_click_new"},
			{&Gtk.GestureDragNew, "gtk_gesture_drag_new"},
			{&Gtk.GestureSingleSetButton, "gtk_gesture_single_set_button"},
			{&Gtk.GestureSingleGetCurrentButton, "gtk_gesture_single_get_current_button"},
			{&Gtk.GestureSetState, "gtk_gesture_set_state"},
			{&Gtk.GestureGetDevice, "gtk_gesture_get_device"},
			{&Gtk.EventControllerGetCurrentEventTime, "gtk_event_controller_get_current_event_time"},
			{&Gtk.EventControllerReset, "gtk_event_controller_reset"},
			{&Gtk.EventControllerMotionNew, "gtk_event_controller_motion_new"},
			{&Gtk.EventControllerGetCurrentEventState, "gtk_event_controller_get_current_event_state"},
			{&Gtk.SettingsGetDefault, "gtk_settings_get_default"},
			{&Gtk.EventControllerScrollNew, "gtk_event_controller_scroll_new"},
			{&Gtk.EventControllerScrollGetUnit, "gtk_event_controller_scroll_get_unit"},
			{&Gtk.EventControllerKeyNew, "gtk_event_controller_key_new"},
			{&Gtk.EventControllerSetPropagationPhase, "gtk_event_controller_set_propagation_phase"},
			{&Gtk.EventControllerGetCurrentEvent, "gtk_event_controller_get_current_event"},
			{&Gtk.IMContextSimpleNew, "gtk_im_context_simple_new"},
			{&Gtk.IMContextSetClientWidget, "gtk_im_context_set_client_widget"},
			{&Gtk.IMContextFilterKeypress, "gtk_im_context_filter_keypress"},
			{&Gtk.IMContextReset, "gtk_im_context_reset"},
			{&Gtk.IMContextFocusIn, "gtk_im_context_focus_in"},
			{&Gtk.IMContextFocusOut, "gtk_im_context_focus_out"},
			{&Gtk.IMMulticontextNew, "gtk_im_multicontext_new"},
			{&Gtk.IMContextSetUsePreedit, "gtk_im_context_set_use_preedit"},
			{&Gtk.IMContextGetPreeditString, "gtk_im_context_get_preedit_string"},
			{&Gtk.IMContextSetCursorLocation, "gtk_im_context_set_cursor_location"},
			{&Gtk.EventControllerKeySetIMContext, "gtk_event_controller_key_set_im_context"},
			{&Gtk.WidgetGetDisplay, "gtk_widget_get_display"},
			{&Gtk.DragCheckThreshold, "gtk_drag_check_threshold"},
			{&Gtk.WidgetComputePoint, "gtk_widget_compute_point"},
			{&Gtk.WindowNew, "gtk_window_new"},
			{&Gtk.WindowSetTitle, "gtk_window_set_title"},
			{&Gtk.WindowSetDefaultSize, "gtk_window_set_default_size"},
			{&Gtk.WindowSetChild, "gtk_window_set_child"},
			{&Gtk.WindowSetTitlebar, "gtk_window_set_titlebar"},
			{&Gtk.WindowSetResizable, "gtk_window_set_resizable"},
			{&Gtk.WindowSetModal, "gtk_window_set_modal"},
			{&Gtk.WindowSetTransientFor, "gtk_window_set_transient_for"},
			{&Gtk.WidgetSetSensitive, "gtk_widget_set_sensitive"},
			{&Gtk.WindowPresent, "gtk_window_present"},
			{&Gtk.WindowMinimize, "gtk_window_minimize"},
			{&Gtk.WindowMaximize, "gtk_window_maximize"},
			{&Gtk.WindowUnmaximize, "gtk_window_unmaximize"},
			{&Gtk.WindowIsMaximized, "gtk_window_is_maximized"},
			{&Gtk.WindowIsActive, "gtk_window_is_active"},
			{&Gtk.WindowClose, "gtk_window_close"},
			{&Gtk.WindowDestroy, "gtk_window_destroy"},
			{&Gtk.NativeGetRenderer, "gtk_native_get_renderer"},
			{&Gtk.NativeGetSurface, "gtk_native_get_surface"},
			{&Gtk.NativeGetSurfaceTransform, "gtk_native_get_surface_transform"},
			{&Gtk.PopoverNew, "gtk_popover_new"},
			{&Gtk.PopoverSetChild, "gtk_popover_set_child"},
			{&Gtk.PopoverSetHasArrow, "gtk_popover_set_has_arrow"},
			{&Gtk.PopoverSetAutohide, "gtk_popover_set_autohide"},
			{&Gtk.PopoverSetPointingTo, "gtk_popover_set_pointing_to"},
			{&Gtk.PopoverSetPosition, "gtk_popover_set_position"},
			{&Gtk.PopoverSetOffset, "gtk_popover_set_offset"},
			{&Gtk.PopoverPopup, "gtk_popover_popup"},
			{&Gtk.PopoverPopdown, "gtk_popover_popdown"},
			{&Gtk.PopoverPresent, "gtk_popover_present"},
			{&Gtk.WidgetSetParent, "gtk_widget_set_parent"},
			{&Gtk.WidgetUnparent, "gtk_widget_unparent"},
			{&Gtk.WidgetSetCanTarget, "gtk_widget_set_can_target"},
			{&Gtk.WidgetAddCSSClass, "gtk_widget_add_css_class"},
			{&Gtk.CSSProviderNew, "gtk_css_provider_new"},
			{&Gtk.CSSProviderLoadFromString, "gtk_css_provider_load_from_string"},
			{&Gtk.StyleContextAddProviderForDisplay, "gtk_style_context_add_provider_for_display"},
			{&Gtk.WidgetSetHalign, "gtk_widget_set_halign"},
			{&Gtk.WidgetSetDirection, "gtk_widget_set_direction"},

			{&Gdk.DisplayGetDefault, "gdk_display_get_default"},
			{&Gdk.FrameClockGetFrameTime, "gdk_frame_clock_get_frame_time"},
			{&Gdk.FrameClockGetRefreshInfo, "gdk_frame_clock_get_refresh_info"},
			{&Gdk.FrameClockGetFrameCounter, "gdk_frame_clock_get_frame_counter"},
			{&Gdk.FrameClockGetTimings, "gdk_frame_clock_get_timings"},
			{&Gdk.FrameTimingsGetComplete, "gdk_frame_timings_get_complete"},
			{&Gdk.FrameTimingsGetPresentationTime, "gdk_frame_timings_get_presentation_time"},
			{&Gdk.FrameTimingsGetRefreshInterval, "gdk_frame_timings_get_refresh_interval"},
			{&Gdk.PaintableSnapshot, "gdk_paintable_snapshot"},
			{&Gdk.ToplevelGetState, "gdk_toplevel_get_state"},
			{&Gdk.ToplevelBeginMove, "gdk_toplevel_begin_move"},
			{&Gdk.KeyEventGetLayout, "gdk_key_event_get_layout"},
			{&Gdk.DisplayTranslateKey, "gdk_display_translate_key"},
			{&Gdk.KeyvalToUnicode, "gdk_keyval_to_unicode"},
			{&Gdk.SurfaceGetScale, "gdk_surface_get_scale"},
			{&Gdk.TextureGetWidth, "gdk_texture_get_width"},
			{&Gdk.TextureGetHeight, "gdk_texture_get_height"},
			{&Gdk.TextureDownload, "gdk_texture_download"},
			{&Gdk.MemoryTextureNew, "gdk_memory_texture_new"},
			{&Gdk.PopupGetPositionX, "gdk_popup_get_position_x"},
			{&Gdk.PopupGetPositionY, "gdk_popup_get_position_y"},
			{&Gdk.DisplayGetClipboard, "gdk_display_get_clipboard"},
			{&Gdk.ClipboardSetText, "gdk_clipboard_set_text"},
			{&Gdk.ClipboardReadTextAsync, "gdk_clipboard_read_text_async"},
			{&Gdk.ClipboardReadTextFinish, "gdk_clipboard_read_text_finish"},

			{&Gsk.RenderNodeUnref, "gsk_render_node_unref"},
			{&Gsk.RendererRenderTexture, "gsk_renderer_render_texture"},
			{&Gsk.PathBuilderNew, "gsk_path_builder_new"},
			{&Gsk.PathBuilderAddRect, "gsk_path_builder_add_rect"},
			{&Gsk.PathBuilderAddRoundedRect, "gsk_path_builder_add_rounded_rect"},
			{&Gsk.PathBuilderFreeToPath, "gsk_path_builder_free_to_path"},
			{&Gsk.PathUnref, "gsk_path_unref"},
			{&Gsk.StrokeNew, "gsk_stroke_new"},
			{&Gsk.StrokeFree, "gsk_stroke_free"},
			{&Gsk.PathBuilderMoveTo, "gsk_path_builder_move_to"},
			{&Gsk.PathBuilderLineTo, "gsk_path_builder_line_to"},
			{&Gsk.PathBuilderQuadTo, "gsk_path_builder_quad_to"},
			{&Gsk.PathBuilderCubicTo, "gsk_path_builder_cubic_to"},
			{&Gsk.PathBuilderClose, "gsk_path_builder_close"},
			{&Gsk.StrokeSetLineCap, "gsk_stroke_set_line_cap"},
			{&Gsk.StrokeSetLineJoin, "gsk_stroke_set_line_join"},
			{&Gsk.StrokeSetMiterLimit, "gsk_stroke_set_miter_limit"},
			{&Gsk.StrokeSetDash, "gsk_stroke_set_dash"},
			{&Gsk.StrokeSetDashOffset, "gsk_stroke_set_dash_offset"},
		}, nil},
		{GObjectLib, []binding{
			{&GObject.TypeRegisterStaticSimple, "g_type_register_static_simple"},
			{&GObject.TypeFromName, "g_type_from_name"},
			{&GObject.TypeQuery, "g_type_query"},
			{&GObject.TypeName, "g_type_name"},
			{&GObject.TypeParent, "g_type_parent"},
			{&GObject.TypeClassRef, "g_type_class_ref"},
			{&GObject.ObjectNewWithProperties, "g_object_new_with_properties"},
			{&GObject.ObjectRef, "g_object_ref"},
			{&GObject.ObjectRefSink, "g_object_ref_sink"},
			{&GObject.ObjectUnref, "g_object_unref"},
			{&GObject.ObjectGetProperty, "g_object_get_property"},
			{&GObject.ObjectSetProperty, "g_object_set_property"},
			{&GObject.ObjectClassFindProperty, "g_object_class_find_property"},
			{&GObject.SignalConnectData, "g_signal_connect_data"},
			{&GObject.SignalHandlerDisconnect, "g_signal_handler_disconnect"},
			{&GObject.SignalHandlerIsConnected, "g_signal_handler_is_connected"},
			{&GObject.SignalLookup, "g_signal_lookup"},
			{&GObject.SignalEmitv, "g_signal_emitv"},
			{&GObject.ValueInit, "g_value_init"},
			{&GObject.ValueSetObject, "g_value_set_object"},
			{&GObject.ValueSetInt, "g_value_set_int"},
			{&GObject.ValueSetDouble, "g_value_set_double"},
			{&GObject.ValueGetInt, "g_value_get_int"},
			{&GObject.ValueSetBoolean, "g_value_set_boolean"},
			{&GObject.ValueGetBoolean, "g_value_get_boolean"},
			{&GObject.ValueGetString, "g_value_get_string"},
			{&GObject.ValueUnset, "g_value_unset"},
		}, nil},
		{GLibLib, []binding{
			{&GLib.Malloc0, "g_malloc0"},
			{&GLib.Free, "g_free"},
			{&GLib.BytesNew, "g_bytes_new"},
			{&GLib.BytesUnref, "g_bytes_unref"},
			{&GLib.ErrorFree, "g_error_free"},
			{&GLib.SlistFree, "g_slist_free"},
			{&GLib.GetMonotonicTime, "g_get_monotonic_time"},
			{&GLib.IdleAdd, "g_idle_add"},
			{&GLib.TimeoutAdd, "g_timeout_add"},
			{&GLib.SourceRemove, "g_source_remove"},
			{&GLib.MainContextIteration, "g_main_context_iteration"},
			{&GLib.MainContextFindSourceByID, "g_main_context_find_source_by_id"},
			{&GLib.MainLoopNew, "g_main_loop_new"},
			{&GLib.MainLoopRun, "g_main_loop_run"},
			{&GLib.MainLoopQuit, "g_main_loop_quit"},
			{&GLib.MainLoopUnref, "g_main_loop_unref"},
		}, nil},
		{GrapheneLib, nil, nil},
		{PangoLib, []binding{
			{&Pango.Version, "pango_version"},
			{&Pango.FontMapCreateContext, "pango_font_map_create_context"},
			{&Pango.FontMapGetFamily, "pango_font_map_get_family"},
			{&Pango.ContextSetRoundGlyphPositions, "pango_context_set_round_glyph_positions"},
			{&Pango.FontMapListFamilies, "pango_font_map_list_families"},
			{&Pango.FontFamilyGetName, "pango_font_family_get_name"},
			{&Pango.FontDescriptionNew, "pango_font_description_new"},
			{&Pango.FontDescriptionFromString, "pango_font_description_from_string"},
			{&Pango.FontDescriptionFree, "pango_font_description_free"},
			{&Pango.FontDescriptionGetFamily, "pango_font_description_get_family"},
			{&Pango.FontDescriptionSetFamily, "pango_font_description_set_family"},
			{&Pango.FontDescriptionSetAbsoluteSize, "pango_font_description_set_absolute_size"},
			{&Pango.FontDescriptionSetWeight, "pango_font_description_set_weight"},
			{&Pango.FontDescriptionSetStyle, "pango_font_description_set_style"},
			{&Pango.LayoutNew, "pango_layout_new"},
			{&Pango.LayoutSetFontDescription, "pango_layout_set_font_description"},
			{&Pango.LayoutSetText, "pango_layout_set_text"},
			{&Pango.LayoutSetWidth, "pango_layout_set_width"},
			{&Pango.LayoutSetWrap, "pango_layout_set_wrap"},
			{&Pango.LayoutGetSize, "pango_layout_get_size"},
			{&Pango.LayoutGetBaseline, "pango_layout_get_baseline"},
			{&Pango.LayoutSetAutoDir, "pango_layout_set_auto_dir"},
			{&Pango.LayoutGetIter, "pango_layout_get_iter"},
			{&Pango.LayoutIterFree, "pango_layout_iter_free"},
			{&Pango.LayoutIterNextLine, "pango_layout_iter_next_line"},
			{&Pango.LayoutIterGetLineReadonly, "pango_layout_iter_get_line_readonly"},
			{&Pango.LayoutIterGetLineYrange, "pango_layout_iter_get_line_yrange"},
			{&Pango.LayoutIterGetBaseline, "pango_layout_iter_get_baseline"},
			{&Pango.LayoutLineGetStartIndex, "pango_layout_line_get_start_index"},
			{&Pango.LayoutLineGetLength, "pango_layout_line_get_length"},
			{&Pango.LayoutLineIndexToX, "pango_layout_line_index_to_x"},
			{&Pango.LayoutIterNextChar, "pango_layout_iter_next_char"},
			{&Pango.LayoutIterGetRunReadonly, "pango_layout_iter_get_run_readonly"},
			{&Pango.LayoutIterGetIndex, "pango_layout_iter_get_index"},
			{&Pango.LayoutIterGetLineExtents, "pango_layout_iter_get_line_extents"},
			{&Pango.LayoutIterGetCharExtents, "pango_layout_iter_get_char_extents"},
			{&Pango.LayoutGetLogAttrsReadonly, "pango_layout_get_log_attrs_readonly"},
			{&Pango.AttrListGetAttributes, "pango_attr_list_get_attributes"},
			{&Pango.AttrListUnref, "pango_attr_list_unref"},
			{&Pango.AttributeDestroy, "pango_attribute_destroy"},
		}, []binding{
			{&Pango.FontMapAddFontFile, "pango_font_map_add_font_file"},
		}},
		{PangoCairoLib, []binding{
			{&PangoCairo.FontMapGetDefault, "pango_cairo_font_map_get_default"},
			{&PangoCairo.ContextSetFontOptions, "pango_cairo_context_set_font_options"},
		}, nil},
		{CairoLib, []binding{
			{&Cairo.FontOptionsCreate, "cairo_font_options_create"},
			{&Cairo.FontOptionsSetHintMetrics, "cairo_font_options_set_hint_metrics"},
			{&Cairo.FontOptionsDestroy, "cairo_font_options_destroy"},
		}, nil},
		{PangoFT2Lib, []binding{
			{&PangoFc.FontMapGetConfig, "pango_fc_font_map_get_config"},
			{&PangoFc.FontMapConfigChanged, "pango_fc_font_map_config_changed"},
		}, nil},
		{FontconfigLib, []binding{
			{&Fc.ConfigGetCurrent, "FcConfigGetCurrent"},
			{&Fc.ConfigAppFontAddFile, "FcConfigAppFontAddFile"},
		}, nil},
	}
}
