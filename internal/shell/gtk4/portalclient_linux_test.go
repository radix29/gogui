//go:build linux

package gtk4

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// The portal client against a real session bus (S4.3b). The test exports
// its own fake portal under a name of its own, so it neither needs nor
// disturbs a real xdg-desktop-portal. native.sh runs it under
// dbus-run-session (S4.3d); without a session bus it is skipped.

// sessionPortal exports portal on a new session-bus connection under a
// unique name and returns the connection and a portalConfig for it.
func sessionPortal(t *testing.T, portal any) (*dbus.Conn, portalConfig) {
	t.Helper()
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus (DBUS_SESSION_BUS_ADDRESS unset; run under dbus-run-session)")
	}
	srv, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skipf("no session bus: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	name := fmt.Sprintf("org.gogui.test.Portal.p%d_%d", os.Getpid(), time.Now().UnixNano())
	if err := srv.Export(portal, portalPath, portalIface); err != nil {
		t.Fatalf("Export: %v", err)
	}
	reply, err := srv.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName(%s) = %v, %v", name, reply, err)
	}
	cfg := defaultPortalConfig()
	cfg.dest = name
	cfg.timeout = 2 * time.Second // a loaded test machine; the real one is 250 ms
	return srv, cfg
}

func TestPortalSessionBus(t *testing.T) {
	srv, cfg := sessionPortal(t, &fakePortalAll{values: appearanceValues()})
	c := newChanges()
	p, err := openPortal(cfg, c.onChange)
	if err != nil {
		t.Fatalf("openPortal: %v", err)
	}
	defer p.Close()
	checkValues(t, p, wantAppearanceValues)

	// A change from another connection (not the portal's owner) is
	// filtered by the match rule's sender; one of another namespace by
	// arg0; the portal's own arrives.
	other, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer other.Close()
	emitSetting(t, other, portalNamespace, keyContrast, uint32(1))
	emitSetting(t, srv, "org.gnome.desktop.interface", keyContrast, uint32(1))
	emitSetting(t, srv, portalNamespace, keyColorScheme, uint32(2))
	c.wait(t)
	time.Sleep(100 * time.Millisecond) // let a wrongly delivered one arrive
	if n := c.count(); n != 1 {
		t.Errorf("onChange calls = %d, want 1", n)
	}
	checkValues(t, p, map[string]any{
		keyColorScheme: uint32(2),
		keyAccentColor: [3]float64{0.2, 0.4, 0.6},
		keyContrast:    uint32(0),
	})
}

func TestPortalSessionBusFallbacks(t *testing.T) {
	for name, portal := range map[string]any{
		"ReadOne": &fakePortalOne{values: appearanceValues()},
		"Read":    &fakePortalRead{values: appearanceValues()},
	} {
		t.Run(name, func(t *testing.T) {
			_, cfg := sessionPortal(t, portal)
			p, err := openPortal(cfg, nil)
			if err != nil {
				t.Fatalf("openPortal: %v", err)
			}
			defer p.Close()
			checkValues(t, p, wantAppearanceValues)
		})
	}
}

// No owner of the portal's name: "unset" within the real timeout.
func TestPortalSessionBusAbsent(t *testing.T) {
	_, cfg := sessionPortal(t, &fakePortalAll{})
	cfg.dest = cfg.dest + ".absent"
	cfg.timeout = portalTimeout
	start := time.Now()
	p, err := openPortal(cfg, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("openPortal: no error for an absent portal")
	} else {
		t.Logf("absent portal: %v (%v)", err, elapsed)
	}
	if elapsed > portalTimeout+100*time.Millisecond {
		t.Errorf("openPortal took %v, want at most %v", elapsed, portalTimeout)
	}
	defer p.Close()
	checkValues(t, p, nil)
}

// A portal that hangs: openPortal returns at the real timeout, unset.
func TestPortalSessionBusHung(t *testing.T) {
	_, cfg := sessionPortal(t, &fakePortalAll{values: appearanceValues(), delay: 2 * time.Second})
	cfg.timeout = portalTimeout
	start := time.Now()
	p, err := openPortal(cfg, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("openPortal: no error for a hung portal")
	}
	if elapsed > portalTimeout+100*time.Millisecond {
		t.Errorf("openPortal took %v, want at most %v", elapsed, portalTimeout)
	}
	defer p.Close()
	checkValues(t, p, nil)
}
