package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// catalogSandbox is sandbox with a catalog install's layout: bin/ and state/ under catalogDir, and the
// add-on in catalog mode with its home in state/. The install by hand stays at handHome.
func catalogSandbox(t *testing.T) {
	t.Helper()
	sandbox(t)
	catalog, home, catalogBin = true, filepath.Join(catalogDir, "state"), filepath.Join(catalogDir, "bin")
	for _, p := range []string{home, catalogBin} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "addon.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetMode(t *testing.T) {
	sandbox(t)
	hand := handHome

	t.Setenv("TECHO5_ADDON_STATE", "")
	setMode(false)
	if catalog || home != hand {
		t.Errorf("no catalog install: catalog %v, home %s", catalog, home)
	}

	os.MkdirAll(catalogDir, 0o755)
	os.WriteFile(filepath.Join(catalogDir, "addon.json"), []byte("{}"), 0o644)
	setMode(false)
	if !catalog || home != filepath.Join(catalogDir, "state") || catalogBin != filepath.Join(catalogDir, "bin") {
		t.Errorf("a command beside a catalog install: catalog %v, home %s, bin %s", catalog, home, catalogBin)
	}
	// rc.local's supervisor is the hand install's, even beside a catalog install.
	setMode(true)
	if catalog || home != hand {
		t.Errorf("rc.local's supervisor: catalog %v, home %s", catalog, home)
	}

	t.Setenv("TECHO5_ADDON_STATE", "/x/state")
	t.Setenv("TECHO5_ADDON_BIN", "/x/bin")
	setMode(true)
	if !catalog || home != "/x/state" || catalogBin != "/x/bin" {
		t.Errorf("started by TECHO5: catalog %v, home %s, bin %s", catalog, home, catalogBin)
	}
	t.Setenv("TECHO5_ADDON_BIN", "")
	setMode(true)
	if catalogBin != "/x/bin" {
		t.Errorf("no TECHO5_ADDON_BIN: bin %s", catalogBin)
	}
}

func TestFetchClientCatalog(t *testing.T) {
	catalogSandbox(t)
	good := archive(t, map[string]string{"netbird": "#!/bin/sh\necho client\n", "LICENSE": "BSD"})
	was := clientArchive
	t.Cleanup(func() { clientArchive = was })
	sum := sha256.Sum256(good)
	// Nothing is downloaded in catalog mode: a download from here would fail.
	clientArchive = pinned{URL: "http://127.0.0.1:1/netbird.tar.gz", Size: int64(len(good)), SHA256: hex.EncodeToString(sum[:])}
	src := filepath.Join(catalogBin, clientFile)
	put := func(b []byte) { // read-only, as TECHO5 installs it
		os.Remove(src)
		if err := os.WriteFile(src, b, 0o444); err != nil {
			t.Fatal(err)
		}
	}

	if err := fetchClient(context.Background()); err == nil || !strings.Contains(err.Error(), "not where") {
		t.Fatalf("no archive in bin/: %v", err)
	}
	for _, bad := range [][]byte{append(append([]byte{}, good[:len(good)-1]...), good[len(good)-1]^1), good[:len(good)-10]} {
		put(bad)
		if err := fetchClient(context.Background()); err == nil {
			t.Fatal("an archive that does not match the pin was taken")
		}
		if left, _ := os.ReadDir(clientDir()); len(left) != 0 {
			t.Fatalf("left behind: %v", left)
		}
	}
	put(good)
	if err := fetchClient(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(clientBin()); !strings.Contains(string(b), "echo client") {
		t.Errorf("client not unpacked: %q", b)
	}
	if !strings.HasPrefix(clientBin(), home+"/") {
		t.Errorf("client at %s, outside the state folder %s", clientBin(), home)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("the catalog's file is gone: %v", err)
	}
}

func TestTakeOverHand(t *testing.T) {
	catalogSandbox(t)
	os.MkdirAll(filepath.Join(handHome, "state"), 0o700)
	os.MkdirAll(filepath.Join(handHome, "client"), 0o755)
	os.WriteFile(filepath.Join(handHome, "state", "default.json"), []byte(`{"WgIface":"netbird0"}`), 0o600)
	os.WriteFile(filepath.Join(handHome, "config.json"), []byte(`{"management":"https://nb.example.com","joined":true,"routes":["192.168.1.0/24"]}`), 0o600)
	os.WriteFile(rcLocal, []byte("#!/bin/sh\necho mine\n"+rcBlock()+"echo after\n"), 0o755)
	// The daemon of the catalog install, not joined yet, made a profile of its own: that goes.
	os.MkdirAll(stateDir(), 0o700)
	os.WriteFile(profilePath(), []byte(`{"fresh":true}`), 0o600)

	if err := stopHand(); err != nil {
		t.Fatal(err)
	}
	takeOver()

	if b, _ := os.ReadFile(rcLocal); string(b) != "#!/bin/sh\necho mine\necho after\n" {
		t.Errorf("rc.local: %q", b)
	}
	if b, _ := os.ReadFile(profilePath()); string(b) != `{"WgIface":"netbird0"}` {
		t.Errorf("peer: %q", b)
	}
	cfg, err := loadConfig()
	if err != nil || !cfg.Joined || cfg.Management != "https://nb.example.com" || !slices.Equal(cfg.Routes, []string{"192.168.1.0/24"}) {
		t.Errorf("settings: %+v %v", cfg, err)
	}
	if _, err := os.Stat(handHome); !os.IsNotExist(err) {
		t.Errorf("the install by hand is still there: %v", err)
	}

	// Joined now: a built-in peer that turns up later is left alone.
	os.MkdirAll(builtinState(), 0o700)
	os.WriteFile(filepath.Join(builtinState(), "default.json"), []byte(`{"other":true}`), 0o600)
	takeOver()
	if _, err := os.Stat(filepath.Join(builtinState(), "default.json")); err != nil {
		t.Errorf("a joined install took the built-in's peer: %v", err)
	}
}

func TestTakeOverBuiltin(t *testing.T) {
	catalogSandbox(t)
	os.MkdirAll(builtinState(), 0o700)
	os.WriteFile(filepath.Join(builtinState(), "default.json"), []byte(`{"WgIface":"netbird0"}`), 0o600)
	st := filepath.Join(techo5Dir, "state.json")

	// The built-in on (TECHO5 up to v1.6.46): nothing is taken.
	os.WriteFile(st, []byte(`{"netbird":{"on":true,"management":"https://nb.example.com"}}`), 0o600)
	takeOver()
	if cfg, _ := loadConfig(); cfg.Joined {
		t.Fatal("took the peer of a built-in NetBird that is on")
	}

	os.WriteFile(st, []byte(`{"netbird":{"joined":true,"management":"https://nb.example.com"}}`), 0o600)
	takeOver()
	cfg, _ := loadConfig()
	if !cfg.Joined || cfg.Management != "https://nb.example.com" {
		t.Errorf("settings: %+v", cfg)
	}
	if b, _ := os.ReadFile(profilePath()); string(b) != `{"WgIface":"netbird0"}` {
		t.Errorf("peer: %q", b)
	}
	if _, err := os.Stat(builtinState()); !os.IsNotExist(err) {
		t.Errorf("the built-in's state is still there: %v", err)
	}
}

func TestTakeOverNothing(t *testing.T) {
	catalogSandbox(t)
	// An install by hand that never joined is not taken; nor is anything when there is nothing.
	os.MkdirAll(handHome, 0o755)
	os.WriteFile(filepath.Join(handHome, "config.json"), []byte(`{}`), 0o600)
	takeOver()
	if cfg, _ := loadConfig(); cfg.Joined {
		t.Errorf("joined from nothing: %+v", cfg)
	}
	if _, err := os.Stat(handHome); err != nil {
		t.Errorf("an install by hand with no peer was deleted: %v", err)
	}
}

func TestConflictCatalog(t *testing.T) {
	sandbox(t)
	if err := conflict(); err != nil {
		t.Fatalf("no catalog install: %v", err)
	}
	os.MkdirAll(catalogDir, 0o755)
	os.WriteFile(filepath.Join(catalogDir, "addon.json"), []byte("{}"), 0o644)
	if err := conflict(); err == nil || !strings.Contains(err.Error(), "Add-ons card") {
		t.Errorf("hand install beside a catalog install: %v", err)
	}
	catalog = true
	if err := conflict(); err != nil {
		t.Errorf("the catalog install itself: %v", err)
	}
}

func TestCatalogRefusesInstall(t *testing.T) {
	catalogSandbox(t)
	if err := install(); err != errCatalog {
		t.Errorf("install: %v", err)
	}
	if err := uninstall(nil); err != errCatalog {
		t.Errorf("uninstall: %v", err)
	}
	if _, err := os.Stat(rcLocal); !os.IsNotExist(err) {
		t.Errorf("rc.local was written: %v", err)
	}
}

// addon.json is what TECHO5's add-on catalog lists: it has to say what the code does.
func TestAddonJSON(t *testing.T) {
	b, err := os.ReadFile("addon.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID       string            `json:"id"`
		Version  string            `json:"version"`
		Command  []string          `json:"command"`
		Args     []string          `json:"args"`
		Files    []string          `json:"files"`
		Ports    []json.RawMessage `json:"ports"`
		Root     bool              `json:"root"`
		MemoryMB int               `json:"memory_mb"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "techo5-netbird" || !slices.Equal(m.Command, []string{"techo5-netbird"}) || !slices.Equal(m.Args, []string{"run"}) {
		t.Errorf("id/command/args: %+v", m)
	}
	if !slices.Equal(m.Files, []string{clientFile}) {
		t.Errorf("files %v, want [%s]", m.Files, clientFile)
	}
	if !m.Root || len(m.Ports) != 0 || m.MemoryMB < 110 {
		t.Errorf("root %v, ports %d, memory %d MB", m.Root, len(m.Ports), m.MemoryMB)
	}
	if m.Version == "" || strings.HasPrefix(m.Version, "v") {
		t.Errorf("version %q: X.Y.Z, without a v", m.Version)
	}
}
