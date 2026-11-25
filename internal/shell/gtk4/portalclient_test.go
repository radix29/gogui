package gtk4

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/radix29/gogui/internal/driver"
)

// The portal client (S4.3b). The decoding is tested on its own; openPortal
// runs end to end on every host against fakeBus, a godbus connection on
// the other end of a net.Pipe that plays both the bus daemon (auth, Hello,
// AddMatch) and the portal. portalclient_linux_test.go runs the same
// client against a real session bus.

func TestDecodeValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"u", uint32(1), uint32(1)},
		{"variant u", dbus.MakeVariant(uint32(2)), uint32(2)},
		{"doubly wrapped u (Read)", dbus.MakeVariant(dbus.MakeVariant(uint32(1))), uint32(1)},
		{"(ddd) as decoded", []any{0.1, 0.2, 0.3}, [3]float64{0.1, 0.2, 0.3}},
		{"variant (ddd) as decoded", wireDDD(1, 0.5, 0), [3]float64{1, 0.5, 0}},
		{"(ddd) out of range is kept", []any{-1.0, -1.0, -1.0}, [3]float64{-1, -1, -1}},
		{"(ddi) is not a color", []any{0.1, 0.2, int32(3)}, []any{0.1, 0.2, int32(3)}},
		{"(dd) is not a color", []any{0.1, 0.2}, []any{0.1, 0.2}},
		{"ad is not a color", []float64{0.1, 0.2, 0.3}, []float64{0.1, 0.2, 0.3}},
		{"other types pass", dbus.MakeVariant("dark"), "dark"},
		{"i stays i", dbus.MakeVariant(int32(1)), int32(1)},
	}
	for _, tt := range tests {
		if got := decodeValue(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: decodeValue(%#v) = %#v, want %#v", tt.name, tt.in, got, tt.want)
		}
	}
}

// wireDDD is a (ddd) variant as godbus decodes it off the wire, a []any.
// (A local dbus.MakeVariant of a struct keeps the struct; only the fakes'
// replies and signals, which godbus encodes, use structs.)
func wireDDD(r, g, b float64) dbus.Variant {
	return dbus.MakeVariantWithSignature([]any{r, g, b}, dbus.ParseSignatureMust("(ddd)"))
}

func TestDecodeReadAll(t *testing.T) {
	got := decodeReadAll(map[string]map[string]dbus.Variant{
		portalNamespace: {
			keyColorScheme: dbus.MakeVariant(uint32(1)),
			keyAccentColor: wireDDD(0.2, 0.4, 0.6),
		},
		"org.gnome.desktop.interface": {keyContrast: dbus.MakeVariant(uint32(1))},
	})
	want := map[string]any{keyColorScheme: uint32(1), keyAccentColor: [3]float64{0.2, 0.4, 0.6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decodeReadAll = %#v, want %#v", got, want)
	}
	if got := decodeReadAll(nil); len(got) != 0 {
		t.Errorf("decodeReadAll(nil) = %#v, want empty", got)
	}
}

// The decoded values feed readAppearance as S4.3a expects them.
func TestDecodedReadAppearance(t *testing.T) {
	p := &portal{values: decodeReadAll(map[string]map[string]dbus.Variant{portalNamespace: {
		keyColorScheme:   dbus.MakeVariant(uint32(1)),
		keyAccentColor:   wireDDD(0.25, 0.5, 1),
		keyContrast:      dbus.MakeVariant(uint32(1)),
		keyReducedMotion: dbus.MakeVariant(uint32(1)),
	}})}
	a := readAppearance(portalOnly{p})
	if !a.Dark || !a.IncreaseContrast || !a.ReduceMotion {
		t.Errorf("readAppearance = %+v, want dark, contrast, reduced motion", a)
	}
	if c := a.Accent; c.R != 0.25 || c.G != 0.5 || c.B != 1 || c.A != 1 {
		t.Errorf("accent = %+v, want (0.25, 0.5, 1, 1)", c)
	}
	// The unset accent of the portal, (-1, -1, -1), keeps the default.
	p = &portal{values: decodeReadAll(map[string]map[string]dbus.Variant{portalNamespace: {
		keyAccentColor: wireDDD(-1, -1, -1),
	}})}
	if a := readAppearance(portalOnly{p}); a != readAppearance(portalOnly{}) {
		t.Errorf("unset accent: %+v, want the defaults", a)
	}
}

// portalOnly is a settingsSource over a portal without GtkSettings.
type portalOnly struct{ p *portal }

func (s portalOnly) portalValue(key string) (any, bool) { return s.p.portalValue(key) }
func (portalOnly) gtkBool(string) (bool, bool)          { return false, false }
func (portalOnly) gtkInt(string) (int32, bool)          { return 0, false }

func TestPortalApply(t *testing.T) {
	sig := func(name string, path dbus.ObjectPath, body ...any) *dbus.Signal {
		return &dbus.Signal{Name: name, Path: path, Body: body}
	}
	good := func(key string, v any) *dbus.Signal {
		return sig(portalSignal, portalPath, portalNamespace, key, dbus.MakeVariant(v))
	}
	tests := []struct {
		name string
		sig  *dbus.Signal
		ok   bool
	}{
		{"nil", nil, false},
		{"color-scheme", good(keyColorScheme, uint32(1)), true},
		{"accent", good(keyAccentColor, []any{1.0, 0.0, 0.0}), true},
		{"unknown key is stored", good("future-key", uint32(1)), true},
		{"other namespace", sig(portalSignal, portalPath, "org.gnome.desktop.interface", keyColorScheme, dbus.MakeVariant(uint32(1))), false},
		{"other member", sig(portalIface+".Other", portalPath, portalNamespace, keyColorScheme, dbus.MakeVariant(uint32(1))), false},
		{"other path", sig(portalSignal, "/elsewhere", portalNamespace, keyColorScheme, dbus.MakeVariant(uint32(1))), false},
		{"bus signal", sig("org.freedesktop.DBus.NameAcquired", "/org/freedesktop/DBus", ":1.42"), false},
		{"short body", sig(portalSignal, portalPath, portalNamespace, keyColorScheme), false},
		{"value not a variant", sig(portalSignal, portalPath, portalNamespace, keyColorScheme, uint32(1)), false},
		{"key not a string", sig(portalSignal, portalPath, portalNamespace, uint32(1), dbus.MakeVariant(uint32(1))), false},
	}
	for _, tt := range tests {
		p := &portal{values: map[string]any{}}
		if got := p.apply(tt.sig); got != tt.ok {
			t.Errorf("%s: apply = %v, want %v", tt.name, got, tt.ok)
		}
		if !tt.ok && len(p.values) != 0 {
			t.Errorf("%s: values = %v, want none", tt.name, p.values)
		}
	}
	p := &portal{values: map[string]any{keyColorScheme: uint32(0)}}
	p.apply(good(keyColorScheme, uint32(1)))
	p.apply(good(keyAccentColor, []any{1.0, 0.5, 0.0}))
	want := map[string]any{keyColorScheme: uint32(1), keyAccentColor: [3]float64{1, 0.5, 0}}
	if !reflect.DeepEqual(p.values, want) {
		t.Errorf("values after apply = %#v, want %#v", p.values, want)
	}
}

func TestPortalErrors(t *testing.T) {
	tests := []struct {
		err      error
		name     string
		answered bool
	}{
		{nil, "", false},
		{errors.New("dbus: connection closed by user"), "", false},
		{dbus.Error{Name: errUnknownMethod}, errUnknownMethod, true},
		{&dbus.Error{Name: errNotFound}, errNotFound, true},
		{dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}, "org.freedesktop.DBus.Error.AccessDenied", true},
		{dbus.Error{Name: errServiceUnknown}, errServiceUnknown, false},
		{dbus.Error{Name: errNameHasNoOwner}, errNameHasNoOwner, false},
		{dbus.Error{Name: errNoReply}, errNoReply, false},
		{dbus.Error{Name: errTimeout}, errTimeout, false},
		{dbus.Error{Name: errDisconnected}, errDisconnected, false},
	}
	for _, tt := range tests {
		if got := dbusErrorName(tt.err); got != tt.name {
			t.Errorf("dbusErrorName(%v) = %q, want %q", tt.err, got, tt.name)
		}
		if got := portalAnswered(tt.err); got != tt.answered {
			t.Errorf("portalAnswered(%v) = %v, want %v", tt.err, got, tt.answered)
		}
	}
}

func TestPortalNil(t *testing.T) {
	var p *portal
	if v, ok := p.portalValue(keyColorScheme); ok || v != nil {
		t.Errorf("nil portal: portalValue = %v, %v, want unset", v, ok)
	}
	p.Close() // no panic
}

// --- the portal side, shared with the linux test ---

// appearanceValues are what the fake portals report in the tests.
func appearanceValues() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		keyColorScheme: dbus.MakeVariant(uint32(1)),
		keyAccentColor: dbus.MakeVariant(struct{ R, G, B float64 }{0.2, 0.4, 0.6}),
		keyContrast:    dbus.MakeVariant(uint32(0)),
		// reduced-motion missing: an older portal (NotFound)
	}
}

// wantAppearanceValues is appearanceValues decoded.
var wantAppearanceValues = map[string]any{
	keyColorScheme: uint32(1),
	keyAccentColor: [3]float64{0.2, 0.4, 0.6},
	keyContrast:    uint32(0),
}

// fakePortalAll is a portal of version 2: ReadAll and ReadOne. A delay
// makes ReadAll hang; a non-nil err is its reply.
type fakePortalAll struct {
	values map[string]dbus.Variant
	delay  time.Duration
	err    *dbus.Error
}

func (f *fakePortalAll) ReadAll(namespaces []string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	time.Sleep(f.delay)
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]map[string]dbus.Variant{}
	for _, ns := range namespaces {
		if ns == portalNamespace {
			out[ns] = f.values
		}
	}
	return out, nil
}

func (f *fakePortalAll) ReadOne(ns, key string) (dbus.Variant, *dbus.Error) {
	return readFake(f.values, ns, key)
}

// fakePortalOne has ReadOne only (ReadAll gives UnknownMethod).
type fakePortalOne struct{ values map[string]dbus.Variant }

func (f *fakePortalOne) ReadOne(ns, key string) (dbus.Variant, *dbus.Error) {
	return readFake(f.values, ns, key)
}

// fakePortalRead has the deprecated, doubly wrapped Read only.
type fakePortalRead struct{ values map[string]dbus.Variant }

func (f *fakePortalRead) Read(ns, key string) (dbus.Variant, *dbus.Error) {
	v, err := readFake(f.values, ns, key)
	if err != nil {
		return v, err
	}
	return dbus.MakeVariant(v), nil
}

func readFake(values map[string]dbus.Variant, ns, key string) (dbus.Variant, *dbus.Error) {
	v, ok := values[key]
	if ns != portalNamespace || !ok {
		return dbus.Variant{}, dbus.NewError(errNotFound, []any{"Requested setting not found"})
	}
	return v, nil
}

// emitSetting emits a SettingChanged from srv.
func emitSetting(t *testing.T, srv *dbus.Conn, ns, key string, v any) {
	t.Helper()
	if err := srv.Emit(portalPath, portalSignal, ns, key, dbus.MakeVariant(v)); err != nil {
		t.Fatalf("Emit: %v", err)
	}
}

// changes counts onChange calls and lets a test wait for them.
type changes struct {
	mu sync.Mutex
	n  int
	ch chan struct{}
}

func newChanges() *changes { return &changes{ch: make(chan struct{}, 64)} }

func (c *changes) onChange() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	c.ch <- struct{}{}
}

func (c *changes) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// wait waits for one more onChange.
func (c *changes) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no onChange within 5 s")
	}
}

func checkValues(t *testing.T, p *portal, want map[string]any) {
	t.Helper()
	if p == nil {
		t.Fatal("portal is nil")
	}
	for _, key := range append(portalKeys[:], "future-key") {
		v, ok := p.portalValue(key)
		w, wok := want[key]
		if ok != wok || !reflect.DeepEqual(v, w) {
			t.Errorf("portalValue(%q) = %#v, %v, want %#v, %v", key, v, ok, w, wok)
		}
	}
}

// --- fakeBus: a bus daemon and portal in one, over a net.Pipe ---

// fakeBus serves one connection: the daemon half of the auth handshake by
// hand, then a godbus connection (srv) that answers Hello and AddMatch and
// exports the portal.
type fakeBus struct {
	srv   *dbus.Conn
	mu    sync.Mutex
	rules []string
}

func (b *fakeBus) Hello() (string, *dbus.Error) { return ":1.42", nil }

func (b *fakeBus) AddMatch(rule string) *dbus.Error {
	b.mu.Lock()
	b.rules = append(b.rules, rule)
	b.mu.Unlock()
	return nil
}

func (b *fakeBus) matchRules() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.rules...)
}

// fakeBusConfig returns a portalConfig whose dial connects to a new
// fakeBus exporting portal (nil: no portal object), and the bus once it
// is served.
func fakeBusConfig(t *testing.T, portal any) (portalConfig, <-chan *fakeBus) {
	t.Helper()
	served := make(chan *fakeBus, 1)
	cfg := defaultPortalConfig()
	cfg.timeout = 2 * time.Second
	cfg.auth = []dbus.Auth{dbus.AuthAnonymous()}
	cfg.dial = func(opts ...dbus.ConnOption) (*dbus.Conn, error) {
		client, server := net.Pipe()
		go func() {
			b, err := serveFakeBus(server, portal)
			if err != nil {
				t.Errorf("fake bus: %v", err)
				_ = server.Close()
				return
			}
			t.Cleanup(func() { _ = b.srv.Close() })
			served <- b
		}()
		return dbus.NewConn(client, opts...)
	}
	return cfg, served
}

func serveFakeBus(rwc net.Conn, portal any) (*fakeBus, error) {
	// The client's side of SASL: a NUL, AUTH (answered with the one
	// mechanism offered), AUTH ANONYMOUS, BEGIN. Read byte by byte so
	// nothing after BEGIN is consumed here.
	nul := make([]byte, 1)
	if _, err := io.ReadFull(rwc, nul); err != nil {
		return nil, err
	}
	for {
		line, err := readAuthLine(rwc)
		if err != nil {
			return nil, err
		}
		switch {
		case line == "AUTH":
			_, err = io.WriteString(rwc, "REJECTED ANONYMOUS\r\n")
		case strings.HasPrefix(line, "AUTH ANONYMOUS"):
			_, err = io.WriteString(rwc, "OK 0123456789abcdef0123456789abcdef\r\n")
		case line == "BEGIN":
			return startFakeBus(rwc, portal)
		default:
			_, err = io.WriteString(rwc, "ERROR\r\n")
		}
		if err != nil {
			return nil, err
		}
	}
}

func readAuthLine(r io.Reader) (string, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		if b[0] == '\n' {
			return strings.TrimSuffix(string(line), "\r"), nil
		}
		line = append(line, b[0])
	}
}

// startFakeBus wraps rwc in a godbus connection. godbus only starts reading
// messages at the end of its own (client) Auth, so that Auth runs against
// a scripted daemon: its writes are dropped, its reads answered.
func startFakeBus(rwc net.Conn, portal any) (*fakeBus, error) {
	srv, err := dbus.NewConn(&scriptedAuth{
		Conn:   rwc,
		script: bytes.NewBufferString("REJECTED ANONYMOUS\r\nOK fedcba9876543210fedcba9876543210\r\n"),
	})
	if err != nil {
		return nil, err
	}
	b := &fakeBus{srv: srv}
	if err := srv.Export(b, "/org/freedesktop/DBus", "org.freedesktop.DBus"); err != nil {
		return nil, err
	}
	if portal != nil {
		if err := srv.Export(portal, portalPath, portalIface); err != nil {
			return nil, err
		}
	}
	// Export first: Auth starts reading, and the client sends Hello at once.
	if err := srv.Auth([]dbus.Auth{dbus.AuthAnonymous()}); err != nil {
		return nil, err
	}
	return b, nil
}

// scriptedAuth answers godbus's client Auth from script and drops its
// writes up to BEGIN; after that it is the connection itself.
type scriptedAuth struct {
	net.Conn
	script *bytes.Buffer
	mu     sync.Mutex
	begun  bool
	sent   []byte
}

func (s *scriptedAuth) Read(p []byte) (int, error) {
	s.mu.Lock()
	if s.script.Len() > 0 {
		defer s.mu.Unlock()
		return s.script.Read(p) // all of it at once, into Auth's bufio
	}
	s.mu.Unlock()
	return s.Conn.Read(p)
}

func (s *scriptedAuth) Write(p []byte) (int, error) {
	s.mu.Lock()
	if !s.begun {
		s.sent = append(s.sent, p...)
		s.begun = bytes.HasSuffix(s.sent, []byte("BEGIN\r\n"))
		s.mu.Unlock()
		return len(p), nil
	}
	s.mu.Unlock()
	return s.Conn.Write(p)
}

// --- openPortal end to end ---

func TestOpenPortalReadAll(t *testing.T) {
	cfg, served := fakeBusConfig(t, &fakePortalAll{values: appearanceValues()})
	c := newChanges()
	p, err := openPortal(cfg, c.onChange)
	if err != nil {
		t.Fatalf("openPortal: %v", err)
	}
	defer p.Close()
	checkValues(t, p, wantAppearanceValues)
	b := <-served

	// The match rule: the portal's SettingChanged for the namespace only.
	rules := b.matchRules()
	if len(rules) != 1 {
		t.Fatalf("match rules = %q, want one", rules)
	}
	for _, part := range []string{
		"type='signal'", "sender='" + portalDest + "'", "path='" + string(portalPath) + "'",
		"interface='" + portalIface + "'", "member='SettingChanged'", "arg0='" + portalNamespace + "'",
	} {
		if !strings.Contains(rules[0], part) {
			t.Errorf("match rule %q lacks %s", rules[0], part)
		}
	}

	// A change of another namespace is ignored, the namespace's own ones
	// arrive in order, each with one onChange.
	emitSetting(t, b.srv, "org.gnome.desktop.interface", keyReducedMotion, uint32(1))
	emitSetting(t, b.srv, portalNamespace, keyColorScheme, uint32(0))
	emitSetting(t, b.srv, portalNamespace, keyAccentColor, struct{ R, G, B float64 }{1, 0, 0})
	emitSetting(t, b.srv, portalNamespace, keyReducedMotion, uint32(1))
	c.wait(t)
	c.wait(t)
	c.wait(t)
	checkValues(t, p, map[string]any{
		keyColorScheme:   uint32(0),
		keyAccentColor:   [3]float64{1, 0, 0},
		keyContrast:      uint32(0),
		keyReducedMotion: uint32(1),
	})
	if a := readAppearance(portalOnly{p}); a.Dark || !a.ReduceMotion || a.Accent.R != 1 || a.Accent.G != 0 {
		t.Errorf("readAppearance after the changes = %+v", a)
	}

	// No onChange after Close; Close twice is fine.
	p.Close()
	n := c.count()
	_ = b.srv.Emit(portalPath, portalSignal, portalNamespace, keyContrast, dbus.MakeVariant(uint32(1)))
	time.Sleep(50 * time.Millisecond)
	if c.count() != n {
		t.Errorf("onChange after Close")
	}
	p.Close()
	if n != 3 {
		t.Errorf("onChange calls = %d, want 3", n)
	}
}

func TestOpenPortalFallbacks(t *testing.T) {
	tests := []struct {
		name   string
		portal any
	}{
		{"ReadOne (ReadAll unknown)", &fakePortalOne{values: appearanceValues()}},
		{"Read (ReadAll and ReadOne unknown)", &fakePortalRead{values: appearanceValues()}},
		{"ReadOne after a ReadAll error", &fakePortalAll{values: appearanceValues(),
			err: dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := fakeBusConfig(t, tt.portal)
			p, err := openPortal(cfg, nil)
			if err != nil {
				t.Fatalf("openPortal: %v", err)
			}
			defer p.Close()
			checkValues(t, p, wantAppearanceValues)
		})
	}
}

// A portal that is absent, has no Settings object or fails: the portal
// stays open with every value unset, and the error says why.
func TestOpenPortalUnread(t *testing.T) {
	tests := []struct {
		name   string
		portal any
		errIs  string
	}{
		{"no Settings object", nil, ""},
		{"service unknown", &fakePortalAll{err: dbus.NewError(errServiceUnknown, nil)}, errServiceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, served := fakeBusConfig(t, tt.portal)
			c := newChanges()
			p, err := openPortal(cfg, c.onChange)
			if err == nil {
				t.Fatal("openPortal: no error")
			}
			if tt.errIs != "" && dbusErrorName(err) != tt.errIs {
				t.Errorf("error %v, want %s", err, tt.errIs)
			}
			defer p.Close()
			checkValues(t, p, nil)
			// Still subscribed.
			emitSetting(t, (<-served).srv, portalNamespace, keyColorScheme, uint32(1))
			c.wait(t)
			checkValues(t, p, map[string]any{keyColorScheme: uint32(1)})
		})
	}
}

// A portal that does not answer within the timeout: openPortal returns in
// time, every value unset, and later changes still arrive.
func TestOpenPortalHungPortal(t *testing.T) {
	cfg, served := fakeBusConfig(t, &fakePortalAll{values: appearanceValues(), delay: 3 * time.Second})
	cfg.timeout = 250 * time.Millisecond
	c := newChanges()
	start := time.Now()
	p, err := openPortal(cfg, c.onChange)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("openPortal: no error")
	}
	if elapsed > cfg.timeout+500*time.Millisecond {
		t.Errorf("openPortal took %v with a %v timeout", elapsed, cfg.timeout)
	}
	defer p.Close()
	checkValues(t, p, nil)
	emitSetting(t, (<-served).srv, portalNamespace, keyContrast, uint32(1))
	c.wait(t)
	checkValues(t, p, map[string]any{keyContrast: uint32(1)})
}

// A bus that accepts the connection and never answers: openPortal gives
// up after the timeout with no portal.
func TestOpenPortalHungBus(t *testing.T) {
	cfg := defaultPortalConfig()
	cfg.timeout = 250 * time.Millisecond
	cfg.auth = []dbus.Auth{dbus.AuthAnonymous()}
	servers := make(chan net.Conn, 1) // dial runs on connectBus's goroutine
	cfg.dial = func(opts ...dbus.ConnOption) (*dbus.Conn, error) {
		client, server := net.Pipe()
		servers <- server
		return dbus.NewConn(client, opts...)
	}
	start := time.Now()
	p, err := openPortal(cfg, nil)
	elapsed := time.Since(start)
	if p != nil || err == nil {
		t.Errorf("openPortal = %v, %v, want nil and an error", p, err)
	}
	if elapsed > cfg.timeout+500*time.Millisecond {
		t.Errorf("openPortal took %v with a %v timeout", elapsed, cfg.timeout)
	}
	// The abandoned connection was closed: the server end sees EOF.
	server := <-servers
	defer server.Close()
	_ = server.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	for {
		if _, err := server.Read(buf); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("server read: %v, want EOF", err)
			}
			break
		}
	}
}

// No session bus: openPortal fails at once with no portal.
func TestOpenPortalNoBus(t *testing.T) {
	cfg := defaultPortalConfig()
	cfg.dial = func(...dbus.ConnOption) (*dbus.Conn, error) {
		return nil, errors.New("dbus: couldn't determine address of session bus")
	}
	p, err := openPortal(cfg, nil)
	if p != nil || err == nil {
		t.Errorf("openPortal = %v, %v, want nil and an error", p, err)
	}
	p.Close()
}

// The accent's special values survive the bus as readAppearance needs them.
func TestOpenPortalAccentBounds(t *testing.T) {
	values := map[string]dbus.Variant{
		keyAccentColor: dbus.MakeVariant(struct{ R, G, B float64 }{math.NaN(), 0, 0}),
	}
	cfg, _ := fakeBusConfig(t, &fakePortalAll{values: values})
	p, err := openPortal(cfg, nil)
	if err != nil {
		t.Fatalf("openPortal: %v", err)
	}
	defer p.Close()
	v, ok := p.portalValue(keyAccentColor)
	rgb, isRGB := v.([3]float64)
	if !ok || !isRGB || !math.IsNaN(rgb[0]) {
		t.Errorf("accent = %#v, %v, want [NaN 0 0]", v, ok)
	}
	if a := readAppearance(portalOnly{p}); a.Accent != readAppearance(portalOnly{}).Accent {
		t.Errorf("NaN accent: %+v, want the default accent", a.Accent)
	}
}

// TestOpenSystemSettings: New's source reads the portal, reports its
// changes through changed and closes it; without GtkSettings props their
// half is unset (gtksettings_test.go has them).
func TestOpenSystemSettings(t *testing.T) {
	cfg, served := fakeBusConfig(t, &fakePortalAll{values: appearanceValues()})
	var logs []string
	logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	c := newChanges()
	src := openSystemSettings(cfg, nil, logf)(c.onChange)
	if a := readAppearance(src); !a.Dark || a.Accent != driver.RGBA(0.2, 0.4, 0.6, 1) {
		t.Errorf("readAppearance = %+v, want dark with the portal's accent", a)
	}
	if _, ok := src.gtkBool(gtkOverlayScrolling); ok {
		t.Error("gtkBool set without GtkSettings")
	}
	b := <-served
	emitSetting(t, b.srv, portalNamespace, keyColorScheme, uint32(2))
	c.wait(t)
	if readAppearance(src).Dark {
		t.Error("still dark after color-scheme 2")
	}
	src.close()
	src.close()
	if len(logs) != 0 {
		t.Errorf("logged %q with a portal", logs)
	}
}

// TestOpenSystemSettingsNoBus: without a session bus every value is unset
// (the default appearance), logged once; close is fine.
func TestOpenSystemSettingsNoBus(t *testing.T) {
	cfg := defaultPortalConfig()
	cfg.dial = func(...dbus.ConnOption) (*dbus.Conn, error) {
		return nil, errors.New("dbus: couldn't determine address of session bus")
	}
	var logs []string
	logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	src := openSystemSettings(cfg, nil, logf)(func() { t.Error("changed without a bus") })
	if a := readAppearance(src); a != driver.DefaultAppearance() {
		t.Errorf("readAppearance = %+v, want the default", a)
	}
	src.close()
	if len(logs) != 1 || !strings.Contains(logs[0], "session bus") {
		t.Errorf("logs = %q, want one about the session bus", logs)
	}
	// A nil logf logs nothing.
	openSystemSettings(cfg, nil, nil)(nil).close()
}
