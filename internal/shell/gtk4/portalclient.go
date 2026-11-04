package gtk4

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// The settings portal client (PLAN.md S4.3b): the org.freedesktop.appearance
// values of org.freedesktop.portal.Settings over the session bus, kept
// current by SettingChanged. It is pure Go (godbus) and builds everywhere;
// only the shell on Linux opens it (S4.3c).

// The portal's well-known name, object and interface, and the namespace
// whose keys readAppearance reads.
const (
	portalDest      = "org.freedesktop.portal.Desktop"
	portalPath      = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalIface     = "org.freedesktop.portal.Settings"
	portalNamespace = "org.freedesktop.appearance"
	portalSignal    = portalIface + ".SettingChanged"

	// portalTimeout bounds the whole start-up read (connect, subscribe,
	// read), so a hung bus or portal cannot block start-up.
	portalTimeout = 250 * time.Millisecond
)

// The D-Bus errors the client tells apart.
const (
	errNotFound       = "org.freedesktop.portal.Error.NotFound"
	errUnknownMethod  = "org.freedesktop.DBus.Error.UnknownMethod"
	errServiceUnknown = "org.freedesktop.DBus.Error.ServiceUnknown"
	errNameHasNoOwner = "org.freedesktop.DBus.Error.NameHasNoOwner"
	errNoReply        = "org.freedesktop.DBus.Error.NoReply"
	errTimeout        = "org.freedesktop.DBus.Error.Timeout"
	errDisconnected   = "org.freedesktop.DBus.Error.Disconnected"
)

// portalKeys are the keys read one by one when ReadAll fails.
var portalKeys = [...]string{keyColorScheme, keyAccentColor, keyContrast, keyReducedMotion}

// portalConfig is what openPortal connects to; the tests replace parts of
// defaultPortalConfig.
type portalConfig struct {
	dest    string        // the portal's bus name
	timeout time.Duration // bounds openPortal
	// dial opens an unauthenticated connection to the session bus.
	dial func(opts ...dbus.ConnOption) (*dbus.Conn, error)
	auth []dbus.Auth // nil: godbus's defaults (EXTERNAL on Linux)
}

func defaultPortalConfig() portalConfig {
	// NoAutoStartup: never launch a session bus (dbus-launch) from an app.
	return portalConfig{dest: portalDest, timeout: portalTimeout, dial: dbus.SessionBusPrivateNoAutoStartup}
}

// portal holds the org.freedesktop.appearance values of the settings
// portal. Its methods are safe for concurrent use and accept a nil
// receiver (no portal: every value unset).
type portal struct {
	conn     *dbus.Conn
	onChange func()
	done     chan struct{} // closed when watch has returned
	close    sync.Once

	mu     sync.Mutex
	values map[string]any // decoded (decodeValue); a missing key is unset
}

// openPortal connects to the session bus, subscribes to SettingChanged for
// org.freedesktop.appearance and reads the namespace: ReadAll, else ReadOne
// per key, else the deprecated Read; NotFound is unset. All of it within
// cfg.timeout.
//
// Without a session bus (or when connecting or subscribing fails) it
// returns nil and the error. When only the read fails (no portal, a hung
// one, an unknown error) it returns the portal with what it could read and
// the error: it stays subscribed, so a portal that starts later still
// reports its changes. Errors are for logging only; a nil portal is a
// valid, empty source.
//
// onChange is called on godbus's goroutine (never on the UI thread) after
// a SettingChanged of the namespace has updated the values, in signal
// order; it must not block or call Close. It is never called before
// openPortal returns or after Close.
func openPortal(cfg portalConfig, onChange func()) (*portal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	conn, err := connectBus(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("portal: session bus: %w", err)
	}
	p := &portal{conn: conn, onChange: onChange, done: make(chan struct{}), values: map[string]any{}}
	// Subscribe before reading, so no change between the read and the
	// subscription is lost. The sequential handler keeps the signals in
	// order and queues them until watch starts after the read: a change
	// sent before the read's reply is already in it, a later one wins.
	sigs := make(chan *dbus.Signal, 8)
	conn.Signal(sigs)
	if err := conn.AddMatchSignalContext(ctx,
		dbus.WithMatchSender(cfg.dest),
		dbus.WithMatchObjectPath(portalPath),
		dbus.WithMatchInterface(portalIface),
		dbus.WithMatchMember("SettingChanged"),
		dbus.WithMatchArg(0, portalNamespace),
	); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("portal: subscribe: %w", err)
	}
	values, rerr := readPortal(ctx, conn.Object(cfg.dest, portalPath))
	p.values = values
	go p.watch(sigs)
	if rerr != nil {
		return p, fmt.Errorf("portal: read: %w", rerr)
	}
	return p, nil
}

// connectBus dials, authenticates and says Hello within ctx. godbus takes
// no context for these, so they run on a goroutine and a timeout closes
// the connection under them.
func connectBus(ctx context.Context, cfg portalConfig) (*dbus.Conn, error) {
	var (
		mu        sync.Mutex
		conn      *dbus.Conn
		abandoned bool
	)
	res := make(chan error, 1)
	go func() {
		c, err := cfg.dial(dbus.WithSignalHandler(dbus.NewSequentialSignalHandler()))
		if err != nil {
			res <- err
			return
		}
		mu.Lock()
		if abandoned {
			mu.Unlock()
			_ = c.Close()
			return
		}
		conn = c
		mu.Unlock()
		if err = c.Auth(cfg.auth); err == nil {
			err = c.Hello()
		}
		res <- err
	}()
	select {
	case err := <-res:
		if err != nil && conn != nil {
			_ = conn.Close()
		}
		if err != nil {
			return nil, err
		}
		return conn, nil
	case <-ctx.Done():
		mu.Lock()
		abandoned = true
		if conn != nil {
			_ = conn.Close() // unblocks Auth or Hello
		}
		mu.Unlock()
		return nil, ctx.Err()
	}
}

// readPortal reads org.freedesktop.appearance: ReadAll, and when the
// portal answers that with an error other than its absence, ReadOne per
// key, falling back to the deprecated Read on UnknownMethod. The values
// are decoded; a NotFound key is left out. On an error it returns what it
// has read so far.
func readPortal(ctx context.Context, obj dbus.BusObject) (map[string]any, error) {
	var all map[string]map[string]dbus.Variant
	err := obj.CallWithContext(ctx, portalIface+".ReadAll", 0, []string{portalNamespace}).Store(&all)
	if err == nil {
		return decodeReadAll(all), nil
	}
	if !portalAnswered(err) {
		return map[string]any{}, err
	}
	values := map[string]any{}
	method := portalIface + ".ReadOne"
	for _, key := range portalKeys {
		var v dbus.Variant
		err := obj.CallWithContext(ctx, method, 0, portalNamespace, key).Store(&v)
		if dbusErrorName(err) == errUnknownMethod && method != portalIface+".Read" {
			method = portalIface + ".Read" // portal version 1: doubly wrapped
			err = obj.CallWithContext(ctx, method, 0, portalNamespace, key).Store(&v)
		}
		switch {
		case err == nil:
			values[key] = decodeValue(v)
		case dbusErrorName(err) == errNotFound:
			// unset
		default:
			return values, err
		}
	}
	return values, nil
}

// portalAnswered reports whether err is an error reply of the portal
// itself (UnknownMethod, AccessDenied, ...), as opposed to its absence, a
// timeout or a broken connection, after which per-key reads cannot help.
func portalAnswered(err error) bool {
	switch dbusErrorName(err) {
	case "", errServiceUnknown, errNameHasNoOwner, errNoReply, errTimeout, errDisconnected:
		return false
	}
	return true
}

// dbusErrorName returns the name of a D-Bus error reply, or "" for nil and
// for any other error.
func dbusErrorName(err error) string {
	var de dbus.Error
	if errors.As(err, &de) {
		return de.Name
	}
	var dp *dbus.Error
	if errors.As(err, &dp) && dp != nil {
		return dp.Name
	}
	return ""
}

// decodeReadAll decodes the org.freedesktop.appearance part of a ReadAll
// reply (a{sa{sv}}).
func decodeReadAll(all map[string]map[string]dbus.Variant) map[string]any {
	values := map[string]any{}
	for key, v := range all[portalNamespace] {
		values[key] = decodeValue(v)
	}
	return values
}

// decodeValue turns a portal value into the plain Go value readAppearance
// expects: variants are unwrapped (the deprecated Read wraps twice), a
// (ddd) struct becomes [3]float64. Anything else is returned unwrapped as
// it is (uint32 stays uint32); readAppearance treats unexpected types as
// unset.
func decodeValue(v any) any {
	for {
		vv, ok := v.(dbus.Variant)
		if !ok {
			break
		}
		v = vv.Value()
	}
	if s, ok := v.([]any); ok && len(s) == 3 {
		var rgb [3]float64
		for i, c := range s {
			f, ok := c.(float64)
			if !ok {
				return v
			}
			rgb[i] = f
		}
		return rgb
	}
	return v
}

// watch applies the SettingChanged signals until the connection closes.
func (p *portal) watch(sigs <-chan *dbus.Signal) {
	defer close(p.done)
	for sig := range sigs {
		if p.apply(sig) && p.onChange != nil {
			p.onChange()
		}
	}
}

// apply stores the value of a SettingChanged of org.freedesktop.appearance
// on the portal's object and reports whether sig was one. Other signals
// (the bus's own, other namespaces, malformed ones) are ignored.
func (p *portal) apply(sig *dbus.Signal) bool {
	if sig == nil || sig.Name != portalSignal || sig.Path != portalPath || len(sig.Body) != 3 {
		return false
	}
	ns, ok1 := sig.Body[0].(string)
	key, ok2 := sig.Body[1].(string)
	v, ok3 := sig.Body[2].(dbus.Variant)
	if !ok1 || !ok2 || !ok3 || ns != portalNamespace {
		return false
	}
	p.mu.Lock()
	p.values[key] = decodeValue(v)
	p.mu.Unlock()
	return true
}

// portalValue returns the decoded value of key in org.freedesktop.appearance
// (the settingsSource half); ok is false when it is unset or p is nil.
func (p *portal) portalValue(key string) (any, bool) {
	if p == nil {
		return nil, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.values[key]
	return v, ok
}

// Close closes the connection and waits until watch has returned, so
// onChange is not called after it. It may be called more than once and on
// a nil portal; it must not be called from onChange.
func (p *portal) Close() {
	if p == nil {
		return
	}
	p.close.Do(func() {
		_ = p.conn.Close()
		<-p.done
	})
}
