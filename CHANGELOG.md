# Changelog

All notable changes to techo5-netbird. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

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

[Unreleased]: https://github.com/tioan/techo5-netbird/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tioan/techo5-netbird/releases/tag/v0.1.0
