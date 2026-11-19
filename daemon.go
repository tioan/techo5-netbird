package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// NetBird's own words for where its daemon stands.
const (
	connected  = "Connected"
	needsLogin = "NeedsLogin"
)

func daemonAddr() string { return "--daemon-addr=unix://" + sockPath() }

// env is what the client is run with, as TECHO5's built-in NetBird ran it: its state in the add-on's
// folder, and the choices that suit these kernels.
func env(extra ...string) []string {
	return append(append(os.Environ(),
		"NB_STATE_DIR="+stateDir(),
		// The command line keeps its own few files under the user's config: the add-on's, not root's home.
		"HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "cli"),
		// WireGuard in the client: most of these kernels have no WireGuard module.
		"NB_WG_KERNEL_DISABLED=true",
		// Routing the old way, and no eBPF: these kernels may have neither policy routing nor BPF to spare.
		"NB_USE_LEGACY_ROUTING=true", "NB_DISABLE_EBPF_WG_PROXY=true",
		// Nothing sent to NetBird but what joining and connecting take.
		"NB_METRICS_PUSH_ENABLED=false",
	), extra...)
}

func command(ctx context.Context, extraEnv []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, clientBin(), append(args, daemonAddr(), "--log-file=console")...)
	cmd.Env = env(extraEnv...)
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// cli runs the client's command line against its daemon, and answers what it said. A variable, for tests.
var cli = func(ctx context.Context, extraEnv []string, args ...string) ([]byte, error) {
	return command(ctx, extraEnv, args...).CombinedOutput()
}

// upArgs joins the device: everything the network could give it is refused, but routes to the networks
// listed (routes.go), and the client's own firewall kept on for the access policies. Each is said
// outright: the client keeps only what it is told. profileSettings says the same for a joined profile.
func upArgs(mgmt, name string, routesOn bool) []string {
	return []string{"up",
		"--management-url=" + mgmt, "--hostname=" + name, "--interface-name=" + iface,
		"--disable-dns", fmt.Sprintf("--disable-client-routes=%v", !routesOn), "--disable-server-routes",
		"--disable-firewall=false", "--disable-ipv6",
		"--allow-server-ssh=false", "--enable-rosenpass=false", "--disable-auto-connect=false"}
}

// daemonStatus is the part of the client's status --json read here.
type daemonStatus struct {
	DaemonStatus string `json:"daemonStatus"`
	IP           string `json:"netbirdIp"`
	FQDN         string `json:"fqdn"`
	Peers        struct {
		Details []struct {
			Networks []string `json:"networks"` // the routes in use through this peer
		} `json:"details"`
	} `json:"peers"`
	Management struct {
		Connected bool   `json:"connected"`
		Error     string `json:"error"`
	} `json:"management"`
}

// status asks the daemon where it stands. A variable, for tests.
var status = func(ctx context.Context) (daemonStatus, error) {
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// The answer only: the command line logs to stderr, and a line of that is not JSON.
	cmd := command(sctx, nil, "status", "--json")
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		return daemonStatus{}, fmt.Errorf("%v: %s", err, lastLine(errOut.Bytes()))
	}
	return parseStatus(out)
}

func parseStatus(out []byte) (daemonStatus, error) {
	var st daemonStatus
	if err := json.Unmarshal(out, &st); err != nil {
		return st, err
	}
	// The address comes with its network's prefix length.
	st.IP, _, _ = strings.Cut(st.IP, "/")
	return st, nil
}

// name is the device's name on the network, from its FQDN.
func (st daemonStatus) name() string {
	return strings.TrimSuffix(strings.SplitN(st.FQDN, ".", 2)[0], ".")
}

// routesInUse are the routes the device uses, through whichever peers route them. (The status's own
// "networks" are the routes it offers, which it never does here.)
func (st daemonStatus) routesInUse() []string {
	var out []string
	for _, p := range st.Peers.Details {
		for _, n := range p.Networks {
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	slices.Sort(out)
	return out
}

func needsKey(s string) bool {
	return s == needsLogin || s == "LoginFailed" || s == "SessionExpired"
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// scrub takes a secret out of what the client said, before it is shown or logged.
func scrub(b []byte, secret string) []byte {
	if secret == "" {
		return b
	}
	return bytes.ReplaceAll(b, []byte(secret), []byte("[setup key]"))
}
