import unittest
from pathlib import Path


HERMES_ROOT = Path(__file__).resolve().parents[1]
PROJECT_ROOT = HERMES_ROOT.parent


class RuntimeContractTest(unittest.TestCase):
    def test_edit_profile_requires_a_closed_vocabulary_pass(self):
        soul = (HERMES_ROOT / "profiles" / "edit" / "SOUL.md").read_text()
        runtime_verifier = (HERMES_ROOT / "verify_runtime.py").read_text()
        plugin_manifest = (HERMES_ROOT / "plugins" / "viki" / "plugin.yaml").read_text()
        history_projection = (HERMES_ROOT / "plugins" / "viki" / "history_projection.py").read_text()
        schemas = (HERMES_ROOT / "plugins" / "viki" / "schemas.py").read_text()

        self.assertIn("Kontrola slovníka", soul)
        self.assertIn("zákazník chce podpísať zmluvu", soul)
        self.assertIn("podstatné mená aj firemné činnosti", soul)
        self.assertIn("slovenského infinitívu", soul)
        self.assertIn("aj pri prvom výskyte", soul)
        self.assertIn("`Overiť`, `Podpísať`, `Vytvoriť` a `Zaznamenať`", soul)
        self.assertIn("`byť`, `mať`, `chcieť` a `môcť`", soul)
        self.assertNotIn("sloveso navrhni ako samostatný koncept iba vtedy", soul)
        self.assertNotIn("Pre slovo `podpísať` nevytváraj", soul)
        self.assertIn("targetClientKey", soul)
        self.assertIn("Každý koncept použitý v scenári", soul)
        self.assertIn("Funkcia nesmie obsahovať BDD kroky", soul)
        self.assertIn("Každá nová funkcia", soul)
        self.assertIn("aspoň jeden scenár", soul)
        self.assertIn("mapuj ich 1:1 na Gherkin", soul)
        self.assertIn("Kontrola definícií krokov", soul)
        self.assertIn("definitionId", soul)
        self.assertIn('"definitionId"', schemas)
        self.assertIn('"arguments"', schemas)
        self.assertIn("apply_viki_draft_changeset", soul)
        self.assertIn("vytvor reálne draftové revízie", soul)
        self.assertIn("Nikdy drafty neschvaľuj ani nepublikuj", soul)
        for content in (soul, runtime_verifier, plugin_manifest):
            self.assertIn("apply_viki_draft_changeset", content)
            self.assertNotIn("propose_viki_changeset", content)
        for content in (runtime_verifier, history_projection):
            self.assertIn("_viki_receipt_projection_v2", content)

    def test_image_is_pinned_and_runs_three_private_supervised_profiles(self):
        dockerfile = (HERMES_ROOT / "Dockerfile").read_text()
        qa_run = (HERMES_ROOT / "s6" / "hermes-qa" / "run").read_text()
        edit_run = (HERMES_ROOT / "s6" / "hermes-edit" / "run").read_text()
        developer_run = (HERMES_ROOT / "s6" / "hermes-developer" / "run").read_text()
        developer_cron_run = (HERMES_ROOT / "s6" / "hermes-developer-cron" / "run").read_text()
        proxy_run = (HERMES_ROOT / "s6" / "hermes-gateway-proxy" / "run").read_text()
        proxy = (HERMES_ROOT / "private_gateway_proxy.py").read_text()

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
        self.assertIn("-p viki-developer serve --isolated --host 127.0.0.1 --port 9121", developer_run)
        self.assertIn("HERMES_TUI_TOOLSETS=viki_develop", developer_run)
        self.assertIn("hermes -p viki-developer cron create", developer_run)
        self.assertIn("every 1m", developer_run)
        self.assertIn("--script check_queue.py", developer_run)
        self.assertIn("python3 /opt/viki-hermes/verify_runtime.py developer", developer_run)
        self.assertIn("hermes -p viki-developer gateway run", developer_cron_run)
        self.assertIn("--no-supervise", developer_cron_run)
        self.assertIn("--external-supervisor", developer_cron_run)

        self.assertIn("private_gateway_proxy.py", proxy_run)
        self.assertIn("9219", proxy)
        self.assertIn("9220", proxy)
        self.assertIn("127.0.0.1", proxy)
        self.assertIn("hermes-gateway-proxy", dockerfile)
        self.assertIn("hermes-developer-cron", dockerfile)

        for profile in ("qa", "edit", "developer"):
            config = (HERMES_ROOT / "profiles" / profile / "config.yaml").read_text()
            self.assertIn("provider: openai-api", config)
            self.assertNotIn("api_mode:", config)

        self.assertIn(
            "rm -f /etc/s6-overlay/s6-rc.d/user/contents.d/dashboard",
            dockerfile,
        )
        self.assertIn("rm -f /etc/cont-init.d/02-reconcile-profiles", dockerfile)

    def test_compose_keeps_hermes_private_without_sharing_vikis_network_namespace(self):
        compose = (PROJECT_ROOT / "docker-compose.yml").read_text()
        hermes_service = compose.split("\n  hermes:\n", 1)[1].split("\nvolumes:\n", 1)[0]

        self.assertNotIn("network_mode:", hermes_service)
        self.assertIn("hermes/Dockerfile", hermes_service)
        self.assertIn("viki-hermes:/opt/data", hermes_service)
        self.assertNotIn("ports:", hermes_service)
        self.assertIn("VIKI_INTERNAL_URL: http://viki:8090", hermes_service)
        self.assertIn("HERMES_QA_TOKEN", hermes_service)
        self.assertIn("HERMES_EDIT_TOKEN", hermes_service)
        self.assertNotIn("OPENAI_API_KEY:?", hermes_service)

        viki_service = compose.split("\n  viki:\n", 1)[1].split("\n  hermes:\n", 1)[0]
        self.assertIn("HERMES_QA_WS_URL: ws://hermes:9219/api/ws", viki_service)
        self.assertIn("HERMES_EDIT_WS_URL: ws://hermes:9220/api/ws", viki_service)
        self.assertIn("VIKI_INTERNAL_ADDRESS: 0.0.0.0:8090", viki_service)
        self.assertIn("DEVELOPMENT_TARGET_URL: http://mock-target:8091", viki_service)
        self.assertFalse(
            any(line.strip().startswith("OPENAI_") for line in viki_service.splitlines())
        )

        mock_target = compose.split("\n  mock-target:\n", 1)[1].split("\n  hermes:\n", 1)[0]
        self.assertIn("nginx:1.29.5-alpine", mock_target)
        self.assertNotIn("ports:", mock_target)


if __name__ == "__main__":
    unittest.main()
