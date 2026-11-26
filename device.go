package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// What the add-on asks of the device before NetBird runs, and why not when it may not. Each is a
// variable, for tests.

// fenced says whether TECHO5's firewall puts its tailnet chain in front of NetBird's interface, for IPv4
// and IPv6: TECHO5-TS lets in 6053, 8181, 22 and ping and drops the rest. NetBird is not started
// without it: a mesh open to every port is open to whoever the mesh lets reach the device. The same
// check TECHO5 made for its built-in NetBird (tailscale.Fenced).
var fenced = func() error {
	for _, tool := range []string{"iptables-legacy", "ip6tables-legacy"} {
		for _, rule := range [][]string{{"INPUT", "-i", iface, "-j", "TECHO5-TS"}, {"TECHO5-TS", "-j", "DROP"}} {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := exec.CommandContext(ctx, tool, append([]string{"-C"}, rule...)...).Run()
			cancel()
			if err != nil {
				return fmt.Errorf("TECHO5's firewall rules for %s are not in place (%s -C %s): this TECHO5 is too old or its firewall failed; see techo5-firewall in its boot log",
					iface, tool, strings.Join(rule, " "))
			}
		}
	}
	return nil
}

// keyed says whether Home Assistant's link to the device has an encryption key of its own, rather than
// the all-zeros one an unprovisioned device answers with. Over NetBird the link would otherwise be open
// in the clear; TECHO5's built-in NetBird asked the same. The key is ESPHome's: 32 bytes, base64.
var keyed = func() bool {
	b, err := os.ReadFile(filepath.Join(techo5Dir, "psk"))
	if err != nil {
		return false
	}
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != 32 {
		return false
	}
	for _, c := range k {
		if c != 0 {
			return true
		}
	}
	return false
}

// techo5Settings is the part of TECHO5's own settings read here.
type techo5Settings struct {
	Tailscale struct {
		On bool `json:"on"`
	} `json:"tailscale"`
	Netbird struct {
		On         bool   `json:"on"`
		Management string `json:"management"`
		Joined     bool   `json:"joined"`
	} `json:"netbird"`
}

// readTecho5 reads TECHO5's settings. A variable, for tests.
var readTecho5 = func() (techo5Settings, error) {
	var s techo5Settings
	b, err := os.ReadFile(filepath.Join(techo5Dir, "state.json"))
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// conflict says what else would claim the mesh's addresses or NetBird's interface: TECHO5's own NetBird,
// or Tailscale (both number their devices in 100.64.0.0/10).
var conflict = func() error {
	s, err := readTecho5()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading TECHO5's settings: %w", err)
	}
	switch {
	case s.Netbird.On:
		return errors.New("TECHO5's own NetBird is on: turn it off (its NetBird switch), or run techo5-netbird migrate")
	case s.Tailscale.On:
		return errors.New("Tailscale is on: Tailscale and NetBird can claim the same addresses; turn Tailscale off first")
	}
	if _, err := net.InterfaceByName("tailscale0"); err == nil {
		return errors.New("Tailscale's interface is up: Tailscale and NetBird can claim the same addresses")
	}
	// Installed by hand, it gives way to an install from TECHO5's add-on catalog, which takes over its
	// peer on its first start (takeover.go): two would fight over the interface.
	if !catalog {
		if _, err := os.Stat(filepath.Join(catalogDir, "addon.json")); err == nil {
			return errors.New("this add-on is installed from TECHO5's Add-ons card now, which runs it instead")
		}
	}
	return nil
}

// noSwitchSince is the TECHO5 release that took the built-in NetBird out, and its switch with it (#580).
var noSwitchSince = [3]int{1, 6, 47}

// hasNetbirdSwitch says whether this TECHO5 still has a NetBird switch of its own: up to v1.6.46, and
// when the release cannot be read, which is the safe answer for advice to leave it off.
func hasNetbirdSwitch() bool {
	b, err := os.ReadFile(releaseFile)
	if err != nil {
		return true
	}
	v, ok := releaseVersion(string(b))
	if !ok {
		return true
	}
	for i := range v {
		if v[i] != noSwitchSince[i] {
			return v[i] < noSwitchSince[i]
		}
	}
	return false
}

// releaseVersion is the version in TECHO5's release line, "techo5 rootfs v1.6.47 built …".
func releaseVersion(line string) ([3]int, bool) {
	var v [3]int
	for _, f := range strings.Fields(line) {
		if !strings.HasPrefix(f, "v") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(f, "v"), ".")
		if len(parts) != 3 {
			continue
		}
		ok := true
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 {
				ok = false
				break
			}
			v[i] = n
		}
		if ok {
			return v, true
		}
	}
	return v, false
}

// kernel64 says whether the kernel runs 64-bit ARM programs, which NetBird's client (linux_arm64) is.
// The add-on itself also comes as a 32-bit build, the one TECHO5's add-on catalog gives every device,
// since TECHO5's own programs are 32-bit. The kernel under them is not always 64-bit: an Echo Show 8
// (2nd gen) on Amazon's kernel runs a 32-bit one, and this check says so there.
var kernel64 = func() error {
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return fmt.Errorf("reading the kernel's architecture: %w", err)
	}
	if m := strings.TrimSpace(string(out)); m != "aarch64" && m != "arm64" {
		return fmt.Errorf("NetBird's client needs a 64-bit kernel, and this one is %s", m)
	}
	return nil
}

// preflight is every check that must pass before NetBird runs.
func preflight() error {
	if err := kernel64(); err != nil {
		return err
	}
	if err := conflict(); err != nil {
		return err
	}
	if !keyed() {
		return errors.New("Home Assistant's link to the device has no encryption key: set one first; over NetBird it would be open in the clear")
	}
	return fenced()
}

// deviceName is the device's name as a NetBird host name, as TECHO5 makes it: lower case, letters and
// digits, hyphens between.
func deviceName() string {
	b, _ := os.ReadFile(filepath.Join(techo5Dir, "name"))
	return hostname(string(b))
}

func hostname(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == '\'':
			// "Kid's Room" is kids-room, not kid-s-room
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	h := strings.TrimSuffix(b.String(), "-")
	if h == "" {
		return "techo5"
	}
	return h
}
