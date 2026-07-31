import json
import os
import sys
import unittest
from pathlib import Path
from unittest.mock import patch


PLUGIN_ROOT = Path(__file__).resolve().parents[1] / "plugins"
sys.path.insert(0, str(PLUGIN_ROOT))

from viki import history_projection, register, schemas, tools  # noqa: E402


class FakeResponse:
    def __init__(self, payload: dict):
        self.payload = json.dumps(payload).encode()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False

    def read(self, _limit: int):
        return self.payload


class VikiPluginTest(unittest.TestCase):
    def setUp(self):
        self.env = patch.dict(
            os.environ,
            {
                "VIKI_INTERNAL_URL": "http://127.0.0.1:8090",
                "VIKI_HERMES_TOOL_TOKEN": "service-secret",
            },
            clear=False,
        )
        self.env.start()

    def tearDown(self):
        self.env.stop()

    def test_read_tool_forwards_runtime_identity_outside_model_arguments(self):
        captured = {}

        def fake_open(request, timeout):
            captured["request"] = request
            captured["timeout"] = timeout
            return FakeResponse({"result": {"documents": []}})

        with patch.object(tools, "_runtime_profile", return_value="qa"), patch.object(
            tools, "urlopen", side_effect=fake_open
        ):
            result = json.loads(
                tools.search_viki(
                    {"query": "zmluva", "limit": 4},
                    session_id="durable-session-123",
                    task_id="turn-456",
                )
            )

        request = captured["request"]
        self.assertEqual(result, {"documents": []})
        self.assertEqual(
            request.full_url,
            "http://127.0.0.1:8090/internal/v1/hermes/tools/search_viki",
        )
        self.assertEqual(request.get_header("Authorization"), "Bearer service-secret")
        self.assertEqual(request.get_header("X-hermes-session-id"), "durable-session-123")
        self.assertEqual(request.get_header("X-hermes-profile"), "qa")
        self.assertEqual(request.get_header("X-hermes-task-id"), "turn-456")
        self.assertEqual(json.loads(request.data), {"query": "zmluva", "limit": 4})
        self.assertNotIn("sessionId", json.loads(request.data))

    def test_missing_runtime_session_fails_closed_without_network(self):
        with patch.object(tools, "urlopen") as open_mock:
            result = json.loads(tools.get_viki_page({"pageId": "page-1"}))

        self.assertEqual(result["error"]["code"], "missing_runtime_context")
        open_mock.assert_not_called()

    def test_qa_profile_cannot_invoke_edit_handler(self):
        with patch.object(tools, "_runtime_profile", return_value="qa"), patch.object(
            tools, "urlopen"
        ) as open_mock:
            result = json.loads(
                tools.propose_viki_changeset(
                    {"summary": "Koncept", "operations": []},
                    session_id="durable-session-123",
                )
            )

        self.assertEqual(result["error"]["code"], "profile_forbidden")
        open_mock.assert_not_called()

    def test_tool_schemas_never_ask_the_model_for_actor_or_session_identity(self):
        schemas_json = json.dumps(schemas.ALL, sort_keys=True).lower()

        for forbidden in ("userid", "organizationid", "sessionid", "profile"):
            self.assertNotIn(forbidden, schemas_json)

    def test_tool_schemas_do_not_expose_removed_illustrative_metadata(self):
        self.assertNotIn("illustrative", json.dumps(schemas.ALL, sort_keys=True))

    def test_changeset_tool_requires_linked_scenario_vocabulary(self):
        description = schemas.PROPOSE_CHANGESET["description"]

        self.assertIn("všetky použité pojmy", description)
        self.assertIn("targetClientKey", description)
        self.assertIn("steps musí byť prázdne", description)

    def test_registers_exact_public_names_in_narrow_toolsets(self):
        registrations = []

        class Context:
            def register_tool(self, **registration):
                registrations.append(registration)

        register(Context())

        self.assertEqual(
            [(item["name"], item["toolset"]) for item in registrations],
            [
                ("search_viki", "viki_read"),
                ("get_viki_page", "viki_read"),
                ("get_viki_revision", "viki_read"),
                ("propose_viki_changeset", "viki_edit"),
            ],
        )
        self.assertEqual(
            [item["schema"]["name"] for item in registrations],
            [item["name"] for item in registrations],
        )
        self.assertEqual(
            set(tools._ENDPOINTS.values()),
            {item["name"] for item in registrations},
        )

    def test_history_projection_exposes_only_viki_receipts(self):
        def original(_history):
            return [
                {"role": "user", "text": "nájdi zmluvu"},
                {
                    "role": "tool",
                    "name": "search_viki",
                    "context": "secret query argument",
                },
                {
                    "role": "tool",
                    "name": "terminal",
                    "context": "unchanged context",
                },
                {
                    "role": "tool",
                    "name": "propose_viki_changeset",
                    "context": "secret changeset argument",
                },
                {
                    "role": "tool",
                    "name": "get_viki_revision",
                    "context": "secret revision argument",
                },
            ]

        class Server:
            _history_to_messages = staticmethod(original)

        history = [
            {"role": "user", "content": "nájdi zmluvu"},
            {
                "role": "tool",
                "tool_name": "search_viki",
                "content": json.dumps(
                    {
                        "documents": [
                            {
                                "revisionId": "revision-1",
                                "pageId": "page-1",
                                "pageTitle": "Zmluva",
                                "draft": False,
                                "content": "never expose retrieved prose",
                                "score": 0.98,
                            }
                        ]
                    }
                ),
            },
            {
                "role": "tool",
                "tool_name": "terminal",
                "content": "non-Viki raw output",
            },
            {
                "role": "tool",
                "tool_name": "propose_viki_changeset",
                "content": json.dumps(
                    {
                        "proposal": {
                            "id": "proposal-2",
                            "turnId": "proposal-2",
                            "summary": "Nový pojem",
                            "status": "awaiting_approval",
                            "operations": [{"content": {"bodyMd": "never expose generated prose"}}],
                        }
                    }
                ),
            },
            {
                "role": "tool",
                "tool_name": "get_viki_revision",
                "content": "not-json",
            },
        ]

        self.assertTrue(history_projection.install(Server))
        first_wrapper = Server._history_to_messages
        self.assertTrue(history_projection.install(Server))
        self.assertIs(Server._history_to_messages, first_wrapper)

        projected = Server._history_to_messages(history)

        self.assertEqual(
            projected[1],
            {
                "role": "tool",
                "name": "search_viki",
                "context": "Hľadanie vo viki",
                "result": {
                    "citations": [
                        {
                            "revisionId": "revision-1",
                            "pageId": "page-1",
                            "pageTitle": "Zmluva",
                            "draft": False,
                        }
                    ],
                    "drafts": [],
                },
            },
        )
        self.assertEqual(
            projected[2],
            {"role": "tool", "name": "terminal", "context": "unchanged context"},
        )
        self.assertEqual(
            projected[3],
            {
                "role": "tool",
                "name": "propose_viki_changeset",
                "context": "Návrh zmien vo viki",
                "result": {
                    "citations": [],
                    "drafts": [],
                    "proposal": {
                        "id": "proposal-2",
                        "turnId": "proposal-2",
                        "summary": "Nový pojem",
                        "status": "awaiting_approval",
                    },
                },
            },
        )
        self.assertEqual(
            projected[4]["result"],
            {"citations": [], "drafts": []},
        )
        serialized = json.dumps(projected, ensure_ascii=False)
        for secret in (
            "secret query argument",
            "secret changeset argument",
            "never expose retrieved prose",
            "never expose generated prose",
            "not-json",
        ):
            self.assertNotIn(secret, serialized)
        self.assertIn("non-Viki raw output", json.dumps(history, ensure_ascii=False))


if __name__ == "__main__":
    unittest.main()
