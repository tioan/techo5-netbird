package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// The supervisor's log, and the daemon's with it, go to a file of the add-on's own that never grows
// past logLimit: at that size it is kept as .1 and a new one is begun. TECHO5's rc.local.log is
// replaced only at a boot, and a device can be up for months.
const logLimit = 1 << 20

var logger = &rotating{}

type rotating struct {
	mu   sync.Mutex
	f    *os.File
	size int64
	// to, when set, is where lines go instead (the commands run by hand write to the terminal).
	to io.Writer
}

func (r *rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.to != nil {
		return r.to.Write(p)
	}
	if r.f == nil || r.size+int64(len(p)) > logLimit {
		if r.f != nil {
			r.f.Close()
			_ = os.Rename(logPath(), logPath()+".1")
		}
		f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return len(p), nil // a log that cannot be written is not a reason to stop
		}
		info, _ := f.Stat()
		r.f, r.size = f, 0
		if info != nil {
			r.size = info.Size()
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func logf(format string, args ...any) {
	fmt.Fprintf(logger, "%s techo5-netbird: %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// lineWriter passes the daemon's output on to the log line by line, with a prefix of its own.
type lineWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		fmt.Fprintf(logger, "netbird: %s\n", bytes.TrimRight(w.buf[:i], "\r"))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 8<<10 { // a line that never ends is cut
		fmt.Fprintf(logger, "netbird: %s\n", w.buf)
		w.buf = w.buf[:0]
	}
	return len(p), nil
}
