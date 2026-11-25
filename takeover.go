package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// What an install from TECHO5's add-on catalog takes over on its first start, while it has no peer of
// its own, so that nobody needs a new setup key: the peer of an install by hand, or else the one the
// built-in NetBird left behind (TECHO5 v1.6.47 turned it off and kept its identity for this).

// builtinPeer is TECHO5's settings, when the built-in NetBird left a peer that can be taken over now.
func builtinPeer() (techo5Settings, error) {
	s, err := readTecho5()
	if err != nil {
		return s, fmt.Errorf("reading TECHO5's settings: %w", err)
	}
	if s.Netbird.On {
		return s, errors.New("TECHO5's own NetBird is on: turn its NetBird switch off first (on the screen or in Home Assistant)")
	}
	if pids := daemonsOn(builtinSock); len(pids) > 0 {
		return s, fmt.Errorf("TECHO5's NetBird daemon is still running (pid %d): wait a moment and try again", pids[0])
	}
	if _, err := os.Stat(filepath.Join(builtinState(), "default.json")); err != nil {
		return s, fmt.Errorf("there is no peer of TECHO5's NetBird to take over (%s)", builtinState())
	}
	return s, nil
}

// takeBuiltin moves the built-in's peer into the add-on's state folder, with the management server it
// was joined to. No daemon of the add-on's may hold that folder.
func takeBuiltin(s techo5Settings, cfg config) (config, error) {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return cfg, err
	}
	// The daemon makes a profile of its own when it first starts, with no peer behind it: that goes.
	_ = os.RemoveAll(stateDir())
	if err := moveDir(builtinState(), stateDir()); err != nil {
		return cfg, fmt.Errorf("moving the peer: %w", err)
	}
	cfg.Management, cfg.Joined = s.Netbird.Management, true
	return cfg, saveConfig(cfg)
}

// handSupervisor is the pid of a supervisor the install by hand runs, or 0. The two share the run
// folder's lock and pid file; the hand install's program is the one in its own bin folder.
func handSupervisor() int {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || pid == os.Getpid() || syscall.Kill(pid, 0) != nil {
		return 0
	}
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil || !strings.HasPrefix(exe, handHome+"/") {
		return 0
	}
	return pid
}

// stopHand takes the install by hand out of rc.local and stops its supervisor, and with it its daemon.
func stopHand() error {
	if changed, err := editRC(withoutBlock); err != nil {
		return fmt.Errorf("taking the install by hand out of %s: %w", rcLocal, err)
	} else if changed {
		logf("the install by hand is no longer started from %s", rcLocal)
	}
	if pid := handSupervisor(); pid != 0 {
		logf("stopping the install by hand (pid %d)", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		waitGone(pid)
	}
	stopDaemons(sockPath())
	return nil
}

// handPeer is the install by hand's settings, when it has a peer to take over.
func handPeer() (config, bool) {
	var c config
	b, err := os.ReadFile(filepath.Join(handHome, "config.json"))
	if err != nil {
		return c, false
	}
	if err := json.Unmarshal(b, &c); err != nil || !c.Joined {
		return c, false
	}
	if _, err := os.Stat(filepath.Join(handHome, "state", "default.json")); err != nil {
		return c, false
	}
	return c, true
}

// takeHand moves the install by hand's peer and settings into the catalog install's state folder, and
// deletes what is left of it: its program, NetBird's client, its log.
func takeHand(hc config) error {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(stateDir())
	if err := moveDir(filepath.Join(handHome, "state"), stateDir()); err != nil {
		return fmt.Errorf("moving the peer: %w", err)
	}
	if err := saveConfig(hc); err != nil {
		return err
	}
	return os.RemoveAll(handHome)
}

// takeOver runs once a catalog install's supervisor holds the lock, before NetBird starts: an install
// with no peer of its own yet takes the hand install's, or the built-in's. Nothing to take is no error:
// the device then waits for a join.
func takeOver() {
	cfg, err := loadConfig()
	if err != nil || cfg.Joined {
		return
	}
	if hc, ok := handPeer(); ok {
		if err := takeHand(hc); err != nil {
			logf("taking over the install by hand: %v", err)
			return
		}
		logf("took over the install by hand's peer (management: %s); %s is deleted", shownURL(orElse(hc.Management, defaultManagement)), handHome)
		return
	}
	s, err := builtinPeer()
	if err != nil {
		return
	}
	if cfg, err = takeBuiltin(s, cfg); err != nil {
		logf("taking over TECHO5's NetBird peer: %v", err)
		return
	}
	logf("took over TECHO5's NetBird peer (management: %s)", shownURL(orElse(cfg.Management, defaultManagement)))
}
