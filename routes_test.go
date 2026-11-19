package main

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestParseRoutes(t *testing.T) {
	was := localNets
	t.Cleanup(func() { localNets = was })
	localNets = func() []netip.Prefix {
		return []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24"), netip.MustParsePrefix("100.64.0.0/16")}
	}
	for _, c := range []struct {
		in   []string
		want []string
		err  string
	}{
		{nil, nil, ""},
		{[]string{"  ", ""}, nil, ""},
		{[]string{"192.168.1.0/24"}, []string{"192.168.1.0/24"}, ""},
		{[]string{"192.168.1.0/24,10.20.0.0/16", "192.168.1.0/24;172.16.5.4/32"}, []string{"192.168.1.0/24", "10.20.0.0/16", "172.16.5.4/32"}, ""},
		{[]string{"192.168.1.0"}, nil, "not a network"},
		{[]string{"home"}, nil, "not a network"},
		{[]string{"fd00::/64"}, nil, "only IPv4"},
		{[]string{"192.168.1.7/24"}, nil, "give the network"},
		{[]string{"0.0.0.0/0"}, nil, "too wide"},
		{[]string{"0.0.0.0/1"}, nil, "too wide"},
		{[]string{"10.0.0.0/7"}, nil, "too wide"},
		{[]string{"127.0.0.0/8"}, nil, "not a network a route"},
		{[]string{"169.254.0.0/16"}, nil, "not a network a route"},
		{[]string{"224.0.0.0/8"}, nil, "not a network a route"},
		{[]string{"250.0.0.0/8"}, nil, "not a network a route"},
		{[]string{"192.168.50.0/24"}, nil, "own network 192.168.50.0/24"},
		{[]string{"192.168.0.0/16"}, nil, "own network 192.168.50.0/24"},
		{[]string{"192.168.50.128/25"}, nil, "own network"},
		{[]string{"100.64.3.0/24"}, nil, "own network 100.64.0.0/16"},
		{strings.Fields(strings.Repeat("10.1.0.0/16 ", 20)), []string{"10.1.0.0/16"}, ""}, // the same twice is once
	} {
		got, err := parseRoutes(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%q: %q %v, want %q", c.in, got, err, c.want)
		}
	}
	var many []string
	for i := range maxRoutes + 1 {
		many = append(many, netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(i), 0, 0}), 16).String())
	}
	if _, err := parseRoutes(many); err == nil || !strings.Contains(err.Error(), "at the most") {
		t.Errorf("%d networks: %v", len(many), err)
	}
}

// What `netbird networks list` says (client/cmd/networks.go, v0.81.4).
const networksList = `Available Networks:

  - ID: home-lan
    Network: 192.168.1.0/24
    Status: Selected

  - ID: office
    Network: 10.20.0.0/16
    Status: Not Selected

  - ID: names
    Domains: *.example.com
    Status: Selected
    Resolved IPs: -

  - ID: printer
    Network: 192.168.1.40/32
    Status: Selected

  - ID: exit
    Network: 0.0.0.0/0, ::/0
    Status: Not Selected
`

func TestParseNetworks(t *testing.T) {
	got := parseNetworks([]byte(networksList))
	want := []network{
		{"home-lan", "192.168.1.0/24", true},
		{"office", "10.20.0.0/16", false},
		{"names", "", true},
		{"printer", "192.168.1.40/32", true},
		{"exit", "0.0.0.0/0, ::/0", false},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v", got)
	}
	if got := parseNetworks([]byte("No networks available.\n")); len(got) != 0 {
		t.Errorf("none: %+v", got)
	}
}

func TestChoose(t *testing.T) {
	nets := parseNetworks([]byte(networksList))
	for _, c := range []struct {
		wanted      []string
		take, leave []string
	}{
		{[]string{"192.168.1.0/24"}, []string{"home-lan", "printer"}, []string{"office", "names", "exit"}},
		{[]string{"192.168.1.40/32"}, []string{"printer"}, []string{"home-lan", "office", "names", "exit"}},
		{[]string{"10.0.0.0/8", "192.168.1.0/24"}, []string{"home-lan", "office", "printer"}, []string{"names", "exit"}},
		{[]string{"172.16.0.0/12"}, nil, []string{"home-lan", "office", "names", "printer", "exit"}},
	} {
		take, leave := choose(nets, prefixes(c.wanted))
		if !slices.Equal(take, c.take) || !slices.Equal(leave, c.leave) {
			t.Errorf("%v: take %v leave %v", c.wanted, take, leave)
		}
	}
}

func TestUnlisted(t *testing.T) {
	wanted := prefixes([]string{"192.168.1.0/24"})
	for _, c := range []struct {
		inUse []string
		want  bool
	}{
		{nil, false},
		{[]string{"192.168.1.0/24"}, false},
		{[]string{"192.168.1.40/32", "192.168.1.0/24"}, false},
		{[]string{"192.168.1.0/24", "10.20.0.0/16"}, true},
		{[]string{"192.168.0.0/16"}, true}, // wider than listed
		{[]string{"*.example.com"}, true},
	} {
		if got := unlisted(c.inUse, wanted); got != c.want {
			t.Errorf("%v: %v", c.inUse, got)
		}
	}
}
