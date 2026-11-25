package main

import (
	"os"
	"path/filepath"
)

// Where the add-on keeps itself, on userdata, which outlives a TECHO5 update. Variables, for tests.
var (
	// home is the add-on's own folder: NetBird's client, the peer's identity and its settings, and, when
	// installed by hand, its program and its log. It is handHome or, installed from TECHO5's add-on
	// catalog, the state folder TECHO5 gives it (setMode).
	home = "/data/techo5-netbird"
	// handHome is where an install by hand (tools/install.py, rc.local) keeps itself.
	handHome = "/data/techo5-netbird"
	// catalogDir is where TECHO5's add-on manager (v1.6.53, feature/addons) installs this add-on:
	// bin/ (the program and the files the catalog pins beside it, root's and read-only), addon.json,
	// and state/, which outlives updates and is kept on a removal unless the owner deletes it too.
	catalogDir = "/data/techo5-addons/techo5-netbird"
	// catalog is whether this add-on was installed from TECHO5's add-on catalog: TECHO5 starts it, keeps
	// it running and updates it, and the catalog pinned NetBird's client for the device to check.
	catalog bool
	// catalogBin is the catalog install's bin/ folder, where NetBird's archive is (clientFile).
	catalogBin string
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
	// releaseFile names the TECHO5 release the device runs ("techo5 rootfs v1.6.47 built …").
	releaseFile = "/etc/techo5-release"
)

// setMode finds which install this is: one TECHO5's add-on manager started (it says so in the
// environment), a command run over SSH on a device with a catalog install, or one by hand. A supervisor
// (run) goes by the environment alone: one rc.local starts is the hand install's, even beside a catalog
// install, which it then gives way to (conflict).
func setMode(supervisor bool) {
	if state := os.Getenv("TECHO5_ADDON_STATE"); state != "" {
		catalog, home = true, state
		catalogBin = os.Getenv("TECHO5_ADDON_BIN")
		if catalogBin == "" {
			catalogBin = filepath.Join(filepath.Dir(state), "bin")
		}
		return
	}
	if _, err := os.Stat(filepath.Join(catalogDir, "addon.json")); err == nil && !supervisor {
		catalog, home, catalogBin = true, filepath.Join(catalogDir, "state"), filepath.Join(catalogDir, "bin")
		return
	}
	catalog, home = false, handHome
}

// withSbin is a PATH with the system's sbin folders at its end, where they are missing: TECHO5's add-on
// manager starts add-ons with /usr/local/bin:/usr/bin:/bin, and the firewall check (iptables-legacy,
// in /usr/sbin) and NetBird's client (ip, in /sbin) need them.
func withSbin(path string) string {
	have := map[string]bool{}
	for _, d := range filepath.SplitList(path) {
		have[d] = true
	}
	for _, d := range []string{"/usr/local/sbin", "/usr/sbin", "/sbin"} {
		if !have[d] {
			if path != "" {
				path += string(filepath.ListSeparator)
			}
			path += d
		}
	}
	return path
}

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
