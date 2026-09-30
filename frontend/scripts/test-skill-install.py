#!/usr/bin/env python3
"""Verify the downloadable bundle and installer entirely inside temporary homes."""

import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import zipfile

FRONTEND = Path(__file__).resolve().parents[1]
ASSETS = FRONTEND / "public" / "skills"
SOURCE = ASSETS / "xunshu-api"
INSTALLER = ASSETS / "install.py"
ARCHIVE = ASSETS / "xunshu-api.zip"


class SkillDistributionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        subprocess.run(["node", "scripts/build-skills.mjs"], cwd=FRONTEND, check=True)

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="xunshu-skill-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.home = self.root / "isolated-home"
        self.home.mkdir()
        self.environment = dict(os.environ, HOME=str(self.home))
        self.environment.pop("CODEX_HOME", None)

    def install(self, *arguments, stdin=False):
        return subprocess.run(
            [sys.executable, "-" if stdin else str(INSTALLER), *arguments],
            input=INSTALLER.read_text() if stdin else None,
            text=True, capture_output=True, env=self.environment, timeout=10,
        )

    def assert_installed(self, result, parent):
        self.assertEqual(result.returncode, 0, result.stderr)
        target = parent / "xunshu-api"
        actual = {str(path.relative_to(target)): path.read_bytes()
                  for path in target.rglob("*") if path.is_file()}
        expected = {str(path.relative_to(SOURCE)): path.read_bytes()
                    for path in SOURCE.rglob("*") if path.is_file()}
        self.assertEqual(actual, expected)
        self.assertIn("XUNSHU_URL", result.stdout)
        self.assertIn("XUNSHU_API_KEY", result.stdout)
        self.assertEqual(list(parent.iterdir()), [target])

    def test_zip_matches_the_readable_source(self):
        with zipfile.ZipFile(ARCHIVE) as archive:
            self.assertIsNone(archive.testzip())
            self.assertEqual(set(archive.namelist()), {
                "xunshu-api/SKILL.md", "xunshu-api/scripts/query.py",
            })
            for info in archive.infolist():
                self.assertEqual(archive.read(info), (ASSETS / info.filename).read_bytes())
                self.assertEqual(info.date_time, (1980, 1, 1, 0, 0, 0))

    def test_assets_are_deterministic(self):
        before = (ARCHIVE.read_bytes(), INSTALLER.read_bytes())
        subprocess.run(["node", "scripts/build-skills.mjs"], cwd=FRONTEND, check=True,
                       capture_output=True)
        self.assertEqual(before, (ARCHIVE.read_bytes(), INSTALLER.read_bytes()))

    def test_codex_default_home(self):
        self.assert_installed(self.install(), self.home / ".codex" / "skills")

    def test_codex_custom_home_and_pipe(self):
        codex_directory = self.root / "配置 中文 Codex"
        self.environment["CODEX_HOME"] = str(codex_directory)
        self.assert_installed(self.install("--agent", "codex", stdin=True),
                              codex_directory / "skills")
        self.assertFalse((self.home / ".codex").exists())

    def test_claude_home_ignores_codex_home(self):
        self.environment["CODEX_HOME"] = str(self.root / "unused-codex")
        self.assert_installed(self.install("--agent", "claude"),
                              self.home / ".claude" / "skills")
        self.assertFalse((self.root / "unused-codex").exists())

    def test_explicit_directory_supports_unicode_and_spaces(self):
        destination = self.root / "我的 技能" / "nested folder"
        self.assert_installed(self.install("--agent", "claude", "--dir", str(destination)),
                              destination)
        self.assertEqual(list(self.home.iterdir()), [])

    def test_reinstall_preserves_local_edits(self):
        destination = self.root / "skills"
        self.assert_installed(self.install("--dir", str(destination)), destination)
        existing = destination / "xunshu-api" / "SKILL.md"
        existing.write_text("User-owned edited instructions\n")
        result = self.install("--dir", str(destination))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Already exists", result.stderr)
        self.assertEqual(existing.read_text(), "User-owned edited instructions\n")
        self.assertEqual(list(destination.iterdir()), [existing.parent])

    def test_existing_empty_directory_is_not_replaced(self):
        destination = self.root / "skills"
        target = destination / "xunshu-api"
        target.mkdir(parents=True)
        result = self.install("--dir", str(destination))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(target.iterdir()), [])

    def test_existing_symlink_is_not_followed_or_replaced(self):
        destination = self.root / "skills"
        destination.mkdir()
        target = destination / "xunshu-api"
        target.symlink_to(self.root / "missing-target", target_is_directory=True)
        result = self.install("--dir", str(destination))
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(target.is_symlink())
        self.assertFalse((self.root / "missing-target").exists())

    def test_installation_lock_preserves_existing_installation(self):
        destination = self.root / "skills"
        lock = destination / ".xunshu-api.install-lock"
        lock.mkdir(parents=True)
        result = self.install("--dir", str(destination))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("in progress", result.stderr)
        self.assertEqual(list(destination.iterdir()), [lock])


if __name__ == "__main__":
    unittest.main()
