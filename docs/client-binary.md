# Why the add-on uses NetBird's own release binary

The add-on runs NetBird's client exactly as NetBird publishes it: the `netbird_<version>_linux_arm64.tar.gz`
archive from NetBird's GitHub release. It does not build the client itself, and it does not publish
a client of its own. Each add-on release pins one NetBird version: its download URL, archive size
and SHA-256 are written into the add-on. Before a downloaded client is unpacked or run, the device
checks it against those values.

TECHO5's built-in NetBird did this the other way: it built the client from NetBird's source with
its own script and published that build itself. This page explains why the add-on does not.

## What you trust

With either approach, you trust NetBird's source code. The question is who turns that source into
the program on your device.

- **NetBird's binary:** you trust NetBird's build, and NetBird alone. The add-on's author never
  touches the client. You can check the pinned SHA-256 against NetBird's own
  `netbird_<version>_checksums.txt` on the same release page.
- **A build by the add-on:** you would trust NetBird's source *and* the add-on author's build machine,
  toolchain and release process. That is one more party in the chain, and a small, single-maintainer
  project is the weaker link.

## It is the same program

NetBird's release builds the Linux client from the same `client/` directory, statically and without
cgo (`CGO_ENABLED=0`), with the same `-s -w` flags and version stamp a build from source would use.
The only extra build tag (`load_wgnt_from_rsrc`) affects Windows. A build of our own would not
remove anything from the client or make it smaller: it would be the same program, built by a
different party.

## Updates come sooner

A NetBird release is something the add-on can use the day it is out: test it on a device, pin the
new hash, release. With a build of our own, every NetBird release would also need a rebuild, a
check that it is reproducible, and an upload of our own.

The same goes for Go itself. Security fixes in Go's standard library (TLS, HTTP, crypto) reach the
client through NetBird's next release. NetBird releases often, and its 0.81.4 build already uses a
newer Go (1.26.10) than the one TECHO5's own build script pinned (1.26.8).

## What a build of our own would have given, and why it matters less here

- **Choosing our own Go version.** We could rebuild for a Go fix without waiting for NetBird. In
  practice NetBird's releases keep up with Go, and a client with an urgent problem gets a NetBird
  release anyway.
- **A reproducible build that anyone can check.** Useful when a project publishes its own binary,
  because it lets others check that the binary matches the source. The add-on publishes no binary,
  so there is nothing of ours to check.
- **A check that no AGPL code is in the client.** TECHO5 needed this because it published the client
  itself. The add-on does not publish the client, so this is between NetBird and its users. NetBird's
  archive says which parts are AGPL (`LICENSES/REUSE.toml`: management, signal and relay, none of them
  in the client) and ships the licenses with it.

## What the pinned hash protects, and what it does not

- It protects against any change after the add-on's version was tested: a replaced release file, a
  tampered download, a broken mirror, a corrupt transfer. A client that does not match is never run.
- It does not protect against a NetBird release that was bad before it was pinned. NetBird publishes a
  checksum file but, as of 0.81.4, no signature or build attestation for its Linux archives, so the
  checksum only proves the file is the one on the release page. This is the same trust you place in
  NetBird when you install its client anywhere else. Every pinned version is tested on a device before
  it is released.

## Sizes

NetBird 0.81.4, arm64: about 14.6 MB to download and 40.3 MB unpacked (a static aarch64 program).
TECHO5's own build was 14.0 MB and about the same size unpacked. The archive also carries NetBird's
README, changelog and licenses. The add-on keeps the licenses next to the client and leaves out the
rest.

## If this changes

If NetBird stops publishing a static arm64 client, or one that runs on these devices, the add-on can
go back to building from NetBird's source. That would be a change to the add-on's release process,
not to how it runs on the device.

