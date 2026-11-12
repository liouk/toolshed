"""Exercise config persistence and add flows without accessing GitHub."""

import errno
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import shutil
import subprocess
import tempfile
import termios
import time
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "ghproj.sh"
CONFIG = """owner: example
projects:
- project: 7
  view_url: https://github.com/pulls/example
  custom_field:
    name: Priority
    default: "P2 - Normal"
    shortcuts:
      0: "P0 - Urgent"
      1: "P1 - High"
      2: "P2 - Normal"
      3: "P3 - Low"
- project: 9
"""
OPTIONS = ["P0 - Urgent", "P1 - High", "P2 - Normal", "P3 - Low"]
MOCK = """#!/usr/bin/env python3
import json, os, pathlib, subprocess, sys, time
args = sys.argv[1:]
with open(os.environ['CALL_LOG'], 'a') as f:
    f.write(json.dumps([pathlib.Path(sys.argv[0]).name] + args) + '\\n')
if pathlib.Path(sys.argv[0]).name == 'gum':
    if args[0] == 'spin':
        if os.environ.get('REAL_GUM'):
            os.execv(os.environ['REAL_GUM'], [os.environ['REAL_GUM']] + args)
        sys.exit(subprocess.call(args[args.index('--') + 1:]))
    if os.environ.get('CANCEL_INPUT'):
        sys.exit(130)
    if 'INPUT_VALUES' in os.environ:
        calls = [json.loads(line) for line in pathlib.Path(os.environ['CALL_LOG']).read_text().splitlines()]
        count = sum(call[:2] == ['gum', 'input'] for call in calls)
        print(json.loads(os.environ['INPUT_VALUES'])[count - 1])
    else:
        print(os.environ.get('INPUT_VALUE', 'example/repo#123'))
elif args[:2] == ['project', 'view']:
    print('Example project')
elif args[:2] == ['project', 'field-list']:
    print(json.dumps({'fields': [] if os.environ.get('MISSING_FIELD') else
                     [{'id': 'FIELD', 'name': 'Priority'}]}))
elif args[:2] == ['api', 'graphql']:
    print(os.environ['METADATA'])
elif args[:2] == ['project', 'item-add']:
    time.sleep(float(os.environ.get('REQUEST_DELAY', '0')))
    if os.environ.get('FAIL_ADD'):
        sys.exit(1)
    print('ITEM' if '--jq' in args else 'added')
elif args[:2] == ['project', 'item-edit']:
    time.sleep(float(os.environ.get('REQUEST_DELAY', '0')))
    sys.exit(1 if os.environ.get('FAIL_EDIT') else 0)
else:
    sys.exit('Unexpected mock command: ' + repr(args))
"""


class GhprojTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.config = self.root / "toolshed/ghproj/config.yaml"
        self.config.parent.mkdir(parents=True)
        self.config.write_text(CONFIG)
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        for name in ("gh", "gum"):
            command = bin_dir / name
            command.write_text(MOCK)
            command.chmod(0o755)
        self.log = self.root / "calls.jsonl"
        self.env = dict(os.environ, XDG_CONFIG_HOME=str(self.root),
                        PATH=f"{bin_dir}:{os.environ['PATH']}",
                        PROJECT_OWNER="", CALL_LOG=str(self.log), TERM="xterm")
        self.metadata("SINGLE_SELECT")

    def metadata(self, kind):
        self.env["METADATA"] = json.dumps({
            "id": "FIELD", "name": "Priority", "dataType": kind,
            "project": {"id": "PROJECT"},
            "options": [{"id": f"OPTION{i}", "name": name}
                        for i, name in enumerate(OPTIONS)],
        })

    def calls(self, action):
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return [call for call in calls if call[:3] == ["gh", "project", action]]

    def run_cli(self, *args):
        return subprocess.run(["bash", str(SCRIPT), *args], env=self.env,
                              text=True, capture_output=True, timeout=10)

    def choose(self, keys, args=("add", "example/repo#123"), menu_key=None, observe=None):
        master, slave = pty.openpty()
        process = subprocess.Popen(["bash", str(SCRIPT), *args], env=self.env,
                                   stdin=slave, stdout=slave, stderr=slave,
                                   start_new_session=True,
                                   preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0))
        os.close(slave)
        output = b""
        sent = False
        deadline = time.monotonic() + 10
        try:
            while time.monotonic() < deadline:
                if select.select([master], [], [], 0.1)[0]:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError as error:
                        if error.errno == errno.EIO:
                            break
                        raise
                    if not chunk:
                        break
                    output += chunk
                    if observe:
                        observe(output)
                    if menu_key and b"tab switch project" in output:
                        os.write(master, menu_key)
                        menu_key = None
                    if not sent and b"digit shortcut selects" in output:
                        os.write(master, keys)
                        sent = True
                elif process.poll() is not None:
                    break
            else:
                self.fail(f"Timed out waiting for selector: {output!r}")
            return process.wait(timeout=2), output.decode(errors="replace")
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            os.close(master)

    def assert_option(self, index):
        edits = self.calls("item-edit")
        self.assertEqual(len(edits), 1)
        self.assertIn("OPTION" + str(index), edits[0])
        self.assertIn("PROJECT", edits[0])
        self.assertIn("ITEM", edits[0])

    def test_default_and_shortcuts(self):
        for keys, index in [(b"\n", 2)] + [(str(i).encode(), i) for i in range(4)]:
            with self.subTest(keys=keys):
                self.log.write_text("")
                code, output = self.choose(keys)
                self.assertEqual(code, 0, output)
                self.assert_option(index)

    def test_navigation(self):
        for keys, index in [(b"j\n", 3), (b"k\n", 1),
                            (b"\x1b[A\n", 1), (b"\x1b[B\n", 3)]:
            with self.subTest(keys=keys):
                self.log.write_text("")
                code, output = self.choose(keys)
                self.assertEqual(code, 0, output)
                self.assert_option(index)

    def test_cancel_before_add(self):
        for keys in (b"\x1b", b"\x03"):
            with self.subTest(keys=keys):
                code, output = self.choose(keys)
                self.assertNotEqual(code, 0, output)
                self.assertEqual(self.calls("item-add"), [])

    def test_link_prompt_then_priority(self):
        code, output = self.choose(b"1", args=("add",))
        self.assertEqual(code, 0, output)
        self.assert_option(1)

    def test_cancel_link_prompt(self):
        self.env["CANCEL_INPUT"] = "1"
        result = self.run_cli("add")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls("item-add"), [])

    def test_batch_uses_one_selection(self):
        self.env["INPUT_VALUE"] = "example/repo#123, https://github.com/example/repo/pull/124"
        code, output = self.choose(b"3", args=(), menu_key=b"A")
        self.assertEqual(code, 0, output)
        self.assertEqual(len(self.calls("item-add")), 2)
        edits = self.calls("item-edit")
        self.assertEqual(len(edits), 2)
        self.assertTrue(all("OPTION3" in call for call in edits))

    def test_invalid_configuration_before_add(self):
        for old, new in [("P2 - Normal", "Missing"),
                         ('0: "P0 - Urgent"', '0: "Missing"'),
                         ('0: "P0 - Urgent"', 'x: "P0 - Urgent"')]:
            with self.subTest(new=new):
                self.config.write_text(CONFIG.replace(old, new, 1))
                result = self.run_cli("add", "example/repo#123")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Custom field:", result.stderr)
                self.assertEqual(self.calls("item-add"), [])

    def test_missing_and_unsupported_fields(self):
        self.env["MISSING_FIELD"] = "1"
        result = self.run_cli("add", "example/repo#123")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cannot resolve", result.stderr)
        del self.env["MISSING_FIELD"]
        self.metadata("ITERATION")
        result = self.run_cli("add", "example/repo#123")
        self.assertIn("unsupported type", result.stderr)
        self.assertEqual(self.calls("item-add"), [])

    def test_config_roundtrip_and_project_removal(self):
        for args in [("config", "add", "10"), ("config", "remove", "9")]:
            result = self.run_cli(*args)
            self.assertEqual(result.returncode, 0, result.stderr)
        saved = self.config.read_text()
        self.assertIn('default: "P2 - Normal"', saved)
        self.assertIn('"0": "P0 - Urgent"', saved)
        code, output = self.choose(b"0")
        self.assertEqual(code, 0, output)
        self.assert_option(0)
        result = self.run_cli("config", "remove", "7")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("custom_field", self.config.read_text())
        result = self.run_cli("add", "example/repo#123")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("item-edit")), 1)

    def test_plain_and_legacy_configs(self):
        for config in ["owner: example\nprojects:\n- project: 9\n",
                       "owner: example\nproject: 9\nsecondary_project: 10\n"]:
            self.config.write_text(config)
            result = self.run_cli("add", "example/repo#123")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(self.calls("item-edit"), [])

    def test_dynamic_input_types(self):
        for kind, value, flag in [("TEXT", "Some text", "--text"),
                                  ("NUMBER", "0", "--number"),
                                  ("DATE", "2026-11-12", "--date")]:
            with self.subTest(kind=kind):
                self.log.write_text("")
                self.metadata(kind)
                self.config.write_text(CONFIG.split("    shortcuts:")[0])
                self.env["INPUT_VALUE"] = value
                result = self.run_cli("add", "example/repo#123")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(flag, self.calls("item-edit")[0])
                self.assertIn(value, self.calls("item-edit")[0])

    def test_number_and_date_validation(self):
        for kind, values in [("NUMBER", ["abc", "1.5"]),
                             ("DATE", ["2026-02-30", "2026-02-28"])]:
            with self.subTest(kind=kind):
                self.log.write_text("")
                self.metadata(kind)
                self.config.write_text(CONFIG.split("    shortcuts:")[0])
                self.env["INPUT_VALUES"] = json.dumps(values)
                result = self.run_cli("add", "example/repo#123")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("Enter a valid", result.stderr)
                self.assertIn(values[-1], self.calls("item-edit")[0])

    def test_update_failure_reports_partial_success(self):
        self.env["FAIL_EDIT"] = "1"
        code, output = self.choose(b"2")
        self.assertNotEqual(code, 0)
        self.assertIn("PR is in project #7", output)
        self.assertNotIn("✓ PR added", output)
        self.assertEqual(len(self.calls("item-add")), 1)

    def test_failed_add_does_not_edit(self):
        self.env["FAIL_ADD"] = "1"
        code, output = self.choose(b"2")
        self.assertNotEqual(code, 0, output)
        self.assertIn("Failed to add PR", output)
        self.assertNotIn("✓ PR added", output)
        self.assertEqual(self.calls("item-edit"), [])

    def test_progress_is_visible_before_requests_finish(self):
        self.env["REQUEST_DELAY"] = "0.25"
        stages = []

        def observe(output):
            if b"Adding PR to project #7" in output and "add" not in stages:
                self.assertEqual(self.calls("item-edit"), [])
                stages.append("add")
            if b"Setting Priority to P1 - High" in output and "edit" not in stages:
                self.assertNotIn("✓ PR added".encode(), output)
                stages.append("edit")

        code, output = self.choose(b"1", observe=observe)
        self.assertEqual(code, 0, output)
        self.assertEqual(stages, ["add", "edit"])
        self.assertIn("Priority: P1 - High", output)
        self.assertIn("✓ PR added to project #7 with Priority: P1 - High", output)
        self.assert_option(1)

    @unittest.skipUnless(shutil.which("gum"), "gum is not installed")
    def test_real_spinner_preserves_item_id_and_errors(self):
        self.env["REAL_GUM"] = shutil.which("gum")
        self.env["REQUEST_DELAY"] = "0.2"
        code, output = self.choose(b"2")
        self.assertEqual(code, 0, output)
        self.assert_option(2)
        self.assertIn("✓ PR added to project #7 with Priority: P2 - Normal", output)
        self.env["FAIL_EDIT"] = "1"
        code, output = self.choose(b"2")
        self.assertNotEqual(code, 0, output)
        self.assertIn("PR is in project #7", output)
        self.assertNotIn("✓ PR added", output)


if __name__ == "__main__":
    unittest.main()
