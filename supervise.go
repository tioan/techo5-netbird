package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// Timings; variables, for tests.
var (
	pollEvery     = 15 * time.Second
	preflightWait = time.Minute
	backoffFirst  = 5 * time.Second
	backoffMax    = 5 * time.Minute
	// steady is how long the daemon must have run for a failure after it to start the backoff afresh.
	steady = 10 * time.Minute
)

// supervisorStatus is what the supervisor last found, in the run folder, for the status command.
type supervisorStatus struct {
	PID     int       `json:"pid"`
	Client  string    `json:"client"`
	Running bool      `json:"running"`
	State   string    `json:"state,omitempty"`
	Name    string    `json:"name,omitempty"`
	IP      string    `json:"ip,omitempty"`
	Routes  []string  `json:"routes_in_use,omitempty"`
	Problem string    `json:"problem,omitempty"`
	Since   time.Time `json:"since"`
}

type supervisor struct {
	reload chan struct{}
	st     supervisorStatus
	// routesOn is whether the running daemon was started to take routes; reselect, that the routes are
	// to be selected again (routes.go).
	routesOn, reselect bool
}

func (s *supervisor) publish(fn func(st *supervisorStatus)) {
	before := s.st
	fn(&s.st)
	s.st.PID, s.st.Client = os.Getpid(), clientVersion
	if s.st.Problem != before.Problem && s.st.Problem != "" {
		logf("%s", s.st.Problem)
	}
	if s.st.State != before.State || s.st.Problem != before.Problem || s.st.Running != before.Running {
		s.st.Since = time.Now()
	}
	b, _ := json.MarshalIndent(s.st, "", "  ")
	_ = writeWhole(statusPath(), 0o644, bytes.NewReader(b))
}

// runSupervisor keeps NetBird's daemon running for as long as the device is up: what rc.local starts.
// One at a time: a second answers at once and leaves the first be.
func runSupervisor() error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errAlreadyRunning
	}
	if err := os.WriteFile(pidPath(), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return err
	}
	defer os.Remove(pidPath())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	s := &supervisor{reload: make(chan struct{}, 1)}
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			select {
			case s.reload <- struct{}{}:
			default:
			}
		}
	}()

	logf("started, NetBird's client %s", clientVersion)
	backoff := backoffFirst
	for ctx.Err() == nil {
		if err := preflight(); err != nil {
			s.publish(func(st *supervisorStatus) { *st = supervisorStatus{Problem: "waiting: " + err.Error()} })
			s.wait(ctx, preflightWait)
			continue
		}
		if err := fetchClient(ctx); err != nil {
			s.publish(func(st *supervisorStatus) { *st = supervisorStatus{Problem: err.Error()} })
			backoff = s.backoff(ctx, backoff)
			continue
		}
		started := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			break
		}
		if err == nil {
			continue // asked to start again (routes on or off)
		}
		s.publish(func(st *supervisorStatus) { *st = supervisorStatus{Problem: err.Error()} })
		if time.Since(started) > steady {
			backoff = backoffFirst
		}
		backoff = s.backoff(ctx, backoff)
	}
	s.publish(func(st *supervisorStatus) { *st = supervisorStatus{Problem: "stopped"} })
	return nil
}

var errAlreadyRunning = errors.New("the supervisor is running already")

// wait waits d, or less if asked to look again (SIGHUP) or to stop.
func (s *supervisor) wait(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-s.reload:
	case <-t.C:
	}
}

// backoff waits d and answers the next wait: twice as long, up to backoffMax.
func (s *supervisor) backoff(ctx context.Context, d time.Duration) time.Duration {
	s.wait(ctx, d)
	return min(2*d, backoffMax)
}

// runOnce runs the daemon until ctx ends, it exits, something comes to conflict with it, or it is to
// start again (nil).
func (s *supervisor) runOnce(ctx context.Context) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	// A daemon left by a supervisor that died without stopping it holds the interface and the socket.
	stopDaemons(sockPath())
	_ = os.Remove(sockPath())
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	routesOn := len(cfg.Routes) > 0
	changed, err := enforceProfile(profilePath(), routesOn)
	if err != nil {
		return fmt.Errorf("NetBird's profile: %w", err)
	}
	if len(changed) > 0 {
		logf("NetBird's profile set to the add-on's settings: %v", changed)
	}
	s.routesOn, s.reselect = routesOn, routesOn

	// The daemon is told to stop when the thread that started it ends, and that thread is kept for as long
	// as it runs, so a supervisor that is killed does not leave it behind.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rctx, stop := context.WithCancel(ctx)
	defer stop()
	cmd := exec.CommandContext(rctx, clientBin(), "service", "run", daemonAddr(),
		"--log-file=console", "--log-level=info")
	cmd.Env = env()
	out := &lineWriter{}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting NetBird: %w", err)
	}
	logf("NetBird's daemon started (pid %d), routes from the mesh %v", cmd.Process.Pid, onOff(routesOn))
	s.publish(func(st *supervisorStatus) { *st = supervisorStatus{Running: true} })
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	halt := func() { stop(); <-exited }

	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	// Quicker at first: the daemon takes a moment to open its socket and connect.
	quick := time.NewTicker(2 * time.Second)
	defer quick.Stop()
	quickUntil := time.Now().Add(time.Minute)
	poll := func() error {
		if err := conflict(); err != nil {
			halt()
			return fmt.Errorf("NetBird stopped: %w", err)
		}
		s.poll(ctx)
		return nil
	}
	if err := poll(); err != nil {
		return err
	}
	for {
		select {
		case err := <-exited:
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("NetBird's daemon exited: %v", err)
		case <-ctx.Done():
			halt()
			return nil
		case <-s.reload:
			cfg, err := loadConfig()
			if err != nil {
				logf("reading the settings again: %v", err)
				continue
			}
			if on := len(cfg.Routes) > 0; on != s.routesOn {
				halt()
				logf("starting NetBird again: routes from the mesh %s", onOff(on))
				return nil
			}
			s.reselect = s.routesOn
			if err := poll(); err != nil {
				return err
			}
		case <-quick.C:
			if time.Now().Before(quickUntil) && s.st.State != connected {
				if err := poll(); err != nil {
					return err
				}
			}
		case <-tick.C:
			if err := poll(); err != nil {
				return err
			}
		}
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// poll reads the daemon's status, keeps the routes to the listed networks, and says what it found.
func (s *supervisor) poll(ctx context.Context) {
	st, err := status(ctx)
	if err != nil {
		s.publish(func(p *supervisorStatus) { *p = supervisorStatus{Running: true, Problem: ""} })
		return
	}
	s.routes(ctx, st)
	s.publish(func(p *supervisorStatus) {
		*p = supervisorStatus{Running: true, State: st.DaemonStatus}
		if st.DaemonStatus == connected {
			p.Name, p.IP, p.Routes = st.name(), st.IP, st.routesInUse()
		}
		switch {
		case needsKey(st.DaemonStatus):
			p.Problem = "needs a setup key: techo5-netbird join"
		case st.DaemonStatus != connected && st.Management.Error != "":
			p.Problem = "the management server: " + st.Management.Error
		}
	})
}

// routes keeps the routes from the mesh to the listed networks while the client takes any and is
// connected: when the list or the daemon is new, and when a route in use is to a network not listed.
func (s *supervisor) routes(ctx context.Context, st daemonStatus) {
	cfg, err := loadConfig()
	if err != nil {
		return
	}
	wanted := prefixes(cfg.Routes)
	if !s.routesOn || len(wanted) == 0 || st.DaemonStatus != connected || (!s.reselect && !unlisted(st.routesInUse(), wanted)) {
		return
	}
	done, err := selectRoutes(ctx, wanted)
	if err != nil {
		logf("routes from the mesh: %v", err)
	}
	if done {
		s.reselect = false
	}
}

// stopDaemons ends any NetBird daemon running on the socket named, and waits a little for it to let go
// of the interface.
func stopDaemons(sock string) {
	for _, pid := range daemonsOn(sock) {
		logf("stopping a NetBird daemon left running (pid %d)", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
			time.Sleep(100 * time.Millisecond)
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// daemonsOn are the pids of NetBird daemons ("service run") on the socket named.
func daemonsOn(sock string) []int {
	want := []byte("--daemon-addr=unix://" + sock)
	entries, _ := os.ReadDir("/proc")
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := bytes.Split(bytes.TrimRight(cmdline, "\x00"), []byte{0})
		if containsArg(args, want) && containsArg(args, []byte("service")) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func containsArg(args [][]byte, a []byte) bool {
	for _, x := range args {
		if bytes.Equal(x, a) {
			return true
		}
	}
	return false
}

// supervisorPID is the running supervisor's pid, or 0.
func supervisorPID() int {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(b)))
	if err != nil || pid <= 0 || syscall.Kill(pid, 0) != nil {
		return 0
	}
	return pid
}

// poke asks the running supervisor to read its settings again and look at once.
func poke() bool {
	pid := supervisorPID()
	return pid != 0 && syscall.Kill(pid, syscall.SIGHUP) == nil
}
