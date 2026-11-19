package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
)

// The add-on is started at boot from TECHO5's rc.local, which other add-ons may use too. It owns only the
// lines between its markers: install adds them, uninstall takes them out, and nothing else in the file
// is touched. Its line puts the supervisor in the background, so whatever comes after it still runs.
const (
	rcBegin = "# techo5-netbird begin: added by techo5-netbird install, removed by techo5-netbird uninstall"
	rcEnd   = "# techo5-netbird end"
)

func rcBlock() string {
	return rcBegin + "\n" + selfPath() + " run > /dev/null 2>&1 &\n" + rcEnd + "\n"
}

// withBlock is the script s with the add-on's block in it, once, at the end; a script that is new
// starts with a shebang.
func withBlock(s string) string {
	s = withoutBlock(s)
	if strings.TrimSpace(s) == "" {
		s = "#!/bin/sh\n"
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	// A script that ends by exec'ing a helper never reaches what comes after it: the block goes before
	// such a line, so it still runs.
	lines := strings.SplitAfter(s, "\n")
	at := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "exec ") {
			at = i
		}
		break
	}
	return strings.Join(lines[:at], "") + rcBlock() + strings.Join(lines[at:], "")
}

// withoutBlock is the script s with the add-on's block taken out, and nothing else changed.
func withoutBlock(s string) string {
	var out []string
	in := false
	for _, l := range strings.SplitAfter(s, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "# techo5-netbird begin"):
			in = true
		case in && t == rcEnd:
			in = false
		case !in:
			out = append(out, l)
		}
	}
	return strings.Join(out, "")
}

// editRC changes rc.local with fn, keeps it executable, and leaves it alone when nothing changes. A script
// left with nothing but its shebang, as install made it, is removed: TECHO5 starts nothing then.
func editRC(fn func(string) string) (changed bool, err error) {
	b, err := os.ReadFile(rcLocal)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	mode := os.FileMode(0o755)
	if info, err := os.Stat(rcLocal); err == nil {
		mode = info.Mode().Perm() | 0o700
	}
	out := fn(string(b))
	if len(b) > 0 && strings.TrimSpace(out) == "#!/bin/sh" {
		return true, os.Remove(rcLocal)
	}
	if out == string(b) {
		if len(b) > 0 {
			return false, os.Chmod(rcLocal, mode)
		}
		return false, nil
	}
	return true, writeWhole(rcLocal, mode, bytes.NewReader([]byte(out)))
}
