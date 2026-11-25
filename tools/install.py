#!/usr/bin/env python3
"""Install or update techo5-netbird on a TECHO5 device, from your computer, in one command.

    python3 tools/install.py <device>
    python3 tools/install.py <device> --migrate
    python3 tools/install.py <device> --join --management https://netbird.example.com
    python3 tools/install.py <device> --version v0.1.0 --dry-run
    python3 tools/install.py <device> --file techo5-netbird

<device> is the device's address or name on your network, as for ssh (root@ is added when no user is
given). Windows, Linux and macOS alike; needs Python 3 and an ssh client (Windows 10 and later have one),
and SSH turned on in TECHO5 with your key set.

What it does, each step checked before the next:

  1. release   downloads techo5-netbird and its SHA-256 from this project's release on GitHub (the
               latest, or --version) and compares them; --file takes a program you have instead
  2. install   sends it to the device over ssh and runs its install there: it copies itself to
               /data/techo5-netbird, downloads and checks NetBird's own client, adds its line to
               rc.local and starts. The copy in /tmp is removed again.
  3. peer      on a device that has no peer of the add-on's yet: moves the peer of TECHO5's built-in
               NetBird over (migrate), or joins with a setup key (join), asking which unless told
               (--migrate, --join, --skip). The setup key is asked for by the device, over ssh -t: it
               never passes through this script.

Run again for an update: the peer and the settings are kept.

The SHA-256 comes from the same release as the program: it catches a download that went wrong, not a
release that is not this project's.
"""
import argparse
import hashlib
import os
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request

REPO = 'tioan/techo5-netbird'
# The program an install by hand runs, and the file of SHA-256 sums beside it in a release. Releases
# before 0.2.0 had one program, under the add-on's name, with a SHA-256 file of its own (LEGACY).
ASSET = 'techo5-netbird-arm64'
SUMS = 'SHA256SUMS'
LEGACY = ('techo5-netbird', 'techo5-netbird.sha256')
REMOTE_BIN = '/data/techo5-netbird/bin/techo5-netbird'
REMOTE_TMP = '/tmp/techo5-netbird.install'

# What the device has, asked before anything is installed; one word per line.
PROBE = (
    f'[ -x {REMOTE_BIN} ] && echo installed; '
    # Installed from TECHO5's Add-ons card (v1.6.53 and later), which updates and removes it.
    '[ -f /data/techo5-addons/techo5-netbird/addon.json ] && echo catalog; '
    'grep -q \'"joined": *true\' /data/techo5-netbird/config.json 2>/dev/null && echo joined; '
    '[ -f /data/misc/techo5/netbird/state/default.json ] && echo builtin-peer; '
    # TECHO5's own NetBird switch, on: up to v1.6.46 only, where it has one.
    'tr -d " \\n\\t" < /data/misc/techo5/state.json 2>/dev/null | grep -q \'"netbird":{[^}]*"on":true\' && echo builtin-on; '
    'uname -m; true'
)


class Fail(Exception):
    """A reason to stop, said plainly."""


# ------------------------------------------------------------------------------------------ release

def release_urls(version, legacy=False):
    """The program's and its SHA-256 file's download URLs, for a release tag or the latest, and the
    program's name in the release."""
    if version:
        base = f'https://github.com/{REPO}/releases/download/{version}/'
    else:
        base = f'https://github.com/{REPO}/releases/latest/download/'
    asset, sums = LEGACY if legacy else (ASSET, SUMS)
    return base + asset, base + sums, asset


def parse_sha256(text, asset=ASSET):
    """The SHA-256 a sha256sum file gives the program."""
    for line in text.splitlines():
        parts = line.split()
        if len(parts) == 2 and parts[1].lstrip('*') == asset:
            h = parts[0].lower()
            if len(h) == 64 and all(c in '0123456789abcdef' for c in h):
                return h
    raise Fail(f'the release\'s SHA-256 file does not name {asset}')


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


def _open(url):
    return urllib.request.urlopen(urllib.request.Request(url, headers={'User-Agent': 'techo5-netbird-installer'}), timeout=60)


def fetch_release(version, folder):
    """Downloads the program into folder and answers its path, once its SHA-256 is the release's."""
    try:
        url, sum_url, asset = release_urls(version)
        try:
            with _open(sum_url) as r:
                sums = r.read()
        except urllib.error.HTTPError as e:
            if e.code != 404 or not version:
                raise
            url, sum_url, asset = release_urls(version, legacy=True)  # a release before 0.2.0
            with _open(sum_url) as r:
                sums = r.read()
        print(f'release:  {url}')
        want = parse_sha256(sums.decode('utf-8', 'replace'), asset)
        out = os.path.join(folder, ASSET)
        with _open(url) as r, open(out, 'wb') as f:
            shutil.copyfileobj(r, f, 1 << 20)
    except urllib.error.HTTPError as e:
        if e.code == 404:
            raise Fail(f'no release {version or "yet"} with techo5-netbird in it ({e.url})') from e
        raise Fail(f'downloading the release: {e}') from e
    except OSError as e:
        raise Fail(f'downloading the release: {e}') from e
    got = sha256_file(out)
    if got != want:
        raise Fail(f'the download\'s SHA-256 is {got}, the release says {want}: not installed')
    print(f'          SHA-256 {got} checked')
    return out


def check_program(path):
    """Refuses a file that is not a 64-bit ARM Linux program, before it goes anywhere."""
    with open(path, 'rb') as f:
        head = f.read(20)
    if len(head) < 20 or head[:4] != b'\x7fELF' or head[4] != 2 or int.from_bytes(head[18:20], 'little') != 183:
        raise Fail(f'{path} is not techo5-netbird for the device (a 64-bit ARM Linux program)')


# ------------------------------------------------------------------------------------------ device

def ssh_target(device):
    return device if '@' in device else 'root@' + device


def ssh_cmd(args, remote, tty=False):
    cmd = ['ssh']
    if tty:
        cmd.append('-t')
    if args.port:
        cmd += ['-p', str(args.port)]
    if args.identity:
        cmd += ['-i', args.identity]
    return cmd + [ssh_target(args.device), remote]


def install_command():
    """What runs on the device: the program comes on stdin, installs itself and goes again from /tmp."""
    return (f'cat > {REMOTE_TMP} && chmod 755 {REMOTE_TMP} && {REMOTE_TMP} install; '
            f'rc=$?; rm -f {REMOTE_TMP}; exit $rc')


def status_command(wait=True):
    """The add-on's status: once NetBird is connected or after a minute, whichever is first, or at once
    on a device with no peer yet, which has nothing to connect with."""
    if not wait:
        return f'{REMOTE_BIN} status'
    return (f'i=0; while [ $i -lt 30 ] && ! {REMOTE_BIN} status | grep -q "^netbird: *Connected"; '
            f'do sleep 2; i=$((i+1)); done; {REMOTE_BIN} status')


def parse_probe(out):
    words = set(out.split())
    return {
        'installed': 'installed' in words,
        'joined': 'joined' in words,
        'builtin_peer': 'builtin-peer' in words,
        'builtin_on': 'builtin-on' in words,
        'catalog': 'catalog' in words,
        'arch_ok': 'aarch64' in words or 'arm64' in words,
    }


def next_step(state, args):
    """migrate, join or None: what to do once installed."""
    if args.skip:
        return None
    if args.migrate:
        return 'migrate'
    if args.join:
        return 'join'
    if state['joined']:
        return None  # an update: the add-on has its peer
    return 'ask'


def ask(question, default=True):
    hint = '[Y/n]' if default else '[y/N]'
    while True:
        a = input(f'{question} {hint} ').strip().lower()
        if not a:
            return default
        if a in ('y', 'yes', 'j', 'ja'):
            return True
        if a in ('n', 'no', 'nein'):
            return False


def run(cmd, **kw):
    return subprocess.run(cmd, **kw).returncode


def main(argv=None):
    p = argparse.ArgumentParser(description='Install or update techo5-netbird on a TECHO5 device.')
    p.add_argument('device', help='the device, as for ssh: an address or name (root@ is added)')
    p.add_argument('--version', help='a release tag, such as v0.1.0 (default: the latest)')
    p.add_argument('--file', help='install this program instead of downloading the release')
    p.add_argument('--dry-run', action='store_true', help='download and check the release only')
    p.add_argument('-p', '--port', type=int, help='the device\'s SSH port')
    p.add_argument('-i', '--identity', help='the SSH key to use')
    peer = p.add_mutually_exclusive_group()
    peer.add_argument('--migrate', action='store_true', help='move the peer of TECHO5\'s built-in NetBird over')
    peer.add_argument('--join', action='store_true', help='join with a setup key, asked for by the device')
    peer.add_argument('--skip', action='store_true', help='install only; migrate or join later')
    p.add_argument('--management', help='for --join: your management server (default: NetBird\'s own)')
    args = p.parse_args(argv)
    sys.stdout.reconfigure(line_buffering=True)  # in order with ssh's output, also when piped
    if args.management and args.migrate:
        p.error('--management goes with --join: migrate keeps the management server the peer was joined to')
    if args.file and args.version:
        p.error('--file or --version, not both')

    try:
        with tempfile.TemporaryDirectory(prefix='techo5-netbird-') as tmp:
            if args.file:
                program = args.file
                check_program(program)
                print(f'program:  {program} (SHA-256 {sha256_file(program)})')
            else:
                program = fetch_release(args.version, tmp)
            check_program(program)
            if args.dry_run:
                print('dry run: nothing sent to the device')
                return 0

            print(f'device:   {ssh_target(args.device)}')
            r = subprocess.run(ssh_cmd(args, PROBE), stdout=subprocess.PIPE, text=True)
            if r.returncode != 0:
                raise Fail('ssh to the device failed: is SSH on in TECHO5, with your key set?')
            state = parse_probe(r.stdout)
            if state['catalog']:
                raise Fail('techo5-netbird is installed from TECHO5\'s Add-ons card on this device '
                           '(setup page, System tab): update it there')
            if not state['arch_ok']:
                raise Fail('the device is not one the add-on runs on (it needs a 64-bit ARM kernel)')
            print('          ' + ('an update of the add-on' if state['installed'] else 'a new install'))

            with open(program, 'rb') as f:
                if run(ssh_cmd(args, install_command()), stdin=f) != 0:
                    raise Fail('the install on the device failed: its message is above')

        step = next_step(state, args)
        if step == 'ask':
            if state['builtin_peer'] and ask('This device has a peer of TECHO5\'s built-in NetBird. '
                                             'Move it over to the add-on, keeping its name and address?'):
                step = 'migrate'
                if state['builtin_on']:
                    input('Turn TECHO5\'s NetBird switch off (on the screen or in Home Assistant), then press Enter. ')
            elif ask('Join your NetBird network now, with a setup key?'):
                step = 'join'
                if not args.management:
                    args.management = input('Management server (empty for NetBird\'s own): ').strip() or None
            else:
                step = None
        if step == 'migrate':
            if run(ssh_cmd(args, f'{REMOTE_BIN} migrate')) != 0:
                raise Fail('migrate did not finish: its message is above')
        elif step == 'join':
            remote = f'{REMOTE_BIN} join'
            if args.management:
                remote += ' --management ' + shell_quote(args.management)
            if run(ssh_cmd(args, remote, tty=True)) != 0:
                raise Fail('join did not finish: its message is above')
        elif not state['joined']:
            print(f'Later: ssh {ssh_target(args.device)} {REMOTE_BIN} migrate, or ssh -t … join')

        run(ssh_cmd(args, status_command(wait=bool(step) or state['joined'])))
        return 0
    except Fail as e:
        print(f'techo5-netbird install: {e}', file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print('\nstopped', file=sys.stderr)
        return 130


def shell_quote(s):
    return "'" + s.replace("'", "'\\''") + "'"


if __name__ == '__main__':
    sys.exit(main())
