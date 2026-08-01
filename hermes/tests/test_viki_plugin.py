import json
import os
import sys
import unittest
from io import BytesIO
from pathlib import Path
from types import SimpleNamespace
from urllib.error import HTTPError, URLError
from unittest.mock import patch


PLUGIN_ROOT = Path(__file__).resolve().parents[1] / "plugins"
sys.path.insert(0, str(PLUGIN_ROOT))

from viki import history_projection, register, schemas, tools  # noqa: E402


class FakeResponse:
    def __init__(self, payload):
        self.payload = payload if isinstance(payload, bytes) else json.dumps(payload).encode()

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

    def test_managed_profile_is_derived_from_hermes_home(self):
        hermes_constants = SimpleNamespace(
            get_hermes_home=lambda: "/opt/data/profiles/viki-edit"
        )
        with patch.dict(sys.modules, {"hermes_constants": hermes_constants}):
            self.assertEqual(tools._runtime_profile(), "edit")

        hermes_constants.get_hermes_home = lambda: "/opt/data/profiles/unmanaged"
        with patch.dict(sys.modules, {"hermes_constants": hermes_constants}):
            with self.assertRaisesRegex(RuntimeError, "not scoped"):
                tools._runtime_profile()

    def test_tool_rejects_unmanaged_session_and_unsafe_configuration(self):
        with patch.object(tools, "_runtime_profile", side_effect=RuntimeError):
            result = json.loads(
                tools.search_viki({"query": "zmluva"}, session_id="session-1")
            )
        self.assertEqual(result["error"]["code"], "missing_runtime_context")

        with patch.object(tools, "_runtime_profile", return_value="qa"), patch.dict(
            os.environ, {"VIKI_HERMES_TOOL_TOKEN": ""}
        ):
            result = json.loads(
                tools.search_viki({"query": "zmluva"}, session_id="session-1")
            )
        self.assertEqual(result["error"]["code"], "configuration_error")

        unsafe_urls = (
            "https://127.0.0.1:8090",
            "http://example.com:8090",
            "http://user@localhost:8090",
            "http://localhost:8090?debug=1",
            "http://localhost:8090#fragment",
        )
        with patch.object(tools, "_runtime_profile", return_value="qa"):
            for unsafe_url in unsafe_urls:
                with self.subTest(url=unsafe_url), patch.dict(
                    os.environ, {"VIKI_INTERNAL_URL": unsafe_url}
                ):
                    result = json.loads(
                        tools.search_viki(
                            {"query": "zmluva"}, session_id="session-1"
                        )
                    )
                    self.assertEqual(result["error"]["code"], "configuration_error")

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

    def test_read_and_edit_tools_decode_success_and_upstream_errors(self):
        calls = []
        responses = iter(
            [
                FakeResponse({"result": {"pageId": "page-1"}}),
                FakeResponse({"result": {"revisionId": "revision-1"}}),
                FakeResponse({"result": {"proposal": {"id": "proposal-1"}}}),
                FakeResponse({"error": {"code": "conflict", "message": "stale"}}),
                FakeResponse({"unexpected": True}),
                FakeResponse(b"not-json"),
                FakeResponse(b"\xff"),
            ]
        )

        def fake_open(request, timeout):
            calls.append(request)
            self.assertEqual(timeout, 30)
            return next(responses)

        with patch.object(tools, "_runtime_profile", return_value="edit"), patch.object(
            tools, "urlopen", side_effect=fake_open
        ):
            with patch.dict(os.environ, {}, clear=False):
                os.environ.pop("VIKI_INTERNAL_URL", None)
                page = json.loads(
                    tools.get_viki_page({"pageId": "page-1"}, session_id="session-1")
                )
            revision = json.loads(
                tools.get_viki_revision(
                    {"revisionId": "revision-1"}, session_id="session-1"
                )
            )
            proposal = json.loads(
                tools.propose_viki_changeset(
                    {"summary": "Zmena", "operations": []}, session_id="session-1"
                )
            )
            upstream_error = json.loads(
                tools.search_viki({"query": "x"}, session_id="session-1")
            )
            unexpected = json.loads(
                tools.search_viki({"query": "x"}, session_id="session-1")
            )
            malformed = json.loads(
                tools.search_viki({"query": "x"}, session_id="session-1")
            )
            invalid_unicode = json.loads(
                tools.search_viki({"query": "x"}, session_id="session-1")
            )

        self.assertEqual(page, {"pageId": "page-1"})
        self.assertEqual(revision, {"revisionId": "revision-1"})
        self.assertEqual(proposal, {"proposal": {"id": "proposal-1"}})
        self.assertEqual(upstream_error["error"]["code"], "conflict")
        for result in (unexpected, malformed, invalid_unicode):
            self.assertEqual(result["error"]["code"], "invalid_upstream_response")
        self.assertEqual(
            calls[0].full_url,
            "http://127.0.0.1:8090/internal/v1/hermes/tools/get_viki_page",
        )
        self.assertIsNone(calls[0].get_header("X-hermes-task-id"))

    def test_tools_bound_upstream_failures_and_response_sizes(self):
        small_error = HTTPError(
            "http://localhost", 409, "conflict", {}, BytesIO(
                json.dumps({"error": {"code": "conflict"}}).encode()
            )
        )
        large_error = HTTPError(
            "http://localhost",
            500,
            "large",
            {},
            BytesIO(b"x" * (tools._MAX_RESPONSE_BYTES + 1)),
        )
        outcomes = (
            (small_error, "conflict"),
            (large_error, "upstream_response_too_large"),
            (TimeoutError(), "upstream_unavailable"),
            (URLError("offline"), "upstream_unavailable"),
            (OSError("offline"), "upstream_unavailable"),
        )

        with patch.object(tools, "_runtime_profile", return_value="qa"):
            for error, expected_code in outcomes:
                with self.subTest(error=type(error).__name__), patch.object(
                    tools, "urlopen", side_effect=error
                ):
                    result = json.loads(
                        tools.search_viki({"query": "x"}, session_id="session-1")
                    )
                    self.assertEqual(result["error"]["code"], expected_code)

            with patch.object(
                tools,
                "urlopen",
                return_value=FakeResponse(b"x" * (tools._MAX_RESPONSE_BYTES + 1)),
            ):
                result = json.loads(
                    tools.search_viki({"query": "x"}, session_id="session-1")
                )
                self.assertEqual(
                    result["error"]["code"], "upstream_response_too_large"
                )

    def test_tool_schemas_never_ask_the_model_for_actor_or_session_identity(self):
        schemas_json = json.dumps(schemas.ALL, sort_keys=True).lower()

        for forbidden in ("userid", "organizationid", "sessionid", "profile"):
            self.assertNotIn(forbidden, schemas_json)

    def test_tool_schemas_do_not_expose_removed_illustrative_metadata(self):
        self.assertNotIn("illustrative", json.dumps(schemas.ALL, sort_keys=True))

    def test_changeset_tool_requires_linked_feature_vocabulary(self):
        description = schemas.PROPOSE_CHANGESET["description"]

        self.assertIn("všetky použité koncepty", description)
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
                            "summary": "Nový koncept",
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
                        "summary": "Nový koncept",
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

    def test_history_projection_normalizes_every_supported_receipt_shape(self):
        def original(_history):
            return [
                "not a message",
                {"role": "assistant", "text": "answer"},
                {"role": "tool", "name": "search_viki"},
                {"role": "tool", "name": "get_viki_page"},
                {"role": "tool", "name": "get_viki_revision"},
                {"role": "tool", "name": "propose_viki_changeset"},
                {"role": "tool", "name": "search_viki"},
                {"role": "tool", "name": "unknown"},
            ]

        class Server:
            _history_to_messages = staticmethod(original)

        revision = {
            "id": "revision-1",
            "pageId": "page-1",
            "title": "Zmluva",
            "status": "draft",
        }
        page_revision = {
            "revisionId": "revision-2",
            "pageId": "page-2",
            "pageTitle": "Zákazník",
            "draft": False,
        }
        history = [
            None,
            {"role": "assistant", "content": "answer"},
            {
                "role": "tool",
                "content": {
                    "result": {"documents": [revision, revision, None, {}]}
                },
            },
            {
                "role": "tool",
                "content": bytearray(
                    json.dumps(
                        {
                            "acceptedRevision": page_revision,
                            "draftRevision": None,
                        }
                    ).encode()
                ),
            },
            {
                "role": "tool",
                "content": json.dumps(json.dumps(page_revision)),
            },
            {"role": "tool", "content": []},
        ]

        self.assertTrue(history_projection.install(Server))
        projected = Server._history_to_messages(history)

        self.assertEqual(projected[2]["result"]["citations"], [{
            "revisionId": "revision-1",
            "pageId": "page-1",
            "pageTitle": "Zmluva",
            "draft": True,
        }])
        self.assertEqual(projected[3]["result"]["citations"], [page_revision])
        self.assertEqual(projected[4]["result"]["citations"], [page_revision])
        self.assertIsNone(projected[5]["result"]["proposal"])
        self.assertEqual(projected[6]["result"], {"citations": [], "drafts": []})
        self.assertNotIn("result", projected[7])

    def test_history_projection_installation_fails_closed(self):
        missing_gateway = ModuleNotFoundError("missing", name="tui_gateway")
        with patch.object(
            history_projection.importlib,
            "import_module",
            side_effect=missing_gateway,
        ):
            self.assertFalse(history_projection.install())

        nested_dependency = ModuleNotFoundError("missing", name="dependency")
        with patch.object(
            history_projection.importlib,
            "import_module",
            side_effect=nested_dependency,
        ):
            with self.assertRaises(ModuleNotFoundError):
                history_projection.install()

        with self.assertRaisesRegex(RuntimeError, "projection is unavailable"):
            history_projection.install(SimpleNamespace())

        server = SimpleNamespace(_history_to_messages=lambda history: history)
        with patch.object(
            history_projection.importlib, "import_module", return_value=server
        ):
            self.assertTrue(history_projection.install())

    def test_history_projection_drops_unsupported_tool_data(self):
        result = history_projection._safe_result(
            "unsupported",
            {
                "documents": [
                    {
                        "revisionId": "revision-secret",
                        "pageId": "page-secret",
                        "pageTitle": "Secret",
                    }
                ]
            },
        )

        self.assertEqual(result, {"citations": [], "drafts": []})


if __name__ == "__main__":
    unittest.main()
