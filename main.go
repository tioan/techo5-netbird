// Command techo5-netbird puts a TECHO5 device on its owner's NetBird network: NetBird's own client, run
// from userdata at boot through TECHO5's rc.local, so it outlasts TECHO5's updates. See README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// version is the add-on's own, set at build time.
var version = "dev"

const usage = `techo5-netbird: NetBird's client on a TECHO5 device, started at boot from rc.local.

  techo5-netbird install             start it at every boot, fetch NetBird's client, start it now
  techo5-netbird join [--management URL]
                                     join with a setup key, read from the terminal or stdin
  techo5-netbird status              where it stands
  techo5-netbird routes              the networks routes from the mesh are taken to
  techo5-netbird routes NET...       take routes to these networks only (e.g. 192.168.1.0/24)
  techo5-netbird routes --clear      take no routes from the mesh
  techo5-netbird migrate             take over TECHO5's built-in NetBird's peer, with no new key
  techo5-netbird uninstall [--purge] stop it and no longer start it at boot; --purge also
                                     deletes the peer's identity and everything else of it
  techo5-netbird version
  techo5-netbird run                 the supervisor itself, as rc.local starts it
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd != "run" {
		logger.to = os.Stdout // commands run by hand say what they do on the terminal
	}
	var err error
	switch cmd {
	case "run":
		err = runSupervisor()
		if errors.Is(err, errAlreadyRunning) {
			err = nil
		}
	case "install":
		err = install()
	case "join":
		err = join(args)
	case "status":
		err = showStatus()
	case "routes":
		err = routesCmd(args)
	case "migrate":
		err = migrate()
	case "uninstall":
		err = uninstall(args)
	case "version", "--version":
		fmt.Printf("techo5-netbird %s, NetBird's client %s\n", version, clientVersion)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "techo5-netbird: %v\n", err)
		os.Exit(1)
	}
}

// install copies the program to its place, fetches NetBird's client, adds the add-on to rc.local, and
// starts the supervisor now, so no restart is needed. Run from elsewhere (/tmp), it installs or updates
// the add-on: the program is replaced whole, by a rename, which a running one does not stand in the way of.
func install() error {
	if err := os.MkdirAll(binDir(), 0o755); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self != selfPath() {
		f, err := os.Open(self)
		if err != nil {
			return err
		}
		err = writeWhole(selfPath(), 0o755, f)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Printf("installed as %s\n", selfPath())
	}
	if err := fetchClient(context.Background()); err != nil {
		return err
	}
	changed, err := editRC(withBlock)
	if err != nil {
		return fmt.Errorf("adding the add-on to %s: %w", rcLocal, err)
	}
	if changed {
		fmt.Printf("added to %s: started at every boot\n", rcLocal)
	}
	// A supervisor running already may be an older release: it is started again, as the one installed.
	if pid := supervisorPID(); pid != 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		waitGone(pid)
		fmt.Println("the running supervisor is stopped, to start the one installed")
	}
	if err := startDetached(); err != nil {
		return fmt.Errorf("starting the supervisor: %w", err)
	}
	fmt.Println("the supervisor is started")
	if err := preflight(); err != nil {
		fmt.Printf("\nNetBird will not run yet: %v\n", err)
		return nil
	}
	fmt.Println("\nNext: techo5-netbird join (or techo5-netbird migrate, for a device on TECHO5's built-in NetBird)")
	return nil
}

// startDetached starts the supervisor in a session of its own, as rc.local would, so it outlives the
// shell that started it.
func startDetached() error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd := exec.Command(selfPath(), "run")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// readKey reads the setup key: from the terminal without echoing it, or from stdin. It is held in memory
// only, never in an argument, a file or the log.
func readKey() (string, error) {
	info, err := os.Stdin.Stat()
	tty := err == nil && info.Mode()&os.ModeCharDevice != 0
	if tty {
		fmt.Fprint(os.Stderr, "NetBird setup key: ")
		if stty("-echo") == nil {
			defer func() { _ = stty("echo"); fmt.Fprintln(os.Stderr) }()
		}
	}
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	key := strings.TrimSpace(line)
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", errors.New("that is not a setup key")
	}
	return key, nil
}

// stty sets the terminal on stdin.
func stty(arg string) error {
	cmd := exec.Command("stty", arg)
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// join joins the device with a setup key, at a management server of the owner's own if one is given.
func join(args []string) error {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	mgmtFlag := fs.String("management", "", "the management server's URL; NetBird's own if none")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgmt, err := managementURL(*mgmtFlag)
	if err != nil {
		return err
	}
	if err := preflight(); err != nil {
		return err
	}
	if supervisorPID() == 0 {
		return errors.New("the supervisor is not running: techo5-netbird install")
	}
	key, err := readKey()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	poke() // a supervisor waiting on a check looks again now
	fmt.Println("waiting for NetBird's daemon to answer...")
	for {
		if _, err := status(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return errors.New("NetBird's daemon is not answering: techo5-netbird status says why")
		case <-time.After(time.Second):
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	name := deviceName()
	out, err := cli(ctx, []string{"NB_SETUP_KEY=" + key}, upArgs(mgmt, name, len(cfg.Routes) > 0)...)
	if err != nil {
		return fmt.Errorf("joining: %s", orElse(lastLine(scrub(out, key)), err.Error()))
	}
	logf("joining as %s at %s", name, shownURL(mgmt))
	cfg.Management, cfg.Joined = mgmt, true
	if mgmt == defaultManagement {
		cfg.Management = ""
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	poke()
	upAt := time.Now()
	for {
		st, err := status(ctx)
		if err == nil && st.DaemonStatus == connected && st.IP != "" {
			fmt.Printf("connected as %s (%s)\n", st.name(), st.IP)
			return nil
		}
		// Still asking for a key a few seconds after taking one: the management server turned it down.
		if err == nil && needsKey(st.DaemonStatus) && time.Since(upAt) > 5*time.Second {
			return errors.New("the management server did not take the key; it may be used up or expired: make a new one")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the key was handed over, but the device has not connected yet (%s)", orElse(st.DaemonStatus, "not answering"))
		case <-time.After(time.Second):
		}
	}
}

func showStatus() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Printf("techo5-netbird %s, NetBird's client %s\n", version, clientVersion)
	_, rcErr := os.Stat(rcLocal)
	b, _ := os.ReadFile(rcLocal)
	switch {
	case rcErr == nil && strings.Contains(string(b), rcBegin):
		fmt.Println("at boot:     started from rc.local")
	default:
		fmt.Println("at boot:     not started (techo5-netbird install)")
	}
	fmt.Printf("management:  %s\n", shownURL(orElse(cfg.Management, defaultManagement)))
	fmt.Printf("routes:      %s\n", orElse(strings.Join(cfg.Routes, " "), "none taken from the mesh"))
	if supervisorPID() == 0 {
		fmt.Println("supervisor:  not running")
		if err := preflight(); err != nil {
			fmt.Printf("problem:     %v\n", err)
		}
		return nil
	}
	var st supervisorStatus
	b, err = os.ReadFile(statusPath())
	if err == nil {
		err = json.Unmarshal(b, &st)
	}
	if err != nil {
		fmt.Println("supervisor:  running, no status yet")
		return nil
	}
	fmt.Printf("supervisor:  running (pid %d)\n", st.PID)
	if st.Running {
		fmt.Printf("netbird:     %s\n", orElse(st.State, "starting"))
	} else {
		fmt.Println("netbird:     not running")
	}
	if st.Name != "" {
		fmt.Printf("connected:   as %s (%s)\n", st.Name, st.IP)
	}
	if len(st.Routes) > 0 {
		fmt.Printf("in use:      %s\n", strings.Join(st.Routes, " "))
	}
	if st.Problem != "" {
		fmt.Printf("problem:     %s\n", st.Problem)
	}
	if !st.Since.IsZero() {
		fmt.Printf("since:       %s\n", st.Since.Format(time.DateTime))
	}
	return nil
}

func routesCmd(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Println(orElse(strings.Join(cfg.Routes, "\n"), "none: no routes are taken from the mesh"))
		return nil
	}
	var routes []string
	if !(len(args) == 1 && args[0] == "--clear") {
		if routes, err = parseRoutes(args); err != nil {
			return err
		}
	}
	cfg.Routes = routes
	if err := saveConfig(cfg); err != nil {
		return err
	}
	if len(routes) == 0 {
		fmt.Println("no routes are taken from the mesh")
	} else {
		fmt.Printf("routes from the mesh are taken to %s only\n", strings.Join(routes, " "))
	}
	if poke() {
		fmt.Println("the supervisor is told; turning routes on or off starts NetBird again")
	}
	return nil
}

// migrate moves TECHO5's built-in NetBird's peer identity to the add-on, with the management server it
// was joined to: the device stays the same peer, and needs no new key. TECHO5's NetBird must be off.
func migrate() error {
	s, err := readTecho5()
	if err != nil {
		return fmt.Errorf("reading TECHO5's settings: %w", err)
	}
	if s.Netbird.On {
		return errors.New("TECHO5's own NetBird is on: turn its NetBird switch off first (on the screen or in Home Assistant)")
	}
	if pids := daemonsOn(builtinSock); len(pids) > 0 {
		return fmt.Errorf("TECHO5's NetBird daemon is still running (pid %d): wait a moment and try again", pids[0])
	}
	if _, err := os.Stat(builtinState() + "/default.json"); err != nil {
		return fmt.Errorf("there is no peer of TECHO5's NetBird to take over (%s)", builtinState())
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.Joined {
		return fmt.Errorf("the add-on has a peer of its own already (%s); techo5-netbird uninstall --purge first to replace it", stateDir())
	}
	if pid := supervisorPID(); pid != 0 {
		// Its daemon, not joined yet, holds the state folder: stopped, and started again below.
		_ = syscall.Kill(pid, syscall.SIGTERM)
		waitGone(pid)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	// The daemon makes a profile of its own when it first starts, with no peer behind it: that goes.
	_ = os.RemoveAll(stateDir())
	if err := moveDir(builtinState(), stateDir()); err != nil {
		return fmt.Errorf("moving the peer: %w", err)
	}
	cfg.Management, cfg.Joined = s.Netbird.Management, true
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("TECHO5's NetBird peer moved to %s (management: %s)\n", stateDir(), shownURL(orElse(cfg.Management, defaultManagement)))
	fmt.Println("Leave TECHO5's own NetBird switch off from now on: that NetBird would need a new key, and both cannot run.")
	if _, err := os.Stat(selfPath()); err == nil && supervisorPID() == 0 {
		if err := startDetached(); err != nil {
			return fmt.Errorf("starting the supervisor: %w", err)
		}
		fmt.Println("the supervisor is started")
	}
	return nil
}

// moveDir renames from to to, or copies and removes it where they are on different filesystems.
func moveDir(from, to string) error {
	err := os.Rename(from, to)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		return err
	}
	return os.RemoveAll(from)
}

func waitGone(pid int) {
	for i := 0; i < 150 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

// uninstall takes the add-on out of rc.local and stops it. Its peer is kept unless --purge.
func uninstall(args []string) error {
	purge := len(args) == 1 && args[0] == "--purge"
	if len(args) > 0 && !purge {
		return errors.New("uninstall takes --purge, or nothing")
	}
	changed, err := editRC(withoutBlock)
	if err != nil {
		return fmt.Errorf("taking the add-on out of %s: %w", rcLocal, err)
	}
	if changed {
		fmt.Printf("taken out of %s: no longer started at boot\n", rcLocal)
	}
	if pid := supervisorPID(); pid != 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		waitGone(pid)
		fmt.Println("the supervisor is stopped")
	}
	stopDaemons(sockPath())
	// The run folder holds only the stopped supervisor's lock, status and socket.
	_ = os.RemoveAll(runDir)
	if !purge {
		fmt.Printf("the peer's identity is kept in %s; techo5-netbird uninstall --purge deletes it\n", stateDir())
		return nil
	}
	if err := os.RemoveAll(home); err != nil {
		return err
	}
	fmt.Printf("%s deleted. Remove the device under Peers in your NetBird dashboard too.\n", home)
	return nil
}
