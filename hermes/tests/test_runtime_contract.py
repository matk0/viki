import unittest
from pathlib import Path


HERMES_ROOT = Path(__file__).resolve().parents[1]
PROJECT_ROOT = HERMES_ROOT.parent


class RuntimeContractTest(unittest.TestCase):
    def test_edit_profile_requires_a_closed_vocabulary_pass(self):
        soul = (HERMES_ROOT / "profiles" / "edit" / "SOUL.md").read_text()

        self.assertIn("Kontrola slovníka", soul)
        self.assertIn("zákazník chce podpísať zmluvu", soul)
        self.assertIn("targetClientKey", soul)
        self.assertIn("Každý pojem použitý v scenári", soul)
        self.assertIn("Scenár nesmie obsahovať BDD kroky", soul)

    def test_image_is_pinned_and_runs_two_loopback_only_supervised_profiles(self):
        dockerfile = (HERMES_ROOT / "Dockerfile").read_text()
        qa_run = (HERMES_ROOT / "s6" / "hermes-qa" / "run").read_text()
        edit_run = (HERMES_ROOT / "s6" / "hermes-edit" / "run").read_text()

        self.assertIn(
            "nousresearch/hermes-agent:v2026.7.20@sha256:f7b35053268f532f98955195c909f15a230470fbcbdacaa9fdecb95707dad04a",
            dockerfile,
        )
        self.assertIn("/etc/cont-init.d/03-viki-bootstrap", dockerfile)
        self.assertIn('CMD ["sleep", "infinity"]', dockerfile)

        self.assertIn("-p viki-qa serve --isolated --host 127.0.0.1 --port 9119", qa_run)
        self.assertIn("HERMES_TUI_TOOLSETS=memory,clarify,viki_read", qa_run)
        self.assertIn("HERMES_TUI_PASS_SESSION_ID=0", qa_run)
        self.assertNotIn("HERMES_TUI_PASS_SESSION_ID=1", qa_run)
        self.assertIn("python3 /opt/viki-hermes/verify_runtime.py qa", qa_run)
        self.assertIn("HERMES_QA_TOKEN", qa_run)
        self.assertNotIn("viki_edit", qa_run)

        self.assertIn("-p viki-edit serve --isolated --host 127.0.0.1 --port 9120", edit_run)
        self.assertIn("HERMES_TUI_TOOLSETS=memory,clarify,viki_read,viki_edit", edit_run)
        self.assertIn("HERMES_TUI_PASS_SESSION_ID=0", edit_run)
        self.assertNotIn("HERMES_TUI_PASS_SESSION_ID=1", edit_run)
        self.assertIn("python3 /opt/viki-hermes/verify_runtime.py edit", edit_run)
        self.assertIn("HERMES_EDIT_TOKEN", edit_run)

        for profile in ("qa", "edit"):
            config = (HERMES_ROOT / "profiles" / profile / "config.yaml").read_text()
            self.assertIn("provider: openai-api", config)
            self.assertNotIn("api_mode:", config)

        self.assertIn(
            "rm -f /etc/s6-overlay/s6-rc.d/user/contents.d/dashboard",
            dockerfile,
        )
        self.assertIn("rm -f /etc/cont-init.d/02-reconcile-profiles", dockerfile)

    def test_compose_keeps_hermes_private_and_persists_its_state(self):
        compose = (PROJECT_ROOT / "docker-compose.yml").read_text()
        hermes_service = compose.split("\n  hermes:\n", 1)[1].split("\nvolumes:\n", 1)[0]

        self.assertIn("network_mode: service:viki", hermes_service)
        self.assertIn("hermes/Dockerfile", hermes_service)
        self.assertIn("viki-hermes:/opt/data", hermes_service)
        self.assertNotIn("ports:", hermes_service)
        self.assertIn("HERMES_QA_TOKEN", hermes_service)
        self.assertIn("HERMES_EDIT_TOKEN", hermes_service)
        self.assertNotIn("OPENAI_API_KEY:?", hermes_service)

        viki_service = compose.split("\n  viki:\n", 1)[1].split("\n  hermes:\n", 1)[0]
        self.assertFalse(
            any(line.strip().startswith("OPENAI_") for line in viki_service.splitlines())
        )


if __name__ == "__main__":
    unittest.main()
