package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	// Root certificates for when the device has none the program can find. Used only then: the
	// system's own come first.
	_ "golang.org/x/crypto/x509roots/fallback"
)

// The NetBird client this release of the add-on runs: NetBird's own release archive, as NetBird
// published it, pinned by size and SHA-256 (docs/client-binary.md). A new NetBird version is a new
// release of the add-on, after it was tested on a device.
const clientVersion = "0.83.2"

type pinned struct {
	URL    string
	Size   int64
	SHA256 string
}

// clientArchive is a variable, for tests.
var clientArchive = pinned{
	URL:    "https://github.com/netbirdio/netbird/releases/download/v" + clientVersion + "/netbird_" + clientVersion + "_linux_arm64.tar.gz",
	Size:   14731206,
	SHA256: "856eece869b6e2808760cc0e3f30d8f9d639728e5c1c5410a8895de28d7244e0",
}

// clientFile is the name NetBird's archive has beside the program in a catalog install: TECHO5's add-on
// catalog pins it next to the add-on's binary (addon.json's "files"), and the device downloads and checks
// it, so the add-on never downloads it there itself.
const clientFile = "netbird.tar.gz"

// clientBin is where the pinned client is kept, by its version, so a release that pins a newer one
// fetches it.
func clientBin() string { return filepath.Join(clientDir(), "netbird-"+clientVersion) }

// room is what a download needs free: the archive and the client unpacked beside it, with some to spare.
const room = 3 * 42 << 20

// httpClient is a variable, for tests.
var httpClient = &http.Client{Timeout: 10 * time.Minute}

// fetchClient makes sure the pinned client is here: downloaded, checked and unpacked, unless it already
// is. Nothing of a download that does not match is ever unpacked or run. Older versions go.
func fetchClient(ctx context.Context) error {
	bin := clientBin()
	if info, err := os.Stat(bin); err == nil && info.Mode().IsRegular() {
		return nil
	}
	dir := clientDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var fs syscall.Statfs_t
	if syscall.Statfs(dir, &fs) == nil && int64(fs.Bavail)*int64(fs.Bsize) < room {
		return fmt.Errorf("not enough room on userdata for NetBird's client (it needs %d MB)", room>>20)
	}
	archive := filepath.Join(dir, "netbird-"+clientVersion+".tar.gz")
	if catalog {
		// What the catalog pinned, checked again against this release's own pin before it is unpacked.
		src := filepath.Join(catalogBin, clientFile)
		f, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("NetBird's client is not where TECHO5's add-on catalog puts it (%s): %w", src, err)
		}
		err = saveChecked(f, clientArchive, archive)
		f.Close()
		if err != nil {
			return fmt.Errorf("NetBird's client in %s: %w", src, err)
		}
	} else {
		logf("downloading NetBird's client %s", clientVersion)
		if err := download(ctx, clientArchive, archive); err != nil {
			return fmt.Errorf("downloading NetBird's client: %w", err)
		}
	}
	defer os.Remove(archive)
	if err := unpack(archive, dir, bin); err != nil {
		return fmt.Errorf("unpacking NetBird's client: %w", err)
	}
	old, _ := filepath.Glob(filepath.Join(dir, "netbird-*"))
	for _, o := range old {
		if o != bin && o != archive {
			_ = os.Remove(o)
		}
	}
	logf("NetBird's client %s is ready", clientVersion)
	return nil
}

// download fetches p to the file to, checked against its size and SHA-256 as it comes: kept beside to
// until it is whole and matches, and removed if it does not.
func download(ctx context.Context, p pinned, to string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "techo5-netbird")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", p.URL, resp.Status)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != p.Size {
		return fmt.Errorf("the server says %d bytes, the add-on expects %d", resp.ContentLength, p.Size)
	}
	return saveChecked(resp.Body, p, to)
}

// saveChecked copies r to the file to, checked against p's size and SHA-256 as it comes: kept beside to
// until it is whole and matches, and removed if it does not.
func saveChecked(r io.Reader, p pinned, to string) error {
	part := to + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer os.Remove(part)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, p.Size+1))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n != p.Size {
		return fmt.Errorf("got %d bytes, expected %d", n, p.Size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, p.SHA256) {
		return fmt.Errorf("SHA-256 %s does not match the pinned %s", got, p.SHA256)
	}
	return os.Rename(part, to)
}

// unpack puts the archive's client at bin and NetBird's licenses beside it in dir: each written whole
// and then renamed, so a device stopped halfway has no half a client. The rest of the archive (README,
// changelog) is left out.
func unpack(archive, dir, bin string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	got := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(h.Name, "./")
		var to string
		var mode os.FileMode
		switch {
		case name == "netbird":
			to, mode = bin, 0o755
		case name == "LICENSE":
			to, mode = filepath.Join(dir, "LICENSE"), 0o644
		case strings.HasPrefix(name, "LICENSES/") && !strings.Contains(name[len("LICENSES/"):], "/") &&
			!strings.Contains(name, ".."):
			if err := os.MkdirAll(filepath.Join(dir, "LICENSES"), 0o755); err != nil {
				return err
			}
			to, mode = filepath.Join(dir, name), 0o644
		default:
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("%s is not a file", name)
		}
		if err := writeWhole(to, mode, tr); err != nil {
			return err
		}
		got = got || name == "netbird"
	}
	if !got {
		return errors.New("there is no client in it")
	}
	return nil
}

// writeWhole writes r to the file to, whole or not at all.
func writeWhole(to string, mode os.FileMode, r io.Reader) error {
	tmp := to + ".part"
	w, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	if err == nil {
		err = w.Sync()
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}
