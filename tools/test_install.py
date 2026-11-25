"""Tests for tools/install.py: python3 -m unittest discover -s tools"""
import argparse
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import install  # noqa: E402


def args(**kw):
    base = dict(device='kitchen.lan', port=None, identity=None, migrate=False, join=False, skip=False,
                management=None)
    base.update(kw)
    return argparse.Namespace(**base)


class ReleaseTest(unittest.TestCase):
    def test_urls(self):
        self.assertEqual(install.release_urls(None), (
            'https://github.com/tioan/techo5-netbird/releases/latest/download/techo5-netbird-arm64',
            'https://github.com/tioan/techo5-netbird/releases/latest/download/SHA256SUMS',
            'techo5-netbird-arm64'))
        self.assertEqual(install.release_urls('v0.1.0', legacy=True), (
            'https://github.com/tioan/techo5-netbird/releases/download/v0.1.0/techo5-netbird',
            'https://github.com/tioan/techo5-netbird/releases/download/v0.1.0/techo5-netbird.sha256',
            'techo5-netbird'))

    def test_parse_sha256(self):
        h, g = 'ab' * 32, 'cd' * 32
        sums = f'{g}  techo5-netbird-arm\n{h}  techo5-netbird-arm64\n{g}  addon.json\n'
        self.assertEqual(install.parse_sha256(sums), h)
        self.assertEqual(install.parse_sha256(f'{h.upper()} *techo5-netbird', 'techo5-netbird'), h)
        for bad in ('', f'{h}  other', f'{h}  techo5-netbird-arm', 'xyz  techo5-netbird-arm64',
                    f'{h[:-2]}  techo5-netbird-arm64'):
            with self.assertRaises(install.Fail):
                install.parse_sha256(bad)

    def test_check_program(self):
        with tempfile.TemporaryDirectory() as d:
            good = os.path.join(d, 'good')
            with open(good, 'wb') as f:  # ELF, 64-bit, little-endian, e_machine 183 (aarch64)
                f.write(b'\x7fELF\x02\x01\x01' + bytes(11) + (183).to_bytes(2, 'little') + bytes(10))
            install.check_program(good)
            for name, data in (('arm32', b'\x7fELF\x01\x01\x01' + bytes(11) + (40).to_bytes(2, 'little')),
                               ('x86', b'\x7fELF\x02\x01\x01' + bytes(11) + (62).to_bytes(2, 'little')),
                               ('text', b'#!/bin/sh\necho hello world\n'),
                               ('short', b'\x7fELF')):
                path = os.path.join(d, name)
                with open(path, 'wb') as f:
                    f.write(data)
                with self.assertRaises(install.Fail, msg=name):
                    install.check_program(path)


class DeviceTest(unittest.TestCase):
    def test_ssh_cmd(self):
        self.assertEqual(install.ssh_cmd(args(), 'true'), ['ssh', 'root@kitchen.lan', 'true'])
        self.assertEqual(install.ssh_cmd(args(device='me@10.0.0.5', port=2222, identity='k'), 'x', tty=True),
                         ['ssh', '-t', '-p', '2222', '-i', 'k', 'me@10.0.0.5', 'x'])

    def test_install_command_removes_the_copy(self):
        c = install.install_command()
        self.assertIn('install;', c)
        self.assertIn('rm -f ' + install.REMOTE_TMP, c)
        self.assertTrue(c.endswith('exit $rc'))

    def test_status_command_waits_then_shows(self):
        c = install.status_command()
        self.assertIn('Connected', c)
        self.assertTrue(c.endswith(install.REMOTE_BIN + ' status'))
        self.assertEqual(install.status_command(wait=False), install.REMOTE_BIN + ' status')

    def test_parse_probe(self):
        s = install.parse_probe('installed\ncatalog\njoined\nbuiltin-peer\nbuiltin-on\naarch64\n')
        self.assertEqual(s, dict(installed=True, joined=True, builtin_peer=True, builtin_on=True, catalog=True,
                                 arch_ok=True))
        s = install.parse_probe('armv7l\n')
        self.assertEqual(s, dict(installed=False, joined=False, builtin_peer=False, builtin_on=False, catalog=False,
                                 arch_ok=False))

    def test_probe_finds_the_switch_on(self):
        # The probe's own grep, against the ways state.json has its NetBird section.
        import subprocess
        line = [l for l in install.PROBE.split('; ') if 'builtin-on' in l][0]
        pattern = line.split("grep -q '")[1].split("'")[0]
        for text, want in (('"netbird": {\n    "on": true,\n    "management": "x"\n  }', True),
                           ('"netbird":{"joined":true,"management":"x"}', False),
                           ('"netbird": {"on": false}', False),
                           ('"tailscale": {"on": true}, "netbird": {}', False)):
            squeezed = ''.join(text.split())
            got = subprocess.run(['grep', '-q', pattern], input=squeezed, text=True).returncode == 0
            self.assertEqual(got, want, text)

    def test_next_step(self):
        new = dict(installed=False, joined=False, builtin_peer=True, arch_ok=True)
        joined = dict(new, installed=True, joined=True)
        self.assertEqual(install.next_step(new, args()), 'ask')
        self.assertIsNone(install.next_step(joined, args()))
        self.assertEqual(install.next_step(joined, args(migrate=True)), 'migrate')
        self.assertEqual(install.next_step(new, args(join=True)), 'join')
        self.assertIsNone(install.next_step(new, args(skip=True)))

    def test_shell_quote(self):
        self.assertEqual(install.shell_quote("https://a.b/c'd"), "'https://a.b/c'\\''d'")


if __name__ == '__main__':
    unittest.main()
