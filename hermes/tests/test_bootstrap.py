import os
import subprocess
import tempfile
import unittest
from pathlib import Path


HERMES_ROOT = Path(__file__).resolve().parents[1]


class BootstrapTest(unittest.TestCase):
    def test_bootstrap_is_idempotent_and_preserves_profile_state(self):
        with tempfile.TemporaryDirectory() as temporary:
            data = Path(temporary) / "data"
            fake_hermes = Path(temporary) / "hermes"
            command_log = Path(temporary) / "hermes-commands.log"
            fake_hermes.write_text(
                """#!/usr/bin/env python3
import os
import shutil
import sys
from pathlib import Path

root = Path(os.environ["HERMES_HOME"])
args = sys.argv[1:]
with Path(os.environ["VIKI_HERMES_TEST_COMMAND_LOG"]).open("a") as log:
    log.write(" ".join(args) + "\\n")

if args[:2] == ["profile", "install"]:
    source = Path(args[2])
    name = args[args.index("--name") + 1]
elif args[:2] == ["profile", "update"]:
    name = args[2]
    source = root / ".viki-distributions" / name
else:
    raise SystemExit(f"unexpected fake Hermes command: {args!r}")

target = root / "profiles" / name
target.mkdir(parents=True, exist_ok=True)
for user_dir in ("memories", "sessions", "skills", "logs", "home"):
    (target / user_dir).mkdir(exist_ok=True)
for entry in source.iterdir():
    destination = target / entry.name
    if entry.is_dir():
        shutil.copytree(entry, destination, dirs_exist_ok=True)
    else:
        shutil.copy2(entry, destination)
"""
            )
            fake_hermes.chmod(0o755)
            qa = data / "profiles" / "viki-qa"
            session = qa / "sessions" / "existing-session.json"
            memory = qa / "memories" / "existing-memory.md"
            session.parent.mkdir(parents=True)
            memory.parent.mkdir(parents=True)
            session.write_text("session-state")
            memory.write_text("durable-memory")

            environment = os.environ | {
                "HERMES_HOME": str(data),
                "VIKI_HERMES_ASSETS": str(HERMES_ROOT),
                "HERMES_MODEL": "gpt-test-model",
                "OPENAI_BASE_URL": "https://api.openai.test/v1",
                "VIKI_HERMES_CLI": str(fake_hermes),
                "VIKI_HERMES_TEST_COMMAND_LOG": str(command_log),
                "VIKI_BOOTSTRAP_AS_HERMES": "1",
            }
            command = [str(HERMES_ROOT / "bootstrap.sh")]

            subprocess.run(command, env=environment, check=True)
            subprocess.run(command, env=environment, check=True)

            self.assertEqual(session.read_text(), "session-state")
            self.assertEqual(memory.read_text(), "durable-memory")
            self.assertTrue((data / "profiles" / "viki-edit" / "sessions").is_dir())
            self.assertTrue((data / "profiles" / "viki-developer" / "sessions").is_dir())
            self.assertTrue(
                (data / "profiles" / "viki-developer" / "scripts" / "check_queue.py").is_file()
            )
            self.assertTrue((qa / "plugins" / "viki" / "plugin.yaml").is_file())
            self.assertTrue(
                (qa / "plugins" / "viki" / "history_projection.py").is_file()
            )
            self.assertTrue((qa / "distribution.yaml").is_file())
            self.assertIn("plugins/viki", (qa / "distribution.yaml").read_text())

            commands = command_log.read_text().splitlines()
            self.assertIn("profile install", commands[0])
            self.assertIn("--name viki-qa --force --yes", commands[0])
            self.assertIn("profile install", commands[1])
            self.assertIn("--name viki-edit --force --yes", commands[1])
            self.assertIn("profile install", commands[2])
            self.assertIn("--name viki-developer --force --yes", commands[2])
            self.assertEqual(commands[3], "profile update viki-qa --force-config --yes")
            self.assertEqual(commands[4], "profile update viki-edit --force-config --yes")
            self.assertEqual(commands[5], "profile update viki-developer --force-config --yes")

            qa_config = (qa / "config.yaml").read_text()
            edit_config = (data / "profiles" / "viki-edit" / "config.yaml").read_text()
            developer_config = (data / "profiles" / "viki-developer" / "config.yaml").read_text()
            self.assertIn('default: "gpt-test-model"', qa_config)
            self.assertIn("provider: openai-api", qa_config)
            self.assertNotIn("base_url:", qa_config)
            qa_platform = qa_config.split("platform_toolsets:", 1)[1].split(
                "known_plugin_toolsets:", 1
            )[0]
            edit_platform = edit_config.split("platform_toolsets:", 1)[1].split(
                "known_plugin_toolsets:", 1
            )[0]
            self.assertIn("cli: [memory, clarify, viki_read]", qa_platform)
            self.assertNotIn("viki_edit", qa_platform)
            self.assertIn(
                "cli: [memory, clarify, viki_read, viki_edit]", edit_platform
            )
            self.assertIn("cli: [viki_develop]", developer_config)


if __name__ == "__main__":
    unittest.main()
