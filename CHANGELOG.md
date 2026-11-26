# Changelog

All notable changes to techo5-netbird. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

For TECHO5's add-on catalog (TECHO5 v1.6.53 and later,
[HuskerMinion/techo5-addons#3](https://github.com/HuskerMinion/techo5-addons/issues/3)).

### Added

- Installed from TECHO5's **Add-ons** card, the add-on keeps everything in the state folder TECHO5 gives
  it, writes nothing to `rc.local`, and logs to the card. TECHO5 starts it and keeps it running.
- There, NetBird's client archive comes from the catalog, which pins it beside the add-on
  (`addon.json`'s `files`); the add-on checks it against its own pin and never downloads it itself.
- On its first start without a peer, a catalog install takes over an install by hand (out of
  `rc.local`, stopped, its peer and settings moved, `/data/techo5-netbird` deleted), or else the peer
  TECHO5's built-in NetBird left behind.
- An `arm` build beside `arm64`: the catalog gives every device the `arm` one, as TECHO5's own programs
  are 32-bit. It needs a 64-bit kernel for NetBird's `linux_arm64` client, and says so on one without.
- Each release has `addon.json`, `SHA256SUMS` and a build provenance attestation.
- The system's sbin folders are added to `PATH` where missing: TECHO5's add-on manager starts add-ons
  without them, and the firewall check and NetBird's client need `iptables-legacy` and `ip`.

### Changed

- NetBird's client 0.83.2 instead of 0.81.4, pinned by its size and SHA-256 as before. An update
  fetches it and deletes 0.81.4. NetBird's updater stays off on Linux in 0.83.2 as well: it only reports a
  new version and never installs one.
- Release assets are `techo5-netbird-arm` and `techo5-netbird-arm64`, with `SHA256SUMS`, instead of
  `techo5-netbird` and `techo5-netbird.sha256`. `tools/install.py` installs `techo5-netbird-arm64`, and
  still the old names with `--version` for a release before 0.2.0.
- An install by hand stops NetBird when a catalog install is on the device, and `install`,
  `uninstall` and `tools/install.py` point to the card there.

## [0.1.1] - 2026-11-21

For TECHO5 v1.6.47, which took the built-in NetBird out and keeps its peer for `migrate`.

### Changed

- `tools/install.py` asks you to turn TECHO5's NetBird switch off only when it is on, and `migrate`
  advises leaving it off only on TECHO5 up to v1.6.46: from v1.6.47 there is no switch.
- The READMEs say what changes with v1.6.47 and list the TECHO5 releases the add-on was tested on.
- `tools/install.py` shows the status at once on a device with no peer yet, instead of waiting a
  minute for a connection that cannot come.
- The READMEs list the tests since 0.1.0: a fresh join with a new setup key, and a TECHO5 update
  (v1.6.45 to v1.6.46) with the add-on running.

## [0.1.0] - 2026-11-19

The first release: NetBird for TECHO5 devices as an add-on outside TECHO5, as proposed in
[HuskerMinion/techo5#580](https://github.com/HuskerMinion/techo5/issues/580).

### Added

- NetBird's own client 0.81.4 for arm64, downloaded by the device from NetBird's release and checked
  against a pinned size and SHA-256 before it is unpacked.
- `install`: copies the add-on to `/data/techo5-netbird`, adds its marked line to TECHO5's
  `rc.local` and starts it; run again, it is an update.
- A supervisor that keeps NetBird running, restarts it with a backoff, and holds it back while
  TECHO5's firewall rule for `netbird0` is missing, Home Assistant's link has no encryption key, or
  TECHO5's own NetBird or Tailscale is on.
- `migrate`: takes over the peer of TECHO5's built-in NetBird, name, address and management server
  included, with no new setup key.
- `join`: joins with a setup key that the device asks for on the terminal and never keeps.
- `routes`: routes from the mesh, off by default, and then only to the listed IPv4 networks; a route
  to any other network is deselected again within 15 seconds.
- `status`, a rotating log, and `uninstall` (with `--purge` to delete the peer too).
- `tools/install.py`: install and update from your computer in one command, over SSH.
- READMEs in English and German, and [docs/client-binary.md](https://github.com/tioan/techo5-netbird/blob/main/docs/client-binary.md) on why the add-on
  uses NetBird's own binary.

[Unreleased]: https://github.com/tioan/techo5-netbird/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/tioan/techo5-netbird/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/tioan/techo5-netbird/releases/tag/v0.1.0
