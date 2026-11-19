package main

import (
	"path/filepath"
)

// Where the add-on keeps itself, on userdata, which outlives a TECHO5 update. Variables, for tests.
var (
	// home is the add-on's own folder: its program, NetBird's client, the peer's identity, its settings
	// and its log.
	home = "/data/techo5-netbird"
	// techo5Dir is TECHO5's state directory on the Echo Shows and the Echo Spot: its settings
	// (state.json), the Home Assistant link's key (psk) and the device's name, read here and never
	// written, and the built-in NetBird's folder, which migrate takes the identity from.
	techo5Dir = "/data/misc/techo5"
	// rcLocal is the script TECHO5 starts at the end of every boot (docs/setup.md, "Running something
	// of your own at boot").
	rcLocal = "/data/techo5-linux/rc.local"
	// runDir holds what lasts only while the device is up: the supervisor's lock, pid and status, and
	// the daemon's socket.
	runDir = "/run/techo5-netbird"
)

func binDir() string      { return filepath.Join(home, "bin") }
func selfPath() string    { return filepath.Join(binDir(), "techo5-netbird") }
func clientDir() string   { return filepath.Join(home, "client") }
func stateDir() string    { return filepath.Join(home, "state") }
func profilePath() string { return filepath.Join(stateDir(), "default.json") }
func configPath() string  { return filepath.Join(home, "config.json") }
func logPath() string     { return filepath.Join(home, "techo5-netbird.log") }
func lockPath() string    { return filepath.Join(runDir, "lock") }
func pidPath() string     { return filepath.Join(runDir, "pid") }
func statusPath() string  { return filepath.Join(runDir, "status.json") }
func sockPath() string    { return filepath.Join(runDir, "daemon.sock") }

// The built-in NetBird's places in TECHO5's state directory (echod/internal/feature/netbird).
func builtinDir() string   { return filepath.Join(techo5Dir, "netbird") }
func builtinState() string { return filepath.Join(builtinDir(), "state") }

// builtinSock is the built-in's daemon socket: the add-on uses another, so that echod, which stops any
// daemon it finds on its own socket, never stops this one.
const builtinSock = "/run/techo5-netbird.sock"

// iface is NetBird's interface. TECHO5's firewall sends it to its tailnet chain by this name
// (techo5-firewall: 6053, 8181, 22 and ping), and TECHO5 treats a connection over it as one from away.
// It must not change.
const iface = "netbird0"
