package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// The client keeps what it was joined with in its profile (state/default.json) and reads it when its
// daemon starts. `netbird up` changes nothing on a daemon that is connected already, so a device joined
// before, by TECHO5's built-in NetBird or by an older add-on, is brought to these settings by editing
// the profile before the daemon starts. Every one is what join says outright too (upArgs).
//
// The device takes nothing from the network: no DNS, no server routes, no IPv6, no SSH server, no
// Rosenpass, and no client routes unless networks are listed (routes.go). Its own firewall is on, so
// the owner's access policies decide which ports each peer reaches, under TECHO5's tailnet chain, which
// stays the ceiling. Its interface is netbird0, the name TECHO5's firewall knows it by.
func profileSettings(routesOn bool) map[string]string {
	return map[string]string{
		"WgIface":             `"` + iface + `"`,
		"DisableFirewall":     "false",
		"DisableDNS":          "true",
		"DisableServerRoutes": "true",
		"DisableClientRoutes": fmt.Sprint(!routesOn),
		"DisableIPv6":         "true",
		"ServerSSHAllowed":    "false",
		"RosenpassEnabled":    "false",
		"DisableAutoConnect":  "false",
	}
}

// enforceProfile writes profileSettings into the profile at path, and answers which it changed. Nothing
// to do without a profile (not joined yet: join says them all), or with each already so.
func enforceProfile(path string, routesOn bool) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c map[string]json.RawMessage
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var changed []string
	for k, v := range profileSettings(routesOn) {
		if strings.TrimSpace(string(c[k])) != v {
			c[k] = json.RawMessage(v)
			changed = append(changed, k)
		}
	}
	if len(changed) == 0 {
		return nil, nil
	}
	slices.Sort(changed)
	out, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return nil, err
	}
	return changed, writeWhole(path, 0o600, bytes.NewReader(out))
}
