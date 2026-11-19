package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// config is the add-on's own settings, in config.json. The peer's identity is NetBird's own, in the
// state folder; this is what the add-on adds to it.
type config struct {
	// Management is the management server the device was joined to: NetBird's own when empty.
	Management string `json:"management,omitempty"`
	// Joined is whether the add-on's peer was ever joined, by join or migrate. Until then, what the daemon
	// keeps in the state folder is a profile it made for itself, with no peer behind it.
	Joined bool `json:"joined,omitempty"`
	// Routes are the networks the device may take routes from the mesh to (routes.go): none at first,
	// and then it takes no routes at all.
	Routes []string `json:"routes,omitempty"`
}

// defaultManagement is NetBird's own management server.
const defaultManagement = "https://api.netbird.io:443"

func loadConfig() (config, error) {
	var c config
	b, err := os.ReadFile(configPath())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", configPath(), err)
	}
	return c, nil
}

func saveConfig(c config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	return writeWhole(configPath(), 0o600, bytes.NewReader(append(b, '\n')))
}

// managementURL is a management server's URL as the client takes it: NetBird's own for none, and only
// http or https to a host, with no user in it.
func managementURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultManagement, nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return "", errors.New("the management server is a URL, such as https://netbird.example.com")
	}
	return u.String(), nil
}

// shownURL is a management server's address without its path or query, which could carry a token.
func shownURL(s string) string {
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return "(not a URL)"
}
