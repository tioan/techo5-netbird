package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// sandbox points every place the add-on uses into a temporary folder.
func sandbox(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	wasHome, wasT5, wasRC, wasRun := home, techo5Dir, rcLocal, runDir
	home, techo5Dir, rcLocal, runDir = filepath.Join(d, "addon"), filepath.Join(d, "techo5"), filepath.Join(d, "rc.local"), filepath.Join(d, "run")
	logger.to = &bytes.Buffer{}
	t.Cleanup(func() {
		home, techo5Dir, rcLocal, runDir = wasHome, wasT5, wasRC, wasRun
		logger.to = nil
	})
	for _, p := range []string{home, techo5Dir, runDir} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestRoutesInUse(t *testing.T) {
	st, err := parseStatus([]byte(`{"daemonStatus":"Connected","netbirdIp":"100.64.0.20/16","fqdn":"kitchen.netbird.cloud","networks":null,
		"peers":{"total":3,"details":[
			{"fqdn":"router.netbird.cloud","networks":["192.168.1.0/24","10.20.0.0/16"]},
			{"fqdn":"laptop.netbird.cloud","networks":null},
			{"fqdn":"backup.netbird.cloud","networks":["192.168.1.0/24"]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if st.IP != "100.64.0.20" || st.name() != "kitchen" {
		t.Errorf("ip %q name %q", st.IP, st.name())
	}
	if got := st.routesInUse(); !slices.Equal(got, []string{"10.20.0.0/16", "192.168.1.0/24"}) {
		t.Errorf("in use: %q", got)
	}
	st, _ = parseStatus([]byte(`{"daemonStatus":"Connected","networks":["192.168.1.0/24"],"peers":{"details":[{"networks":null}]}}`))
	if got := st.routesInUse(); len(got) != 0 {
		t.Errorf("the device's own routes taken for routes in use: %q", got)
	}
}

func TestUpArgs(t *testing.T) {
	args := strings.Join(upArgs("https://nb.example.com", "kitchen", false), " ")
	for _, want := range []string{"--interface-name=netbird0", "--disable-dns", "--disable-client-routes=true",
		"--disable-server-routes", "--disable-firewall=false", "--disable-ipv6", "--allow-server-ssh=false",
		"--enable-rosenpass=false", "--hostname=kitchen", "--management-url=https://nb.example.com"} {
		if !strings.Contains(args, want) {
			t.Errorf("%q missing in %s", want, args)
		}
	}
	if !strings.Contains(strings.Join(upArgs("x", "y", true), " "), "--disable-client-routes=false") {
		t.Error("routes on not said")
	}
}

func TestEnforceProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "default.json")
	read := func() map[string]any {
		var c map[string]any
		b, _ := os.ReadFile(profile)
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		return c
	}

	// No profile yet: nothing to do (join says it).
	if changed, err := enforceProfile(profile, true); err != nil || changed != nil {
		t.Fatal(changed, err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("a profile was made: %v", err)
	}

	// A profile as TECHO5's built-in NetBird joined it before #595: firewall off. The rest is kept.
	os.WriteFile(profile, []byte(`{"PrivateKey":"abc","WgIface":"netbird0","DisableClientRoutes":true,"DisableServerRoutes":true,
		"DisableDNS":true,"DisableFirewall":true,"DisableIPv6":true,"ServerSSHAllowed":false,"RosenpassEnabled":false,
		"DisableAutoConnect":false,"ManagementURL":{"Scheme":"https","Host":"nb.example.com"}}`), 0o600)
	changed, err := enforceProfile(profile, false)
	if err != nil || !slices.Equal(changed, []string{"DisableFirewall"}) {
		t.Fatalf("changed %v %v", changed, err)
	}
	if c := read(); c["DisableFirewall"] != false || c["PrivateKey"] != "abc" || c["ManagementURL"] == nil {
		t.Errorf("profile: %v", c)
	}

	// Set right already: left as it is, byte for byte.
	b, _ := os.ReadFile(profile)
	if changed, err := enforceProfile(profile, false); err != nil || changed != nil {
		t.Fatal(changed, err)
	}
	if b2, _ := os.ReadFile(profile); !bytes.Equal(b, b2) {
		t.Errorf("profile rewritten: %s", b2)
	}

	// Routes on, and a profile that drifted (another interface, SSH and DNS on): each set back.
	os.WriteFile(profile, []byte(`{"WgIface":"wt0","DisableDNS":false,"ServerSSHAllowed":true}`), 0o600)
	changed, err = enforceProfile(profile, true)
	if err != nil {
		t.Fatal(err)
	}
	c := read()
	if c["WgIface"] != "netbird0" || c["DisableDNS"] != true || c["ServerSSHAllowed"] != false ||
		c["DisableClientRoutes"] != false || c["DisableServerRoutes"] != true || c["DisableIPv6"] != true {
		t.Errorf("profile: %v (changed %v)", c, changed)
	}
	if fi, _ := os.Stat(profile); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}

	os.WriteFile(profile, []byte("{"), 0o600)
	if _, err := enforceProfile(profile, true); err == nil {
		t.Error("no error for a broken profile")
	}
}

func TestRCBlock(t *testing.T) {
	sandbox(t)
	block := rcBlock()
	for _, c := range []struct{ in, want string }{
		{"", "#!/bin/sh\n" + block},
		{"#!/bin/sh\n/data/other/helper &\n", "#!/bin/sh\n/data/other/helper &\n" + block},
		{"#!/bin/sh\n/data/other/helper &", "#!/bin/sh\n/data/other/helper &\n" + block},
		// A script that ends by exec'ing a helper: the block goes before that line.
		{"#!/bin/sh\n# started at every boot\nexec /data/somewhere/helper\n\n",
			"#!/bin/sh\n# started at every boot\n" + block + "exec /data/somewhere/helper\n\n"},
		// Already in: not twice.
		{"#!/bin/sh\n" + block + "/data/other/helper &\n", "#!/bin/sh\n/data/other/helper &\n" + block},
	} {
		if got := withBlock(c.in); got != c.want {
			t.Errorf("withBlock(%q)\n= %q\nwant %q", c.in, got, c.want)
		}
	}
	other := "#!/bin/sh\n/data/other/helper &\nexec /data/x\n"
	if got := withoutBlock(withBlock(other)); got != other {
		t.Errorf("round trip: %q", got)
	}

	// On disk: made executable, and the owner's own lines kept.
	os.WriteFile(rcLocal, []byte(other), 0o644)
	if changed, err := editRC(withBlock); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := editRC(withBlock); err != nil || changed {
		t.Fatal("twice:", changed, err)
	}
	fi, _ := os.Stat(rcLocal)
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("not executable: %v", fi.Mode())
	}
	if changed, err := editRC(withoutBlock); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if b, _ := os.ReadFile(rcLocal); string(b) != other {
		t.Errorf("after uninstall: %q", b)
	}

	// A script install made goes again with uninstall.
	os.Remove(rcLocal)
	if changed, err := editRC(withBlock); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := editRC(withoutBlock); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if _, err := os.Stat(rcLocal); !os.IsNotExist(err) {
		t.Errorf("rc.local left: %v", err)
	}
}

func TestHostname(t *testing.T) {
	for in, want := range map[string]string{
		"Kitchen": "kitchen", "Kid's Room": "kids-room", "  Echo  Show 8 ": "echo-show-8", "Küche": "k-che", "": "techo5", "---": "techo5",
	} {
		if got := hostname(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestKeyed(t *testing.T) {
	sandbox(t)
	psk := filepath.Join(techo5Dir, "psk")
	if keyed() {
		t.Error("keyed without a key")
	}
	os.WriteFile(psk, []byte(base64.StdEncoding.EncodeToString(make([]byte, 32))+"\n"), 0o600)
	if keyed() {
		t.Error("the all-zeros key taken for one")
	}
	k := bytes.Repeat([]byte{7}, 32)
	os.WriteFile(psk, []byte(base64.StdEncoding.EncodeToString(k)+"\n"), 0o600)
	if !keyed() {
		t.Error("a real key not taken")
	}
	os.WriteFile(psk, []byte("not base64"), 0o600)
	if keyed() {
		t.Error("garbage taken for a key")
	}
}

func TestConflict(t *testing.T) {
	sandbox(t)
	st := filepath.Join(techo5Dir, "state.json")
	for _, c := range []struct{ json, err string }{
		{`{"netbird":{"joined":true,"management":"https://nb.example.com"}}`, ""},
		{`{"netbird":{"on":true}}`, "TECHO5's own NetBird is on"},
		{`{"tailscale":{"on":true}}`, "Tailscale is on"},
	} {
		os.WriteFile(st, []byte(c.json), 0o600)
		err := conflict()
		if (c.err == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: %v", c.json, err)
		}
	}
	os.Remove(st)
	if err := conflict(); err != nil {
		t.Errorf("no settings: %v", err)
	}
}

func TestManagementURL(t *testing.T) {
	for in, want := range map[string]string{
		"": defaultManagement, " https://nb.example.com ": "https://nb.example.com", "http://10.0.0.1:33073": "http://10.0.0.1:33073",
		"nb.example.com": "", "ftp://x": "", "https://user:pw@nb.example.com": "",
	} {
		got, err := managementURL(in)
		if (want == "") != (err != nil) || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	if got := shownURL("https://nb.example.com/path?token=secret"); got != "https://nb.example.com" {
		t.Errorf("shown %q", got)
	}
}

func TestScrub(t *testing.T) {
	if got := string(scrub([]byte("error: key ABC-123 refused"), "ABC-123")); strings.Contains(got, "ABC-123") {
		t.Errorf("%q", got)
	}
}

// archive makes a tar.gz like NetBird's release archive.
func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(files[n])), Typeflag: tar.TypeReg})
		tw.Write([]byte(files[n]))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestFetchClient(t *testing.T) {
	sandbox(t)
	good := archive(t, map[string]string{
		"netbird": "#!/bin/sh\necho client\n", "LICENSE": "BSD", "LICENSES/BSD-3-Clause.txt": "bsd",
		"LICENSES/AGPL-3.0.txt": "agpl", "README.md": "readme", "changelog.yml": "log",
	})
	serve := good
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(serve) }))
	defer srv.Close()
	was := clientArchive
	t.Cleanup(func() { clientArchive = was })
	sum := sha256.Sum256(good)
	clientArchive = pinned{URL: srv.URL + "/netbird.tar.gz", Size: int64(len(good)), SHA256: hex.EncodeToString(sum[:])}

	// Anything that is not the pinned archive is refused, and nothing of it is left or unpacked.
	for _, bad := range [][]byte{
		append(append([]byte{}, good[:len(good)-1]...), good[len(good)-1]^1), // same size, other bytes
		good[:len(good)-10], // short
	} {
		serve = bad
		if err := fetchClient(context.Background()); err == nil {
			t.Fatal("a download that does not match was taken")
		}
		if left, _ := os.ReadDir(clientDir()); len(left) != 0 {
			t.Fatalf("left behind: %v", left)
		}
	}

	serve = good
	os.MkdirAll(clientDir(), 0o755)
	os.WriteFile(filepath.Join(clientDir(), "netbird-0.80.0"), []byte("old"), 0o755)
	if err := fetchClient(context.Background()); err != nil {
		t.Fatal(err)
	}
	var names []string
	filepath.WalkDir(clientDir(), func(p string, d os.DirEntry, err error) error {
		if !d.IsDir() {
			r, _ := filepath.Rel(clientDir(), p)
			names = append(names, r)
		}
		return nil
	})
	slices.Sort(names)
	want := []string{"LICENSE", "LICENSES/AGPL-3.0.txt", "LICENSES/BSD-3-Clause.txt", "netbird-" + clientVersion}
	if !slices.Equal(names, want) {
		t.Errorf("client folder %v, want %v", names, want)
	}
	if fi, _ := os.Stat(clientBin()); fi.Mode().Perm() != 0o755 {
		t.Errorf("client mode %v", fi.Mode())
	}

	// There already: not fetched again.
	serve = nil
	if err := fetchClient(context.Background()); err != nil {
		t.Errorf("fetched again: %v", err)
	}
}

func TestUnpackRefuses(t *testing.T) {
	d := t.TempDir()
	for name, files := range map[string]map[string]string{
		"no client": {"LICENSE": "x"},
	} {
		p := filepath.Join(d, "a.tar.gz")
		os.WriteFile(p, archive(t, files), 0o644)
		if err := unpack(p, d, filepath.Join(d, "netbird")); err == nil {
			t.Errorf("%s: unpacked", name)
		}
	}
	// A link where the client should be.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "netbird", Linkname: "/bin/sh", Typeflag: tar.TypeSymlink})
	tw.Close()
	gz.Close()
	p := filepath.Join(d, "b.tar.gz")
	os.WriteFile(p, buf.Bytes(), 0o644)
	if err := unpack(p, d, filepath.Join(d, "netbird")); err == nil {
		t.Error("a link taken for the client")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	sandbox(t)
	c, err := loadConfig()
	if err != nil || c.Management != "" || c.Routes != nil {
		t.Fatal(c, err)
	}
	if err := saveConfig(config{Management: "https://nb.example.com", Joined: true, Routes: []string{"192.168.1.0/24"}}); err != nil {
		t.Fatal(err)
	}
	c, err = loadConfig()
	if err != nil || c.Management != "https://nb.example.com" || !c.Joined || !slices.Equal(c.Routes, []string{"192.168.1.0/24"}) {
		t.Fatal(c, err)
	}
	if fi, _ := os.Stat(configPath()); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
}

func TestMoveDir(t *testing.T) {
	d := t.TempDir()
	from, to := filepath.Join(d, "a"), filepath.Join(d, "b")
	os.MkdirAll(filepath.Join(from, "sub"), 0o700)
	os.WriteFile(filepath.Join(from, "default.json"), []byte("{}"), 0o600)
	if err := moveDir(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "default.json")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(from); !errors.Is(err, os.ErrNotExist) {
		t.Error("source still there")
	}
}

func TestLineWriter(t *testing.T) {
	var out bytes.Buffer
	logger.to = &out
	t.Cleanup(func() { logger.to = nil })
	w := &lineWriter{}
	w.Write([]byte("one\r\ntw"))
	w.Write([]byte("o\n"))
	if got := out.String(); got != "netbird: one\nnetbird: two\n" {
		t.Errorf("%q", got)
	}
}

func TestRotating(t *testing.T) {
	sandbox(t)
	r := &rotating{}
	line := bytes.Repeat([]byte("x"), 1000)
	for range 2 * logLimit / 1000 {
		r.Write(line)
	}
	fi, err := os.Stat(logPath())
	if err != nil || fi.Size() > logLimit {
		t.Errorf("log %v %v", fi, err)
	}
	if fi, err := os.Stat(logPath() + ".1"); err != nil || fi.Size() > logLimit {
		t.Errorf("kept log %v %v", fi, err)
	}
}

func TestReleaseVersion(t *testing.T) {
	for line, want := range map[string][3]int{
		"techo5 rootfs v1.6.47 built 2026-11-21T03:08Z, daemon echod version v1.6.47 (53e11da)": {1, 6, 47},
		"techo5 rootfs v1.10.0 built x": {1, 10, 0},
	} {
		if v, ok := releaseVersion(line); !ok || v != want {
			t.Errorf("%q: %v %v, want %v", line, v, ok, want)
		}
	}
	for _, line := range []string{"", "techo5 rootfs dev", "v1.6", "vx.y.z"} {
		if _, ok := releaseVersion(line); ok {
			t.Errorf("%q: a version, want none", line)
		}
	}
}

func TestHasNetbirdSwitch(t *testing.T) {
	d := t.TempDir()
	was := releaseFile
	t.Cleanup(func() { releaseFile = was })
	releaseFile = filepath.Join(d, "missing")
	if !hasNetbirdSwitch() {
		t.Error("no release file: want the switch assumed")
	}
	for release, want := range map[string]bool{
		"techo5 rootfs v1.6.46 built x": true,
		"techo5 rootfs v1.6.47 built x": false,
		"techo5 rootfs v1.7.0 built x":  false,
		"techo5 rootfs v0.9.99 built x": true,
		"something else":                true,
	} {
		releaseFile = filepath.Join(d, "release")
		if err := os.WriteFile(releaseFile, []byte(release+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := hasNetbirdSwitch(); got != want {
			t.Errorf("%q: %v, want %v", release, got, want)
		}
	}
}
