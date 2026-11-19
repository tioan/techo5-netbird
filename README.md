# techo5-netbird

**English** | [Deutsch](README.de.md)

Puts a [TECHO5](https://github.com/HuskerMinion/techo5) device on your own [NetBird](https://netbird.io)
network, so a Home Assistant somewhere else can reach it. Use NetBird's own service or a management
server you run yourself.

It is NetBird's own client, run from userdata and started at every boot through TECHO5's
[rc.local](https://github.com/HuskerMinion/techo5/blob/main/docs/setup.md#running-something-of-your-own-at-boot),
so it outlasts TECHO5's updates. It is made and kept by its author, not by TECHO5, and TECHO5 does not
test it.

## What it does, and what it doesn't

It runs NetBird the way TECHO5's built-in NetBird did:

- **Home Assistant over NetBird.** Home Assistant reaches the device on its encrypted ESPHome
  connection (port 6053), at its NetBird name or address.
- **Nothing else gets in.** TECHO5's own firewall lets NetBird's interface (`netbird0`) reach only
  ports 6053, 8181 (the setup page, only while Remote setup is on) and 22 (SSH, if you turned it on),
  and ping. The add-on does not start NetBird unless that firewall rule is in place. NetBird's access
  policies narrow it further, per peer; a policy can never open more.
- **The device takes nothing from the network.** No DNS, no IPv6, no NetBird SSH server, no routes
  (unless you list networks: see [Routes from the mesh](#routes-from-the-mesh)). It routes nothing of
  its own into the mesh and is no exit node. The client sends NetBird no metrics.
- **NetBird's client, as NetBird publishes it.** The add-on downloads NetBird's own release archive for
  arm64 and checks its size and SHA-256 against the values pinned in this release before it unpacks
  it. Why it doesn't build its own: [docs/client-binary.md](docs/client-binary.md).

There is no switch or status in Home Assistant and nothing on the screen: the add-on is set up and
looked at over SSH.

## Devices

Every TECHO5 device that ran TECHO5's built-in NetBird and starts `rc.local` at boot: the Echo Shows
and the Echo Spot. One arm64 program runs on all of them, the Show 8 (2nd gen) with its 32-bit
userland included.

**Tested on:** an Echo Show 8 (2nd gen). The Echo Spot should work the same way, but has not been
tested. Not for the Echo Dots and the Skylight.

## What you need

- TECHO5 with SSH on and your key set (Settings on the device, or the setup page).
- An **encryption key** on Home Assistant's link to the device. Without one, the add-on does not start
  NetBird: over NetBird, the link would be open in the clear.
- TECHO5's own NetBird **off**, and Tailscale off. NetBird and Tailscale both number their devices in
  100.64.0.0/10 and cannot run side by side.
- About 60 MB free on userdata, and some 110 MB of memory while it runs.

## Install

Download `techo5-netbird` from the [latest release](https://github.com/tioan/techo5-netbird/releases/latest)
and compare its SHA-256 with the one on the release page. Then, from your computer:

```sh
ssh root@<device> 'cat > /tmp/techo5-netbird && chmod 755 /tmp/techo5-netbird && /tmp/techo5-netbird install && rm /tmp/techo5-netbird' < techo5-netbird
```

`install` copies itself to `/data/techo5-netbird/bin`, downloads NetBird's client (about 15 MB, 40 MB unpacked), adds one marked line to
`/data/techo5-linux/rc.local` (anything else in that file is left as it is), and starts the add-on.

### A device already on TECHO5's built-in NetBird

Keep the same peer, with no new setup key:

1. Turn TECHO5's **NetBird** switch off, on the screen or in Home Assistant.
2. Install the add-on as above.
3. `ssh root@<device> /data/techo5-netbird/bin/techo5-netbird migrate`

The commands below are shown as `techo5-netbird`: run them on the device as
`/data/techo5-netbird/bin/techo5-netbird`.

`migrate` moves the peer's identity from TECHO5's NetBird folder to the add-on's, with the management
server it was joined to. Leave TECHO5's NetBird switch off from then on: turned on, it would need a new
key, and the add-on stops NetBird while it is on.

### A new device

1. **Make a setup key** in your NetBird dashboard: **Setup Keys → Create Setup Key**. Make it one-off,
   with a short expiry, and give it a group (say `techo5`) for your access policies.
2. **Join:**

   ```sh
   ssh -t root@<device> /data/techo5-netbird/bin/techo5-netbird join --management https://netbird.example.com
   ```

   Leave out `--management` for NetBird's own service. The key is asked for on the terminal and not
   shown as you type; it can also come on stdin. It is used once, to join, and not kept: not in a
   file, an argument or the log.
3. **Add the device to Home Assistant by its NetBird name**, as `<name>.<your peer domain>`, with the
   same encryption key, and remove the old entry that used its home address.

## Access policies

A new NetBird network lets every peer reach every other. Let only your Home Assistant host's group
reach the `techo5` group on TCP 6053, and only your own devices reach TCP 8181 and 22. The client's
own firewall is on and enforces these, under TECHO5's firewall, which stays the ceiling.

## Routes from the mesh

Off until you list networks. Then the device may use the routes the mesh offers it, such as the LAN at
home behind a routing peer, but only to the networks listed:

```sh
techo5-netbird routes 192.168.1.0/24            # only this network
techo5-netbird routes 192.168.1.0/24 10.20.0.0/16
techo5-netbird routes                           # what is listed
techo5-netbird routes --clear                   # none: no routes at all
```

IPv4 networks only, /8 at the widest, and none that overlap the device's own networks. NetBird would
take every route its management server offers, including ones added later. The add-on keeps the
selection to the listed networks, and selects again within 15 seconds when another route shows up.
Turning routes on or off restarts NetBird. The device never routes for others and takes no exit
node.

## Day to day

```sh
techo5-netbird status
```

It says whether the add-on starts at boot, whether NetBird runs and is connected, under which name
and address, the routes in use, and what is wrong if anything is. The log is
`/data/techo5-netbird/techo5-netbird.log`, NetBird's own lines included. At 1 MB it is kept as `.1`
and a new one is begun.

The add-on restarts NetBird if it exits: after 5 seconds at first, then up to 5 minutes apart. It
waits while a check fails (no firewall rule, no encryption key, TECHO5's NetBird or Tailscale on) and
looks again every minute.

## Updates

- **TECHO5 updates** leave the add-on alone: it lives on userdata, and `rc.local` starts it again.
- **Add-on updates:** install the new release the same way as under [Install](#install). It replaces
  the old one and starts again; the peer and the settings are kept. A release that pins a newer
  NetBird downloads it, and removes the old one.

## Uninstall

```sh
techo5-netbird uninstall           # stops it, no longer started at boot; the peer is kept
techo5-netbird uninstall --purge   # and deletes /data/techo5-netbird, the peer's identity with it
```

Then remove the device under **Peers** in your NetBird dashboard. `rc.local` keeps everything else
in it.

## Good to know

- **A reset of the device for someone else** (TECHO5's Settings) does not delete the add-on or its
  peer. Run `techo5-netbird uninstall --purge` first.
- **Turning NetBird off from far away** cuts your way to the device. Stopping the add-on over SSH
  through NetBird is the same.
- **A diagnostics bundle** from TECHO5 does not know the add-on and does not hide its NetBird name or
  address. Read it before you share it.
- **Cost on the device:** on an Echo Show 8 (2nd gen), NetBird took about 1.5% of one core when idle
  and about 4.5% with some 50 packets a second going through, and around 110 MB of memory.

## Building

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=<version>" -o techo5-netbird .
```

The tests are Linux-only: `CGO_ENABLED=0 go test ./...`.

## License

MIT, see [LICENSE](LICENSE). NetBird's client is not part of this repository or its releases: the
device downloads it from NetBird's own release, with NetBird's licenses, which the add-on keeps
beside it (BSD-3-Clause for the client). See [NOTICE](NOTICE).

Parts are adapted from TECHO5 (MIT), as noted in the source.
