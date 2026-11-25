//go:build windows

package w32

// Constants from the Windows SDK headers (WinUser.h unless noted) for what
// S2.3 uses. The names are the SDK's.

// Window styles.
const (
	WS_OVERLAPPED   = 0x00000000
	WS_POPUP        = 0x80000000
	WS_CHILD        = 0x40000000
	WS_MINIMIZE     = 0x20000000
	WS_VISIBLE      = 0x10000000
	WS_CLIPSIBLINGS = 0x04000000
	WS_CLIPCHILDREN = 0x02000000
	WS_MAXIMIZE     = 0x01000000
	WS_CAPTION      = 0x00C00000
	WS_SYSMENU      = 0x00080000
	WS_THICKFRAME   = 0x00040000
	WS_MINIMIZEBOX  = 0x00020000
	WS_MAXIMIZEBOX  = 0x00010000

	WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME |
		WS_MINIMIZEBOX | WS_MAXIMIZEBOX
)

// Extended window styles.
const (
	WS_EX_TOOLWINDOW       = 0x00000080
	WS_EX_APPWINDOW        = 0x00040000
	WS_EX_NOREDIRECTIONBMP = 0x00200000
	WS_EX_NOACTIVATE       = 0x08000000
)

// Class styles.
const (
	CS_VREDRAW    = 0x0001
	CS_HREDRAW    = 0x0002
	CS_DBLCLKS    = 0x0008
	CS_OWNDC      = 0x0020
	CS_NOCLOSE    = 0x0200
	CS_DROPSHADOW = 0x00020000
)

// CW_USEDEFAULT for CreateWindowEx's position and size ((int)0x80000000).
const CW_USEDEFAULT = -0x80000000

// Pseudo-HWNDs: the parent of a message-only window, and SetWindowPos
// insert-after values.
const (
	HWND_MESSAGE   = ^uintptr(2) // (HWND)-3
	HWND_TOP       = 0
	HWND_BOTTOM    = 1
	HWND_TOPMOST   = ^uintptr(0) // (HWND)-1
	HWND_NOTOPMOST = ^uintptr(1) // (HWND)-2
)

// Get/SetWindowLongPtrW indices.
const (
	GWLP_WNDPROC    = -4
	GWLP_HINSTANCE  = -6
	GWLP_HWNDPARENT = -8 // the owner of a top-level window
	GWLP_ID         = -12
	GWL_STYLE       = -16
	GWL_EXSTYLE     = -20
	GWLP_USERDATA   = -21
)

// WM_MOUSEACTIVATE results.
const (
	MA_ACTIVATE   = 1
	MA_NOACTIVATE = 3
)

// Window messages.
const (
	WM_NULL                        = 0x0000
	WM_CREATE                      = 0x0001
	WM_DESTROY                     = 0x0002
	WM_MOVE                        = 0x0003
	WM_SIZE                        = 0x0005
	WM_ACTIVATE                    = 0x0006
	WM_SETFOCUS                    = 0x0007
	WM_KILLFOCUS                   = 0x0008
	WM_ENABLE                      = 0x000A
	WM_SETTEXT                     = 0x000C
	WM_PAINT                       = 0x000F
	WM_CLOSE                       = 0x0010
	WM_QUIT                        = 0x0012
	WM_ERASEBKGND                  = 0x0014
	WM_SHOWWINDOW                  = 0x0018
	WM_SETTINGCHANGE               = 0x001A
	WM_ACTIVATEAPP                 = 0x001C
	WM_CANCELMODE                  = 0x001F
	WM_SETCURSOR                   = 0x0020
	WM_MOUSEACTIVATE               = 0x0021
	WM_GETMINMAXINFO               = 0x0024
	WM_WINDOWPOSCHANGING           = 0x0046
	WM_WINDOWPOSCHANGED            = 0x0047
	WM_NCCREATE                    = 0x0081
	WM_NCDESTROY                   = 0x0082
	WM_NCCALCSIZE                  = 0x0083
	WM_NCHITTEST                   = 0x0084
	WM_NCPAINT                     = 0x0085
	WM_NCACTIVATE                  = 0x0086
	WM_NCMOUSEMOVE                 = 0x00A0
	WM_NCLBUTTONDOWN               = 0x00A1
	WM_NCLBUTTONUP                 = 0x00A2
	WM_NCLBUTTONDBLCLK             = 0x00A3
	WM_NCRBUTTONDOWN               = 0x00A4
	WM_NCRBUTTONUP                 = 0x00A5
	WM_KEYDOWN                     = 0x0100
	WM_KEYUP                       = 0x0101
	WM_CHAR                        = 0x0102
	WM_DEADCHAR                    = 0x0103
	WM_SYSKEYDOWN                  = 0x0104
	WM_SYSKEYUP                    = 0x0105
	WM_SYSCHAR                     = 0x0106
	WM_SYSDEADCHAR                 = 0x0107
	WM_UNICHAR                     = 0x0109
	WM_SYSCOMMAND                  = 0x0112
	WM_TIMER                       = 0x0113
	WM_MOUSEMOVE                   = 0x0200
	WM_LBUTTONDOWN                 = 0x0201
	WM_LBUTTONUP                   = 0x0202
	WM_LBUTTONDBLCLK               = 0x0203
	WM_RBUTTONDOWN                 = 0x0204
	WM_RBUTTONUP                   = 0x0205
	WM_RBUTTONDBLCLK               = 0x0206
	WM_MBUTTONDOWN                 = 0x0207
	WM_MBUTTONUP                   = 0x0208
	WM_MBUTTONDBLCLK               = 0x0209
	WM_MOUSEWHEEL                  = 0x020A
	WM_XBUTTONDOWN                 = 0x020B
	WM_XBUTTONUP                   = 0x020C
	WM_XBUTTONDBLCLK               = 0x020D
	WM_MOUSEHWHEEL                 = 0x020E
	WM_CAPTURECHANGED              = 0x0215
	WM_ENTERMENULOOP               = 0x0211
	WM_EXITMENULOOP                = 0x0212
	WM_ENTERSIZEMOVE               = 0x0231
	WM_EXITSIZEMOVE                = 0x0232
	WM_NCMOUSELEAVE                = 0x02A2
	WM_MOUSELEAVE                  = 0x02A3
	WM_DPICHANGED                  = 0x02E0
	WM_GETDPISCALEDSIZE            = 0x02E4
	WM_THEMECHANGED                = 0x031A
	WM_DWMCOMPOSITIONCHANGED       = 0x031E
	WM_DWMCOLORIZATIONCOLORCHANGED = 0x0320
	WM_USER                        = 0x0400
	WM_APP                         = 0x8000
)

// WM_SIZE wParam values.
const (
	SIZE_RESTORED  = 0
	SIZE_MINIMIZED = 1
	SIZE_MAXIMIZED = 2
)

// WM_SYSCOMMAND wParam values (the low four bits are the system's).
const (
	SC_SIZE     = 0xF000
	SC_MOVE     = 0xF010
	SC_MINIMIZE = 0xF020
	SC_MAXIMIZE = 0xF030
	SC_CLOSE    = 0xF060
	SC_RESTORE  = 0xF120
)

// WM_ACTIVATE wParam low word.
const (
	WA_INACTIVE    = 0
	WA_ACTIVE      = 1
	WA_CLICKACTIVE = 2
)

// WM_NCHITTEST results.
const (
	HTERROR       = -2
	HTTRANSPARENT = -1
	HTNOWHERE     = 0
	HTCLIENT      = 1
	HTCAPTION     = 2
	HTSYSMENU     = 3
	HTMINBUTTON   = 8
	HTMAXBUTTON   = 9
	HTLEFT        = 10
	HTRIGHT       = 11
	HTTOP         = 12
	HTTOPLEFT     = 13
	HTTOPRIGHT    = 14
	HTBOTTOM      = 15
	HTBOTTOMLEFT  = 16
	HTBOTTOMRIGHT = 17
	HTBORDER      = 18
	HTCLOSE       = 20
)

// WM_NCCALCSIZE result flag.
const WVR_VALIDRECTS = 0x0400

// Mouse-message key state (wParam), XBUTTON numbers and the wheel unit.
const (
	MK_LBUTTON  = 0x0001
	MK_RBUTTON  = 0x0002
	MK_SHIFT    = 0x0004
	MK_CONTROL  = 0x0008
	MK_MBUTTON  = 0x0010
	MK_XBUTTON1 = 0x0020
	MK_XBUTTON2 = 0x0040

	XBUTTON1 = 0x0001
	XBUTTON2 = 0x0002

	WHEEL_DELTA = 120

	// SPI_GETWHEELSCROLLLINES value: one notch scrolls a page.
	WHEEL_PAGESCROLL = 0xFFFFFFFF
)

// TrackMouseEvent flags.
const (
	TME_HOVER     = 0x00000001
	TME_LEAVE     = 0x00000002
	TME_NONCLIENT = 0x00000010
	TME_CANCEL    = 0x80000000

	HOVER_DEFAULT = 0xFFFFFFFF
)

// PeekMessageW flags.
const (
	PM_NOREMOVE = 0x0000
	PM_REMOVE   = 0x0001
	PM_NOYIELD  = 0x0002
)

// ShowWindow commands.
const (
	SW_HIDE            = 0
	SW_SHOWNORMAL      = 1
	SW_SHOWMINIMIZED   = 2
	SW_SHOWMAXIMIZED   = 3
	SW_MAXIMIZE        = 3
	SW_SHOWNOACTIVATE  = 4
	SW_SHOW            = 5
	SW_MINIMIZE        = 6
	SW_SHOWMINNOACTIVE = 7
	SW_SHOWNA          = 8
	SW_RESTORE         = 9
)

// SetWindowPos flags.
const (
	SWP_NOSIZE         = 0x0001
	SWP_NOMOVE         = 0x0002
	SWP_NOZORDER       = 0x0004
	SWP_NOREDRAW       = 0x0008
	SWP_NOACTIVATE     = 0x0010
	SWP_FRAMECHANGED   = 0x0020
	SWP_SHOWWINDOW     = 0x0040
	SWP_HIDEWINDOW     = 0x0080
	SWP_NOCOPYBITS     = 0x0100
	SWP_NOOWNERZORDER  = 0x0200
	SWP_NOSENDCHANGING = 0x0400
)

// WINDOWPLACEMENT flags.
const (
	WPF_SETMINPOSITION       = 0x0001
	WPF_RESTORETOMAXIMIZED   = 0x0002
	WPF_ASYNCWINDOWPLACEMENT = 0x0004
)

// Monitors: MonitorFromWindow/Point flags, MONITORINFO.dwFlags and
// GetDpiForMonitor's MONITOR_DPI_TYPE (ShellScalingApi.h).
const (
	MONITOR_DEFAULTTONULL    = 0
	MONITOR_DEFAULTTOPRIMARY = 1
	MONITOR_DEFAULTTONEAREST = 2

	MONITORINFOF_PRIMARY = 1

	MDT_EFFECTIVE_DPI = 0
	MDT_ANGULAR_DPI   = 1
	MDT_RAW_DPI       = 2
)

// DPI. The DPI_AWARENESS_CONTEXT values are pseudo-handles ((HANDLE)-n).
const (
	USER_DEFAULT_SCREEN_DPI = 96

	DPI_AWARENESS_CONTEXT_UNAWARE              = ^uintptr(0) // -1
	DPI_AWARENESS_CONTEXT_SYSTEM_AWARE         = ^uintptr(1) // -2
	DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE    = ^uintptr(2) // -3
	DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = ^uintptr(3) // -4
)

// GetSystemMetrics(ForDpi) indices.
const (
	SM_CXSCREEN       = 0
	SM_CYSCREEN       = 1
	SM_CYCAPTION      = 4
	SM_CXFRAME        = 32
	SM_CYFRAME        = 33
	SM_CXSIZEFRAME    = SM_CXFRAME
	SM_CYSIZEFRAME    = SM_CYFRAME
	SM_CXDOUBLECLK    = 36
	SM_CYDOUBLECLK    = 37
	SM_CXDRAG         = 68
	SM_CYDRAG         = 69
	SM_CXPADDEDBORDER = 92
)

// SystemParametersInfoW actions and the HIGHCONTRAST flag.
const (
	SPI_GETWORKAREA            = 0x0030
	SPI_GETHIGHCONTRAST        = 0x0042
	SPI_GETWHEELSCROLLLINES    = 0x0068
	SPI_GETWHEELSCROLLCHARS    = 0x006C
	SPI_GETCLIENTAREAANIMATION = 0x1042
	SPI_GETCARETTIMEOUT        = 0x2022

	HCF_HIGHCONTRASTON = 0x00000001
)

// Standard cursors for LoadCursorW (MAKEINTRESOURCE ids, hInstance 0).
const (
	IDC_ARROW    = 32512
	IDC_IBEAM    = 32513
	IDC_WAIT     = 32514
	IDC_CROSS    = 32515
	IDC_SIZENWSE = 32642
	IDC_SIZENESW = 32643
	IDC_SIZEWE   = 32644
	IDC_SIZENS   = 32645
	IDC_SIZEALL  = 32646
	IDC_NO       = 32648
	IDC_HAND     = 32649
)

// MapVirtualKeyExW map types.
const (
	MAPVK_VK_TO_VSC    = 0
	MAPVK_VSC_TO_VK    = 1
	MAPVK_VK_TO_CHAR   = 2
	MAPVK_VSC_TO_VK_EX = 3
)

// TOUNICODE_NOSTATE is the ToUnicodeEx flag (bit 2) that leaves the
// keyboard state, a pending dead key included, alone; from Windows 10
// 1607 on.
const TOUNICODE_NOSTATE = 0x4

// Virtual keys for the VK table of S2.3e and the modifier state. Letters
// and digits are their ASCII codes ("A" to "Z", "0" to "9").
const (
	VK_LBUTTON    = 0x01
	VK_RBUTTON    = 0x02
	VK_MBUTTON    = 0x04
	VK_BACK       = 0x08
	VK_TAB        = 0x09
	VK_CLEAR      = 0x0C
	VK_RETURN     = 0x0D
	VK_SHIFT      = 0x10
	VK_CONTROL    = 0x11
	VK_MENU       = 0x12
	VK_PAUSE      = 0x13
	VK_CAPITAL    = 0x14
	VK_ESCAPE     = 0x1B
	VK_SPACE      = 0x20
	VK_PRIOR      = 0x21
	VK_NEXT       = 0x22
	VK_END        = 0x23
	VK_HOME       = 0x24
	VK_LEFT       = 0x25
	VK_UP         = 0x26
	VK_RIGHT      = 0x27
	VK_DOWN       = 0x28
	VK_SNAPSHOT   = 0x2C
	VK_INSERT     = 0x2D
	VK_DELETE     = 0x2E
	VK_HELP       = 0x2F
	VK_LWIN       = 0x5B
	VK_RWIN       = 0x5C
	VK_APPS       = 0x5D
	VK_NUMPAD0    = 0x60
	VK_NUMPAD9    = 0x69
	VK_MULTIPLY   = 0x6A
	VK_ADD        = 0x6B
	VK_SEPARATOR  = 0x6C
	VK_SUBTRACT   = 0x6D
	VK_DECIMAL    = 0x6E
	VK_DIVIDE     = 0x6F
	VK_F1         = 0x70
	VK_F4         = 0x73
	VK_F10        = 0x79
	VK_F12        = 0x7B
	VK_F24        = 0x87
	VK_NUMLOCK    = 0x90
	VK_SCROLL     = 0x91
	VK_LSHIFT     = 0xA0
	VK_RSHIFT     = 0xA1
	VK_LCONTROL   = 0xA2
	VK_RCONTROL   = 0xA3
	VK_LMENU      = 0xA4
	VK_RMENU      = 0xA5
	VK_PROCESSKEY = 0xE5 // the key went to the IME
	VK_PACKET     = 0xE7 // a Unicode character from SendInput
)

// DWM window attributes and values (dwmapi.h).
const (
	DWMWA_USE_IMMERSIVE_DARK_MODE  = 20
	DWMWA_WINDOW_CORNER_PREFERENCE = 33 // Windows 11 only
	DWMWA_SYSTEMBACKDROP_TYPE      = 38 // Windows 11 22H2 (build 22621) on

	DWMWCP_DEFAULT    = 0
	DWMWCP_DONOTROUND = 1
	DWMWCP_ROUND      = 2
	DWMWCP_ROUNDSMALL = 3

	DWMSBT_AUTO            = 0
	DWMSBT_NONE            = 1
	DWMSBT_MAINWINDOW      = 2 // Mica
	DWMSBT_TRANSIENTWINDOW = 3 // Acrylic
	DWMSBT_TABBEDWINDOW    = 4
)

// Window capture (S2.4d_3, the tests' PrintWindow cross-check): the
// PrintWindow flag (WinUser.h) and the DIB constants (wingdi.h).
const (
	PW_CLIENTONLY        = 0x00000001
	PW_RENDERFULLCONTENT = 0x00000002
	BI_RGB               = 0
	DIB_RGB_COLORS       = 0
)

// Waiting (S2.4f): MsgWaitForMultipleObjectsEx's wake mask and flag
// (WinUser.h), the wait results and INFINITE (WinBase.h), the most handles
// it takes besides the message queue, and the waitable-timer flag and
// access (synchapi.h, winnt.h).
const (
	QS_ALLINPUT         = 0x04FF
	MWMO_INPUTAVAILABLE = 0x0004

	INFINITE      = 0xFFFFFFFF
	WAIT_OBJECT_0 = 0x00000000
	WAIT_TIMEOUT  = 0x00000102
	WAIT_FAILED   = 0xFFFFFFFF

	MAXIMUM_WAIT_OBJECTS = 64

	CREATE_WAITABLE_TIMER_HIGH_RESOLUTION = 0x00000002
	TIMER_ALL_ACCESS                      = 0x001F0003
)

// The clipboard (S5.2a): the Unicode text format (WinUser.h) and the
// movable global memory it travels in (WinBase.h).
const (
	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002
)

// The bounds of a USER timer's elapse in ms (WinUser.h, S5.2c): SetTimer
// raises a shorter one to the minimum and a longer one to the maximum.
const (
	USER_TIMER_MINIMUM = 0x0000000A
	USER_TIMER_MAXIMUM = 0x7FFFFFFF
)

// The IME messages (WinUser.h, S5.3e). WM_IME_STARTCOMPOSITION …
// WM_IME_COMPOSITION share the WM_IME_KEYLAST range of the key messages.
const (
	WM_IME_STARTCOMPOSITION = 0x010D
	WM_IME_ENDCOMPOSITION   = 0x010E
	WM_IME_COMPOSITION      = 0x010F
	WM_IME_SETCONTEXT       = 0x0281
	WM_IME_NOTIFY           = 0x0282
	WM_IME_CONTROL          = 0x0283
	WM_IME_COMPOSITIONFULL  = 0x0284
	WM_IME_SELECT           = 0x0285
	WM_IME_CHAR             = 0x0286
	WM_IME_REQUEST          = 0x0288
	WM_IME_KEYDOWN          = 0x0290
	WM_IME_KEYUP            = 0x0291
)

// The parts of a composition (imm.h): the GCS_* flags in the lParam of
// WM_IME_COMPOSITION, and the index of ImmGetCompositionStringW.
const (
	GCS_COMPREADSTR      = 0x0001
	GCS_COMPREADATTR     = 0x0002
	GCS_COMPREADCLAUSE   = 0x0004
	GCS_COMPSTR          = 0x0008
	GCS_COMPATTR         = 0x0010
	GCS_COMPCLAUSE       = 0x0020
	GCS_CURSORPOS        = 0x0080
	GCS_DELTASTART       = 0x0100
	GCS_RESULTREADSTR    = 0x0200
	GCS_RESULTREADCLAUSE = 0x0400
	GCS_RESULTSTR        = 0x0800
	GCS_RESULTCLAUSE     = 0x1000
)

// The attribute of each UTF-16 unit of a composition string (imm.h,
// GCS_COMPATTR). The ATTR_TARGET_* units are the clause being converted.
const (
	ATTR_INPUT               = 0x00
	ATTR_TARGET_CONVERTED    = 0x01
	ATTR_CONVERTED           = 0x02
	ATTR_TARGET_NOTCONVERTED = 0x03
	ATTR_INPUT_ERROR         = 0x04
	ATTR_FIXEDCONVERTED      = 0x05
)

// The negative results of ImmGetCompositionStringW (imm.h).
const (
	IMM_ERROR_NODATA  = -1
	IMM_ERROR_GENERAL = -2
)

// The styles of COMPOSITIONFORM and CANDIDATEFORM (imm.h, dwStyle).
const (
	CFS_DEFAULT        = 0x0000
	CFS_RECT           = 0x0001
	CFS_POINT          = 0x0002
	CFS_FORCE_POSITION = 0x0020
	CFS_CANDIDATEPOS   = 0x0040
	CFS_EXCLUDE        = 0x0080
)

// The flags of ImmAssociateContextEx (imm.h).
const (
	IACE_CHILDREN        = 0x0001
	IACE_DEFAULT         = 0x0010
	IACE_IGNORENOCONTEXT = 0x0020
)

// The actions of ImmNotifyIME and their values (imm.h).
const (
	NI_OPENCANDIDATE       = 0x0010
	NI_CLOSECANDIDATE      = 0x0011
	NI_SELECTCANDIDATESTR  = 0x0012
	NI_CHANGECANDIDATELIST = 0x0013
	NI_COMPOSITIONSTR      = 0x0015

	CPS_COMPLETE = 0x0001
	CPS_CONVERT  = 0x0002
	CPS_REVERT   = 0x0003
	CPS_CANCEL   = 0x0004
)

// The IME windows the lParam of WM_IME_SETCONTEXT asks the default IME
// window to show (imm.h). The shell clears ISC_SHOWUICOMPOSITIONWINDOW,
// since it draws the preedit itself.
const (
	ISC_SHOWUICANDIDATEWINDOW    = 0x00000001
	ISC_SHOWUIALLCANDIDATEWINDOW = 0x0000000F
	ISC_SHOWUIGUIDELINE          = 0x40000000
	ISC_SHOWUICOMPOSITIONWINDOW  = 0x80000000
	ISC_SHOWUIALL                = 0xC000000F
)
