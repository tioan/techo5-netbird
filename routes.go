package main

// Routes from the mesh (role 1 of HuskerMinion/techo5#580): the device may use the routes the mesh offers
// it, but only to the networks its owner lists, and none until somebody lists one. It is a client of those
// routes only: it routes nothing of its own into the mesh, forwards nothing, and takes no exit node.
//
// NetBird's client has no setting for "only these networks". It takes the routes its management server
// gives it, all of them selected unless deselected (`netbird networks select`), and a route that comes
// along later is selected too. So the client is told to take routes at all only while the list has
// something in it, and its selection is kept to the listed networks: whenever the routes in use are not
// all listed ones, and whenever the list or the daemon is new.
//
// Adapted from the author's own HuskerMinion/techo5#603 (MIT), which TECHO5 did not take in.

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// maxRoutes is how many networks may be listed: a few homes' LANs, not a routing table.
const maxRoutes = 16

// minRouteBits is the widest network that may be listed. Wider is a default route in pieces, which is an
// exit node, and NetBird's client in legacy routing mode drops anything wider than /8 anyway.
const minRouteBits = 8

// parseRoutes reads networks given one per argument, or separated by commas or spaces, and answers them
// in their usual form, or what is wrong with them. None is fine: that turns routes off.
func parseRoutes(args []string) ([]string, error) {
	fields := strings.FieldsFunc(strings.Join(args, " "), func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
	local := localNets()
	var out []string
	for _, f := range fields {
		p, err := netip.ParsePrefix(f)
		if err != nil {
			return nil, fmt.Errorf("%q is not a network, such as 192.168.1.0/24", f)
		}
		switch {
		case !p.Addr().Is4():
			return nil, fmt.Errorf("%s: only IPv4 networks (NetBird runs without IPv6 here)", f)
		case p != p.Masked():
			return nil, fmt.Errorf("%s: that is an address in %s; give the network", f, p.Masked())
		case p.Bits() < minRouteBits:
			return nil, fmt.Errorf("%s is too wide: /%d at the most (no default route, no exit node)", f, minRouteBits)
		case p.Addr().IsLoopback() || p.Addr().IsLinkLocalUnicast() || p.Addr().IsMulticast() || p.Addr().IsUnspecified() ||
			p.Overlaps(netip.MustParsePrefix("240.0.0.0/4")):
			return nil, fmt.Errorf("%s is not a network a route can go to", f)
		}
		for _, l := range local {
			if p.Overlaps(l) {
				return nil, fmt.Errorf("%s overlaps this device's own network %s", f, l)
			}
		}
		if s := p.String(); !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if len(out) > maxRoutes {
		return nil, fmt.Errorf("%d networks: %d at the most", len(out), maxRoutes)
	}
	return out, nil
}

// localNets are the networks this device is on itself, which a route from the mesh must not take over:
// its Wi-Fi's, and the mesh's own, by the addresses its interfaces have. A variable, for tests.
var localNets = func() []netip.Prefix {
	var out []netip.Prefix
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			p, ok := netip.AddrFromSlice(n.IP)
			if !ok || !p.Unmap().Is4() {
				continue
			}
			bits, _ := n.Mask.Size()
			out = append(out, netip.PrefixFrom(p.Unmap(), bits).Masked())
		}
	}
	return out
}

// prefixes are the listed networks.
func prefixes(routes []string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range routes {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// within is whether a route's network lies inside one of the listed ones. A route's network that is not
// one (a domain route's names) is not.
func within(network string, wanted []netip.Prefix) bool {
	p, err := netip.ParsePrefix(strings.TrimSpace(network))
	if err != nil {
		return false
	}
	for _, w := range wanted {
		if w.Bits() <= p.Bits() && w.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// unlisted is whether any route in use goes to a network that is not listed.
func unlisted(inUse []string, wanted []netip.Prefix) bool {
	for _, n := range inUse {
		if !within(n, wanted) {
			return true
		}
	}
	return false
}

// network is one route the client was offered, as `netbird networks list` shows it.
type network struct {
	ID       string
	Range    string // one network, or a few joined by ", "; empty for a domain route
	Selected bool
}

// parseNetworks reads `netbird networks list`. Its text is all the command line has, so it is read
// strictly: an entry starts at "- ID:", and only a "Network:" line gives it a range.
func parseNetworks(out []byte) []network {
	var nets []network
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if id, ok := strings.CutPrefix(line, "- ID:"); ok {
			nets = append(nets, network{ID: strings.TrimSpace(id)})
			continue
		}
		if len(nets) == 0 {
			continue
		}
		n := &nets[len(nets)-1]
		if r, ok := strings.CutPrefix(line, "Network:"); ok {
			n.Range = strings.TrimSpace(r)
		} else if s, ok := strings.CutPrefix(line, "Status:"); ok {
			n.Selected = strings.TrimSpace(s) == "Selected"
		}
	}
	return slices.DeleteFunc(nets, func(n network) bool { return n.ID == "" })
}

// choose splits the routes offered into those to take, every network of which is listed, and the rest.
func choose(nets []network, wanted []netip.Prefix) (take, leave []string) {
	for _, n := range nets {
		ok := n.Range != ""
		for _, r := range strings.Split(n.Range, ",") {
			ok = ok && within(r, wanted)
		}
		if ok {
			take = append(take, n.ID)
		} else {
			leave = append(leave, n.ID)
		}
	}
	return take, leave
}

// selectRoutes keeps the client's selection to the listed networks. "select" replaces the selection with
// the routes named; with none to name, the others are deselected by name. Never "deselect all": that
// would also refuse a listed network the mesh starts to offer later. Done is whether the mesh offered any
// routes to choose from: until it does, a selection kept from before still stands.
func selectRoutes(ctx context.Context, wanted []netip.Prefix) (done bool, err error) {
	out, err := cli(ctx, nil, "networks", "list")
	if err != nil {
		return false, fmt.Errorf("listing the mesh's routes: %s", orElse(lastLine(out), err.Error()))
	}
	take, leave := choose(parseNetworks(out), wanted)
	switch {
	case len(take) > 0:
		out, err = cli(ctx, nil, append([]string{"networks", "select"}, take...)...)
	case len(leave) > 0:
		out, err = cli(ctx, nil, append([]string{"networks", "deselect"}, leave...)...)
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("selecting the mesh's routes: %s", orElse(lastLine(out), err.Error()))
	}
	logf("routes from the mesh selected: taken [%s], left [%s]", strings.Join(take, " "), strings.Join(leave, " "))
	return true, nil
}
